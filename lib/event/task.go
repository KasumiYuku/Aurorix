package event

func runSubscriber(ctx *Context, h Handler, ev Event) {
	defer func() {
		if r := recover(); r != nil {
			busLog.Errorf("事件订阅者 panic: %v", r)
		}
	}()
	h(ctx, ev)
}
