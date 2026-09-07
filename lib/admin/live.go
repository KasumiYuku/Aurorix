package admin

import (
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/schedule"
	"github.com/KasumiYuku/Aurorix/lib/state"
	"github.com/KasumiYuku/Aurorix/lib/templates"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// liveBus 订阅门控的实时总线: 仅在存在 SSE 订阅者时运行 1s 快照 ticker 与
// 日志转发, 订阅者归零即整体停止, 管理台空闲成本为零。
type liveBus struct {
	deps Deps

	mu   sync.Mutex
	subs map[*liveSub]struct{}
	gen  *liveGen
}

// liveGen 一次"有订阅者"生命周期内的后台资源, 停止时按代关闭, 重启不串线。
type liveGen struct {
	ticker *time.Ticker
	stop   chan struct{}
}

// liveSub 单条 SSE 连接的帧队列, 慢客户端丢旧帧 (快照本身是全量状态, 丢帧自愈)。
type liveSub struct {
	ch chan []byte
}

func newLiveBus(deps Deps) *liveBus {
	return &liveBus{deps: deps, subs: make(map[*liveSub]struct{})}
}

// subscribe 建立订阅并立即写入当前快照与任务列表; 首个订阅者拉起后台循环。
func (b *liveBus) subscribe() *liveSub {
	b.mu.Lock()
	sub := &liveSub{ch: make(chan []byte, 8)}
	b.subs[sub] = struct{}{}
	needStart := b.gen == nil
	b.mu.Unlock()

	if needStart {
		b.start()
	}
	if frame, ok := b.snapshotFrame(); ok {
		b.send(sub, frame)
	}
	b.send(sub, b.jobsFrame())
	return sub
}

func (b *liveBus) unsubscribe(sub *liveSub) {
	b.mu.Lock()
	_, ok := b.subs[sub]
	if ok {
		delete(b.subs, sub)
		close(sub.ch)
	}
	needStop := ok && len(b.subs) == 0 && b.gen != nil
	gen := b.gen
	if needStop {
		b.gen = nil
	}
	b.mu.Unlock()

	if needStop {
		close(gen.stop)
		gen.ticker.Stop()
		schedule.SetChangeHook(nil)
	}
}

// start 订阅者从 0 到 1: 拉起快照/日志两条循环, 挂上任务变更通知。
func (b *liveBus) start() {
	b.mu.Lock()
	if b.gen != nil {
		b.mu.Unlock()
		return
	}
	gen := &liveGen{ticker: time.NewTicker(time.Second), stop: make(chan struct{})}
	b.gen = gen
	schedule.SetChangeHook(func() { b.bumpJobs() })
	b.mu.Unlock()

	go b.tickLoop(gen)
	go b.logLoop(gen)
}

// tickLoop 每秒组一次全量快照并广播。
func (b *liveBus) tickLoop(gen *liveGen) {
	for {
		select {
		case <-gen.stop:
			return
		case <-gen.ticker.C:
			if frame, ok := b.snapshotFrame(); ok {
				b.push(frame)
			}
		}
	}
}

// logLoop 订阅 logx 并把每条日志转发为 SSE 事件。
func (b *liveBus) logLoop(gen *liveGen) {
	ch, cancel := logx.Subscribe()
	defer cancel()
	for {
		select {
		case <-gen.stop:
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			frame := make([]byte, 0, len(data)+32)
			frame = append(frame, "event: log\ndata: "...)
			frame = append(frame, data...)
			frame = append(frame, '\n', '\n')
			b.push(frame)
		}
	}
}

// bumpJobs 任务列表变化时立即推送; 无订阅者时略过 (新订阅自带全量列表)。
func (b *liveBus) bumpJobs() {
	b.mu.Lock()
	idle := len(b.subs) == 0
	b.mu.Unlock()
	if idle {
		return
	}
	b.push(b.jobsFrame())
}

// push 广播一帧; 慢订阅者清空积压后写入最新帧, 不死锁不堆积。
func (b *liveBus) push(frame []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		b.send(s, frame)
	}
}

// send 单订阅者投递, 缓冲满时先丢旧帧再写最新。
func (b *liveBus) send(s *liveSub, frame []byte) {
	select {
	case s.ch <- frame:
	default:
		for {
			select {
			case <-s.ch:
			default:
				goto drainDone
			}
		}
	drainDone:
		select {
		case s.ch <- frame:
		default:
		}
	}
}

// snapshotFrame 组装并编码当前全量快照; now 为服务器时钟毫秒,
// 前端据此每秒重锚 uptime, 进程重启后自动归零重计。
func (b *liveBus) snapshotFrame() ([]byte, bool) {
	view := gin.H{
		"now":     time.Now().UnixMilli(),
		"runtime": state.Snapshot(),
		"counts": gin.H{
			"plugins":   plugin.RegisteredCount(),
			"commands":  plugin.GetCommandCount(),
			"jobs":      schedule.GetJobCount(),
			"templates": templates.GetMarkdownTemplateCount(),
		},
		"logs": gin.H{
			"total":  logx.Total(),
			"errors": logx.Errors(),
		},
	}
	if b.deps.Gateway != nil {
		view["gateway"] = b.deps.Gateway()
	}
	if b.deps.Assets != nil {
		view["assets"] = assetsSummary(b.deps.Assets)
	}
	data, err := json.Marshal(view)
	if err != nil {
		return nil, false
	}
	frame := make([]byte, 0, len(data)+32)
	frame = append(frame, "event: snapshot\ndata: "...)
	frame = append(frame, data...)
	frame = append(frame, '\n', '\n')
	return frame, true
}

// jobsFrame 编码完整任务列表帧。
func (b *liveBus) jobsFrame() []byte {
	data, _ := json.Marshal(schedule.Jobs())
	frame := make([]byte, 0, len(data)+28)
	frame = append(frame, "event: jobs\ndata: "...)
	frame = append(frame, data...)
	frame = append(frame, '\n', '\n')
	return frame
}

// handleStream SSE 实时流入口。快照每秒一帧 (兼作心跳), 任务列表随变更即时
// 推送, 日志逐条推送; 断线由前端退避重连, 重连即拿到全新全量状态。
func (b *liveBus) handleStream(c *gin.Context) {
	sub := b.subscribe()
	defer b.unsubscribe(sub)

	rc := http.NewResponseController(c.Writer)
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	rc.Flush()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case frame := <-sub.ch:
			if _, err := c.Writer.WriteString(string(frame)); err != nil {
				return
			}
			rc.Flush()
		}
	}
}
