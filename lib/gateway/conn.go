package gateway

// 单次连接的私有状态: 写互斥、心跳护栏与鉴权帧, 连接结束整体废弃。

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

	wm     sync.Mutex // 串行化写帧
	hbOnce sync.Once
	hbStop chan struct{}

	interval time.Duration // HELLO 下发的心跳间隔
	acked    atomic.Bool   // 上一次心跳是否获 ACK, 仅本连接可见
}

func newConn(c *Client, ws *websocket.Conn, token string) *conn {
	return &conn{c: c, ws: ws, token: token, hbStop: make(chan struct{})}
}

// write 串行化写帧并带写超时。
func (w *conn) write(f frame) error {
	w.wm.Lock()
	defer w.wm.Unlock()
	w.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return w.ws.WriteJSON(f)
}

func (w *conn) kill() { w.ws.Close() }

// startHeartbeat 在 READY/RESUMED 后启动, 仅启动一次。
func (w *conn) startHeartbeat() {
	if w.interval <= 0 {
		return
	}
	w.hbOnce.Do(func() {
		w.acked.Store(true)
		go w.pump()
	})
}

// stopHeartbeat 停心跳循环, ticker 与 goroutine 随连接回收。
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
		// 上一次心跳未获 ACK: 连接假死, 掐断触发重连
		if !w.acked.CompareAndSwap(true, false) {
			w.kill()
			return
		}
		if err := w.write(frame{Op: opHeartbeat, D: mustJSON(map[string]any{"seq": w.c.getSeq()})}); err != nil {
			w.kill()
			return
		}
	}
}

// serve 握手后进入读循环, 直到连接断开或服务端要求重连。
func (w *conn) serve() error {
	if err := w.handshake(); err != nil {
		return err
	}
	for {
		w.ws.SetReadDeadline(time.Now().Add(readWindow))
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

// handshake 等待 HELLO, 按会话状态发送 IDENTIFY 或 RESUME。
func (w *conn) handshake() error {
	for {
		w.ws.SetReadDeadline(time.Now().Add(readWindow))
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
		w.interval = time.Duration(hello.HeartbeatInterval) * time.Millisecond
		if sess := w.c.getSession(); sess != "" {
			// RESUME 用裸 token, 区别于 IDENTIFY 的 "QQBot " 前缀
			err = w.write(frame{Op: opResume, D: mustJSON(map[string]any{
				"token":      w.token,
				"session_id": sess,
				"seq":        w.c.getSeq(),
			})})
		} else {
			err = w.write(frame{Op: opIdentify, D: mustJSON(map[string]any{
				"token":      "QQBot " + w.token,
				"intents":    w.c.intents,
				"shard":      [2]int{0, 1}, // 单实例语义: 固定整片
				"properties": map[string]string{"os": runtime.GOOS, "browser": "", "device": ""},
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
		// 服务端请求立即心跳
		return w.write(frame{Op: opHeartbeat, D: mustJSON(map[string]any{"seq": w.c.getSeq()})})
	case opHeartbeatACK:
		w.acked.Store(true)
		w.c.markAck()
	case opReconnect:
		return errReconnect
	case opInvalidSession:
		// 会话失效一律重新鉴权, 不再按 d 标志尝试 RESUME:
		// 休眠/断网后服务端会话早已过期, RESUME 只会无限失败
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
		}
		json.Unmarshal(f.D, &ready)
		w.c.setSession(ready.SessionID)
		w.c.setOnline(true)
		w.c.markReady()
		w.startHeartbeat()
		logger.Infof("连接就绪, session=%s", ready.SessionID)
	case "RESUMED":
		w.c.setOnline(true)
		w.c.markReady()
		w.startHeartbeat()
		logger.Infof("会话恢复成功")
	}
	w.c.bumpSeq(f.S)
	// 系统事件不外泄, 其余(含未知)全进事件总线
	if f.T == "READY" || f.T == "RESUMED" {
		return
	}
	w.c.dispatch(f)
}

// classify 断开原因分类: 关闭码决定会话去留与是否继续重连。
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
			// 鉴权失败: 缓存 token 一并作废, 下次重取
			w.c.api.InvalidateToken()
		}
		return errInvalidSession
	}
	return err
}
