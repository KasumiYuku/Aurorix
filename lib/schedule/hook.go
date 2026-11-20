package schedule

import "sync/atomic"

var changeHook atomic.Value

// SetChangeHook 挂载变更通知回调; 传 nil 表示卸载。
func SetChangeHook(fn func()) {
	changeHook.Store(fn)
}

// NotifyChanged 触发一次变更通知, 无回调时静默。
func NotifyChanged() {
	if fn, ok := changeHook.Load().(func()); ok && fn != nil {
		fn()
	}
}
