package message

import (
	"sync/atomic"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/contract"
)

var footerHook atomic.Pointer[func(contract.CanMarshal, string, constant.Scene) contract.CanMarshal]

// SetFooterHook 注入底部按钮钩子, 传 nil 关闭; 钩子收到原有键盘、触发回复的原消息文本与目标场景, 自己决定用哪套按钮。
func SetFooterHook(fn func(contract.CanMarshal, string, constant.Scene) contract.CanMarshal) {
	if fn == nil {
		footerHook.Store(nil)
		return
	}
	footerHook.Store(&fn)
}

func applyFooter(kb contract.CanMarshal, originalInput string, scene constant.Scene) contract.CanMarshal {
	fn := footerHook.Load()
	if fn == nil {
		return kb
	}
	return (*fn)(kb, originalInput, scene)
}
