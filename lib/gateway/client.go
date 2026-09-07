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

	maxBackoff = 30 * time.Second // 网络退避上限
	writeWait  = 10 * time.Second // 写帧超时, 防止对端黑洞卡死
	readWindow = 90 * time.Second // 读帧超时, 防止服务端静默
)

// 决定退避节奏与会话去留
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
	gatewayURL string
	intents    int
	shard      [2]int

	backoff time.Duration // 新连接退避基数, 瞬时失败逐次翻倍

	mu        sync.Mutex
	cur       *conn     // 当前活动连接, 供 Stop 掐线
	sessionID string    // 服务端会话, RESUME 依据
	seq       int64     // 最近确认序号
	connAt    time.Time // 当前连接建立时刻, 状态页展示
	ackAt     time.Time // 最近心跳 ACK 时刻, 状态页展示
	online    bool      // READY/RESUMED 之后为真

	stopOnce sync.Once
	stop     chan struct{}
	cancel   context.CancelFunc // 根上下文取消, 掐断一切在途建连
	stopCtx  context.Context
	stopped  chan struct{}

	readyN atomic.Uint64 // READY/RESUMED 次数, 重连护栏判定依据
}

// New 创建网关客户端。
func New(api *api.BotAPI, gatewayURL string, intents int, shard [2]int) *Client {
	if shard[0] == 0 && shard[1] == 0 {
		shard = [2]int{0, 1}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		api:        api,
		gatewayURL: gatewayURL,
		intents:    intents,
		shard:      shard,
		backoff:    time.Second,
		stop:       make(chan struct{}),
		stopCtx:    ctx,
		cancel:     cancel,
		stopped:    make(chan struct{}),
	}
}

// Start 启动网关并阻塞直到进程退出（调用方应 go Start）
// 网络抖动走指数退避; opReconnect 保留会话走 RESUME
// 会话失效走限额感知重新鉴权; 服务端禁连时停止
func (c *Client) Start() {
	defer close(c.stopped)
	backoff := c.backoff
	strikes := 0 // 连续未就绪的重连次数, 达标强制重新鉴权
	for {
		if c.closed() {
			return
		}
		before := c.readyN.Load()
		err := c.run()
		switch {
		case err == nil:
			if !c.breathe(time.Second) {
				return
			}
		case errors.Is(err, errStop):
			logger.Errorf("网关不可用, 停止重连: %v", err)
			return
		case errors.Is(err, errReconnect):
			// 链路明确可用, 重置退避, 避免新连接被历史拖慢
			backoff = c.backoff
			logger.Infof("服务端要求重连, 1s 后恢复会话")
			strikes = c.countStrike(strikes, before)
			if !c.breathe(jitter(time.Second)) {
				return
			}
		case errors.Is(err, errInvalidSession):
			// 会话被服务端丢弃: 放弃旧会话, 按启动限额决定何时重新鉴权
			c.dropSession()
			if !c.waitForSlot() {
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

// countStrike 记录一轮未就绪重连; 连续 5 次未 READY 强制丢会话重新鉴权。
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
	Heartbeat bool   `json:"heartbeat_ack"` // 是否已收到过心跳 ACK
	SinceMS   int64  `json:"since_ms"`      // 当前连接已保持时长
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
	}
}

// dial 建连并握手; ctx 绑定停止信号, 任何阶段可被 Stop 打断。
// 显式超时兜底: TCP 5s + 握手 15s, 不依赖库默认值。
func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	d := &websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		NetDialContext:   (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	}
	ws, _, err := d.DialContext(ctx, c.gatewayURL, nil)
	if err != nil {
		return nil, err
	}
	// 握手完成后取消 Dial 超时约束, 交由心跳护栏管理连接
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

// dispatch 事件转发到中间件管线。
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
			logger.Errorf("解析事件数据失败 %s: %v", f.T, err)
			return
		}
	}
	middleware.ProcessAsync(payload, c.api)
}

// waitForSlot 会话失效后按 /gateway/bot 的 session_start_limit 决定重连时机。
func (c *Client) waitForSlot() bool {
	info, err := c.api.GatewayBot()
	if err != nil {
		logger.Warnf("查询会话启动限额失败, 按默认退避重连: %v", err)
		return c.breathe(jitter(5 * time.Second))
	}
	if info.Limit.Remaining > 0 || info.Limit.ResetAfter <= 0 {
		logger.Warnf("会话失效, 剩余启动配额 %d, 稍后重新鉴权", info.Limit.Remaining)
		return c.breathe(jitter(3 * time.Second))
	}
	// 配额耗尽: 等待配额窗口重置, reset_after 单位为毫秒
	wait := time.Duration(info.Limit.ResetAfter) * time.Millisecond
	logger.Warnf("会话启动配额耗尽 (%d/%d), %.0fm 后重新鉴权",
		info.Limit.Remaining, info.Limit.Total, wait.Minutes())
	return c.breathe(wait)
}

func (c *Client) closed() bool {
	select {
	case <-c.stop:
		return true
	default:
		return false
	}
}

// breathe 在退避间隔与停止信号之间选择; false 表示应退出主循环
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

// jitter 在 d 基础上叠加 ±25% 随机抖动, 避免多实例同步重连
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

// dropSession 清空本地会话, 下次握手强制 IDENTIFY
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
