package message

import (
	"sync/atomic"

	"github.com/KasumiYuku/Aurorix/lib/contract"
)

// 框架级「底部按钮」的注入点。
//
// 这里只留一个 CanMarshal → CanMarshal 的钩子, 具体实现由 lib/buttons 提供、lib/bot 在启动时接上:
// message 不能反向依赖 buttons —— buttons → context → message 已是既成依赖, 再加一条就成环。
var footerHook atomic.Pointer[func(contract.CanMarshal, string) contract.CanMarshal]

// SetFooterHook 注入底部按钮钩子; 传 nil 关闭。
// 钩子收到消息上原有的键盘(可能是 nil, 表示这条消息本来没按钮)和触发这条回复的原消息文本, 返回合并后的键盘。
func SetFooterHook(fn func(contract.CanMarshal, string) contract.CanMarshal) {
	if fn == nil {
		footerHook.Store(nil)
		return
	}
	footerHook.Store(&fn)
}

// applyFooter 在键盘序列化前把框架级按钮合进去; 没装钩子时原样返回。
func applyFooter(kb contract.CanMarshal, originalInput string) contract.CanMarshal {
	fn := footerHook.Load()
	if fn == nil {
		return kb
	}
	return (*fn)(kb, originalInput)
}
