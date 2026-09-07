// Package event 事件域: 网关/webhook 载荷解析为类型化事件, 支持注册式订阅。
// 回调签名与 Go 生态惯例一致: func(ctx, *具体事件类型) — ctx 提供指令同款
// 消息 API, 事件数据作为第二参数直接可用。订阅者先于内置分发执行。
package event

import (
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
)

// Event 类型化事件。
type Event interface {
	Type() constant.EventType
}

// Context 事件订阅上下文: 嵌入 MessageManager 提供指令同款消息 API
// (Text/Msg/Markdown 等), 发送目标自动指向事件来源。
type Context struct {
	*context.MessageManager
}

// newContext 按事件目标构造订阅上下文。
func newContext(client *api.BotAPI, ev Event) *Context {
	mm := &context.MessageManager{Qapi: client}
	applyTarget(mm, ev)
	return &Context{MessageManager: mm}
}

// applyTarget 把事件的目标 (群/私聊) 灌入 MessageManager,
// 使 ctx.Text/Markdown 等直接发往事件来源。
func applyTarget(mm *context.MessageManager, ev any) {
	switch e := ev.(type) {
	case *MessageEvent:
		mm.EventId, mm.MessageId = e.EventID, e.MessageID
		mm.GroupId, mm.UserId, mm.Target = e.GroupID, e.UserID, e.Origin
	case *InteractionEvent:
		mm.EventId, mm.MessageId = e.EventID, e.MessageID
		mm.GroupId, mm.UserId = e.GroupID, e.UserID
		if e.Scene == "c2c" {
			mm.Target = constant.PrivateMessage
		} else {
			mm.Target = constant.GroupMessage
		}
	case *JoinRequestEvent:
		mm.EventId = e.RequestID
		mm.GroupId, mm.UserId = e.GroupID, e.UserID
		mm.Target = constant.GroupMessage
	case *MemberEvent:
		mm.GroupId = e.GroupID
		mm.Target = constant.GroupMessage
	case *RobotEvent:
		mm.GroupId = e.GroupID
		mm.Target = constant.GroupMessage
	case *ReceiveEvent:
		mm.GroupId, mm.UserId = e.GroupID, e.UserID
		mm.Target = constant.GroupMessage
	case *AuditEvent:
		mm.Target = constant.GroupMessage
	}
}
