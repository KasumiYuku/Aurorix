package event

// 订阅者任务调度: 复用中间件任务池的并发执行, 避免引入平行池。
// 任务池不对外暴露, 通过 SetTaskScheduler 注入解耦依赖方向。

// dispatchTask 提交订阅者任务。默认直接 go, 由 middleware 注入池化调度。
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
