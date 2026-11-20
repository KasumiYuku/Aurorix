package message

import (
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// Delivery 一次发送的结果: 平台 ID 与是否走主动通道; 主动投递可能已经落到私聊。
type Delivery struct {
	IDs    []string
	Active bool
}

// Replace 用 send 发一次、拿到平台 ID 之后再把 prev 撤掉, 是"发新的、撤旧的"唯一一处实现。先发后撤(反过来发送失败时用户手里就空着), 主动投递不撤(它已经不在原会话里), 撤回走 RecallLater 异步执行; send 返回全部 ID, 有一条落地就撤旧的。
func Replace(qapi *api.BotAPI, target constant.Target, prev []Sent, send func() (Delivery, error)) (Delivery, error) {
	got, err := send()
	if len(got.IDs) == 0 {
		return Delivery{}, err
	}
	if got.Active {
		return got, err
	}
	RecallLater(qapi, target, prev...)
	return got, err
}
