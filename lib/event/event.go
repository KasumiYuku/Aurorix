// Package event 事件域: 网关/webhook 载荷解析为类型化事件, 支持注册式订阅。
// 订阅者先于内置分发执行; 未知事件不丢弃, 透传为 UnknownEvent。
package event

import (
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// Event 类型化事件。
type Event interface {
	Type() constant.EventType
}

// Context 事件上下文: 事件对象 + 平台客户端, 供订阅者观察与主动回复。
type Context struct {
	Client *api.BotAPI
	Event  Event
}
