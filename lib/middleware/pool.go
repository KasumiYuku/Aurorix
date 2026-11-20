package middleware

import (
	"sync/atomic"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/event"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/structers"
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
}

var pool = &taskPool{tasks: make(chan func(), poolQueue)}

// Go 提交任务; 队列满就阻塞 —— 背压传回上游, 协程数永远不出 poolMax。
func (p *taskPool) Go(f func()) {
	p.tasks <- f
	p.grow()
}

func (p *taskPool) grow() {
	for {
		n := p.alive.Load()
		if n >= poolMax {
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
			if len(p.tasks) > 0 {
				t.Reset(poolIdle)
				continue
			}
			return
		}
	}
}

func (p *taskPool) exec(f func()) {
	defer func() {
		if r := recover(); r != nil {
			dispatchLog.Errorf("任务panic: %v", r)
		}
	}()
	f()
}

// ProcessAsync 异步处理事件, 网关与 webhook 共用。
func ProcessAsync(payload structers.Payload, client *api.BotAPI) {
	pool.Go(func() {
		if blockForMaintenance(payload, client) {
			return
		}
		event.Ingest(payload, client)
	})
}
