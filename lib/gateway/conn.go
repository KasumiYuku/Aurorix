package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type conn struct {
	c     *Client
	ws    *websocket.Conn
	token string

	wm     sync.Mutex
	hbOnce sync.Once
	hbStop chan struct{}

	interval time.Duration
	acked    atomic.Bool
}

func newConn(c *Client, ws *websocket.Conn, token string) *conn {
	return &conn{c: c, ws: ws, token: token, hbStop: make(chan struct{})}
}

func (w *conn) write(f frame) error {
	w.wm.Lock()
	defer w.wm.Unlock()
	w.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return w.ws.WriteJSON(f)
}

func (w *conn) kill() { w.ws.Close() }

func (w *conn) startHeartbeat() {
	if w.interval <= 0 {
		return
	}
	w.hbOnce.Do(func() {
		w.acked.Store(true)
		go w.pump()
	})
}

func (w *conn) stopHeartbeat() {
	select {
	case <-w.hbStop:
	default:
		close(w.hbStop)
	}
}

func (w *conn) pump() {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-w.hbStop:
			return
		case <-t.C:
		}
		if !w.acked.CompareAndSwap(true, false) {
			w.kill()
			return
		}
		if err := w.write(frame{Op: opHeartbeat, D: mustJSON(w.c.getSeq())}); err != nil {
			w.kill()
			return
		}
	}
}

const heartbeatCeiling = 45 * time.Second

func heartbeatInterval(ms int) time.Duration {
	return min(time.Duration(ms)*time.Millisecond, heartbeatCeiling)
}

func (w *conn) window() time.Duration {
	if w.interval <= 0 {
		return readWindow
	}
	return max(readWindow, 2*w.interval)
}

func (w *conn) serve() error {
	if err := w.handshake(); err != nil {
		return err
	}
	for {
		w.ws.SetReadDeadline(time.Now().Add(w.window()))
		_, raw, err := w.ws.ReadMessage()
		if err != nil {
			return w.classify(err)
		}
		var f frame
		if json.Unmarshal(raw, &f) != nil {
			continue
		}
		if err := w.handle(f); err != nil {
			return err
		}
	}
}

func (w *conn) handshake() error {
	for {
		w.ws.SetReadDeadline(time.Now().Add(w.window()))
		_, raw, err := w.ws.ReadMessage()
		if err != nil {
			return fmt.Errorf("握手读帧: %w", err)
		}
		var f frame
		if json.Unmarshal(raw, &f) != nil {
			continue
		}
		if f.Op != opHello {
			continue
		}
		var hello struct {
			HeartbeatInterval int `json:"heartbeat_interval"`
		}
		json.Unmarshal(f.D, &hello)
		w.interval = heartbeatInterval(hello.HeartbeatInterval)
		if sess := w.c.getSession(); sess != "" {
			err = w.write(frame{Op: opResume, D: mustJSON(map[string]any{
				"token":      "QQBot " + w.token,
				"session_id": sess,
				"seq":        w.c.getSeq(),
			})})
		} else {
			err = w.write(frame{Op: opIdentify, D: mustJSON(map[string]any{
				"token":      "QQBot " + w.token,
				"intents":    w.c.intents,
				"shard":      [2]int{w.c.shardID, w.c.shardCount},
				"properties": map[string]string{"$os": runtime.GOOS, "$browser": "", "$device": ""},
			})})
		}
		if err != nil {
			return fmt.Errorf("发送鉴权帧: %w", err)
		}
		return nil
	}
}

func (w *conn) handle(f frame) error {
	switch f.Op {
	case opHeartbeat:
		return w.write(frame{Op: opHeartbeat, D: mustJSON(w.c.getSeq())})
	case opHeartbeatACK:
		w.acked.Store(true)
		w.c.markAck()
	case opReconnect:
		return errReconnect
	case opInvalidSession:
		return errInvalidSession
	case opDispatch:
		w.onDispatch(f)
	}
	return nil
}

func (w *conn) onDispatch(f frame) {
	switch f.T {
	case "READY":
		var ready struct {
			SessionID string `json:"session_id"`
			User      struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"user"`
		}
		json.Unmarshal(f.D, &ready)
		w.c.setSession(ready.SessionID)
		w.c.setReadyBot(ready.User.ID, ready.User.Username)
		w.c.setOnline(true)
		w.c.markReady()
		w.startHeartbeat()
		logger.Infof("连接就绪, session=%s bot=%s(%s)", ready.SessionID, ready.User.Username, ready.User.ID)
	case "RESUMED":
		w.c.setOnline(true)
		w.c.markReady()
		w.startHeartbeat()
		logger.Infof("会话恢复成功")
	}
	w.c.bumpSeq(f.S)
	if f.T == "READY" || f.T == "RESUMED" {
		return
	}
	w.c.dispatch(f)
}

func (w *conn) classify(err error) error {
	var ce *websocket.CloseError
	if !errors.As(err, &ce) {
		return err
	}
	switch {
	case !canResume(ce.Code):
		return fmt.Errorf("%w: code=%d", errStop, ce.Code)
	case needReidentify(ce.Code):
		if ce.Code == codeAuthFail {
			w.c.api.InvalidateToken()
		}
		return errInvalidSession
	}
	return err
}
