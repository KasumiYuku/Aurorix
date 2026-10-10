package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/middleware"
	"github.com/KasumiYuku/Aurorix/lib/structers"

	"github.com/gorilla/websocket"
)

var logger = logx.New("gateway")

const (
	opDispatch       = 0
	opHeartbeat      = 1
	opIdentify       = 2
	opResume         = 6
	opReconnect      = 7
	opInvalidSession = 9
	opHello          = 10
	opHeartbeatACK   = 11

	maxBackoff = 30 * time.Second
	writeWait  = 10 * time.Second
	readWindow = 90 * time.Second
)

var (
	errReconnect      = errors.New("服务端要求重连")
	errInvalidSession = errors.New("会话失效, 需重新鉴权")
	errStop           = errors.New("服务端禁止重连")
)

type frame struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	T  string          `json:"t,omitempty"`
	S  int64           `json:"s,omitempty"`
	ID string          `json:"id,omitempty"`
}

// Client WebSocket 网关客户端。
type Client struct {
	api        *api.BotAPI
	src        Source
	gatewayURL string
	intents    int
	shardID    int
	shardCount int

	backoff time.Duration

	mu        sync.Mutex
	cur       *conn
	sessionID string
	botID     string
	botName   string
	seq       int64
	connAt    time.Time
	ackAt     time.Time
	online    bool

	stopOnce sync.Once
	stop     chan struct{}
	cancel   context.CancelFunc
	stopCtx  context.Context
	stopped  chan struct{}

	readyN atomic.Uint64
}

// New 创建网关客户端; 连接地址与分片取自 src。
func New(api *api.BotAPI, src Source, intents, shardID int) *Client {
	ep := src.Endpoint()
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		api:        api,
		src:        src,
		gatewayURL: ep.URL,
		intents:    intents,
		shardID:    shardID,
		shardCount: max(ep.Shards, 1),
		backoff:    time.Second,
		stop:       make(chan struct{}),
		stopCtx:    ctx,
		cancel:     cancel,
		stopped:    make(chan struct{}),
	}
}

// Start 启动网关并阻塞直到进程退出（调用方应 go Start）
func (c *Client) Start() {
	defer close(c.stopped)
	backoff := c.backoff
	strikes := 0
	for {
		if c.closed() {
			return
		}
		before := c.readyN.Load()
		err := c.run()
		if c.readyN.Load() != before {
			backoff = c.backoff
		}
		switch {
		case err == nil:
			if !c.breathe(time.Second) {
				return
			}
		case errors.Is(err, errStop):
			logger.Errorf("网关不可用, 停止重连: %v", err)
			return
		case errors.Is(err, errReconnect):
			backoff = c.backoff
			logger.Infof("服务端要求重连, 1s 后恢复会话")
			strikes = c.countStrike(strikes, before)
			if !c.breathe(jitter(time.Second)) {
				return
			}
		case errors.Is(err, errInvalidSession):
			c.dropSession()
			if !c.breathe(c.src.ReauthDelay()) {
				return
			}
		default:
			delay := jitter(backoff)
			logger.Warnf("网络断开重连: %v, %.0fs 后重试", err, delay.Seconds())
			strikes = c.countStrike(strikes, before)
			if !c.breathe(delay) {
				return
			}
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

func (c *Client) countStrike(strikes int, before uint64) int {
	if c.readyN.Load() != before {
		return 0
	}
	strikes++
	if strikes >= 5 {
		c.dropSession()
		logger.Warnf("连续 %d 次重连未就绪, 强制重新鉴权", strikes)
		return 0
	}
	return strikes
}

// Stop 停止网关并回收连接与心跳任务。幂等。
func (c *Client) Stop() {
	c.stopOnce.Do(func() {
		close(c.stop)
		c.cancel()
	})
	c.mu.Lock()
	cur := c.cur
	c.mu.Unlock()
	if cur != nil {
		cur.kill()
	}
	<-c.stopped
}

// Status 网关实时状态, 管理台轮询用。
type Status struct {
	Connected bool   `json:"connected"`
	SessionID string `json:"session_id"`
	Seq       int64  `json:"seq"`
	Heartbeat bool   `json:"heartbeat_ack"`
	SinceMS   int64  `json:"since_ms"`
	BotID     string `json:"bot_id"`
	BotName   string `json:"bot_name"`
	ShardID   int    `json:"shard_id"`
	Shards    int    `json:"shards"`
}

func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	since := int64(0)
	if !c.connAt.IsZero() {
		since = time.Since(c.connAt).Milliseconds()
	}
	return Status{
		Connected: c.online,
		SessionID: c.sessionID,
		Seq:       c.seq,
		Heartbeat: !c.ackAt.IsZero(),
		SinceMS:   since,
		BotID:     c.botID,
		BotName:   c.botName,
		ShardID:   c.shardID,
		Shards:    c.shardCount,
	}
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	d := &websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		NetDialContext:   (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	}
	ws, _, err := d.DialContext(ctx, c.gatewayURL, c.src.Header())
	if err != nil {
		return nil, err
	}
	ws.SetReadDeadline(time.Now().Add(readWindow))
	return ws, nil
}

func (c *Client) run() error {
	ctx, cancel := context.WithCancel(c.stopCtx)
	defer cancel()
	token, err := c.api.AccessToken()
	if err != nil {
		return fmt.Errorf("获取 access token: %w", err)
	}
	ws, err := c.dial(ctx)
	if err != nil {
		return fmt.Errorf("连接网关 %s: %w", c.gatewayURL, err)
	}
	defer ws.Close()
	c.mu.Lock()
	c.connAt = time.Now()
	c.online = false
	c.mu.Unlock()

	w := newConn(c, ws, token)
	c.mu.Lock()
	c.cur = w
	c.mu.Unlock()
	defer w.stopHeartbeat()
	return w.serve()
}

func (c *Client) dispatch(f frame) {
	payload := structers.Payload{
		ID:        f.ID,
		Op:        f.Op,
		T:         f.T,
		EventType: constant.EventType(f.T),
	}
	if len(f.D) > 0 {
		payload.RawEvent = f.D
		if err := json.Unmarshal(f.D, &payload.Data); err != nil {
			logger.Warnf("解析事件数据失败 %s: %v (透传)", f.T, err)
		}
	}
	middleware.ProcessAsync(payload, c.api)
}

func (c *Client) closed() bool {
	select {
	case <-c.stop:
		return true
	default:
		return false
	}
}

func (c *Client) breathe(d time.Duration) bool {
	if d <= 0 {
		return !c.closed()
	}
	select {
	case <-c.stop:
		return false
	case <-time.After(d):
		return !c.closed()
	}
}

func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	spread := int64(d) / 2
	return d + time.Duration(rand.Int64N(spread)-spread/2)
}

func (c *Client) getSession() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

func (c *Client) setSession(id string) {
	c.mu.Lock()
	c.sessionID = id
	c.mu.Unlock()
}

func (c *Client) setReadyBot(id, name string) {
	c.mu.Lock()
	c.botID, c.botName = id, name
	c.mu.Unlock()
}

func (c *Client) getSeq() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seq
}

func (c *Client) bumpSeq(s int64) {
	c.mu.Lock()
	if s > c.seq {
		c.seq = s
	}
	c.mu.Unlock()
}

func (c *Client) setOnline(v bool) {
	c.mu.Lock()
	c.online = v
	c.mu.Unlock()
}

func (c *Client) markAck() {
	c.mu.Lock()
	c.ackAt = time.Now()
	c.mu.Unlock()
}

func (c *Client) markReady() { c.readyN.Add(1) }

func (c *Client) dropSession() {
	c.mu.Lock()
	c.sessionID = ""
	c.seq = 0
	c.mu.Unlock()
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
