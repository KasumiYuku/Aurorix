package event

var dispatchTask = func(ctx *Context, h Handler, ev Event) {
	go runSubscriber(ctx, h, ev)
}

// SetTaskScheduler 注入任务调度器 (如中间件任务池), 替代默认 goroutine。
func SetTaskScheduler(f func(func())) {
	dispatchTask = func(ctx *Context, h Handler, ev Event) {
		f(func() { runSubscriber(ctx, h, ev) })
	}
}

func runSubscriber(ctx *Context, h Handler, ev Event) {
	defer func() {
		if r := recover(); r != nil {
			busLog.Errorf("事件订阅者 panic: %v", r)
		}
	}()
	h(ctx, ev)
}
