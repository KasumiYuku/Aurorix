package plugin

import "errors"

// 进程控制接缝: 插件不该自己决定进程的生死, 实现由装配期注入(见 lib/bot)。
type Control struct {
	Restartable func() error
	Restart     func()
}

// ErrControlMissing 表示插件侧进程控制未装配(未接入 bot 的装配流程)。
var ErrControlMissing = errors.New("进程控制未装配")

var control Control

// UseControl 注入进程控制实现, 由装配期调用一次。
func UseControl(c Control) { control = c }

// Restartable 报告当前进程能否由插件触发重启。
func Restartable() error {
	if control.Restartable == nil {
		return ErrControlMissing
	}
	return control.Restartable()
}

// Restart 触发重启; 未装配时什么也不做, 调用前请先用 Restartable 确认。
func Restart() {
	if control.Restart != nil {
		control.Restart()
	}
}
