package middleware

import (
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/event"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/structers"
	"sync/atomic"
	"time"
)

var dispatchLog = logx.New("dispatch")

const (
	poolQueue = 1024
	poolIdle  = 30 * time.Second
	poolMax   = 256
)

type taskPool struct {
	tasks chan func()
	alive atomic.Int64
	sem   chan struct{}
}

var pool = &taskPool{tasks: make(chan func(), poolQueue), sem: make(chan struct{}, poolMax)}

// Go 提交任务; 队列满直接临时 goroutine, 保证不丢。
func (p *taskPool) Go(f func()) {
	select {
	case p.tasks <- f:
	default:
		go p.exec(f)
		return
	}
	for {
		n := p.alive.Load()
		if n >= int64(len(p.tasks)) || n >= poolMax {
			return
		}
		if p.alive.CompareAndSwap(n, n+1) {
			go p.loop()
			return
		}
	}
}

func (p *taskPool) loop() {
	defer p.alive.Add(-1)
	t := time.NewTimer(poolIdle)
	defer t.Stop()
	for {
		select {
		case f := <-p.tasks:
			p.exec(f)
			t.Reset(poolIdle)
		case <-t.C:
			return
		}
	}
}

func (p *taskPool) exec(f func()) {
	p.sem <- struct{}{}
	defer func() { <-p.sem }()
	defer func() {
		if r := recover(); r != nil {
			dispatchLog.Errorf("任务panic: %v", r)
		}
	}()
	f()
}

// ProcessAsync 异步处理事件, 网关与 webhook 共用。
func ProcessAsync(payload structers.Payload, client *api.BotAPI) {
	pool.Go(func() { event.Ingest(payload, client) })
}
