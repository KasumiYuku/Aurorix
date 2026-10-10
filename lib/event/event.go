// Package event 事件域: 网关/webhook 载荷解析为类型化事件, 支持注册式订阅。
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
type Context struct {
	*context.MessageManager
}

func newContext(client *api.BotAPI, ev Event) *Context {
	mm := &context.MessageManager{Qapi: client}
	applyTarget(mm, ev)
	return &Context{MessageManager: mm}
}

func applyTarget(mm *context.MessageManager, ev any) {
	switch e := ev.(type) {
	case *MessageEvent:
		mm.MessageId = e.MessageID
		mm.GroupId, mm.UserId, mm.Target = e.GroupID, e.UserID, e.Origin
	case *JoinRequestEvent:
		mm.EventId = e.RequestID
		mm.GroupId, mm.UserId = e.GroupID, e.UserID
		mm.Target = constant.GroupMessage
	case *ReceiveEvent:
		mm.GroupId, mm.UserId = e.GroupID, e.UserID
		mm.Target = constant.GroupMessage
	case *FriendEvent:
		mm.UserId = e.UserID
		mm.Target = constant.PrivateMessage
	case *MemberEvent:
		if e.Added {
			mm.EventId, mm.MessageId = e.EventID, e.MessageID
		}
		mm.GroupId, mm.UserId = e.GroupID, e.UserID
		mm.Target = constant.GroupMessage
	case *UnknownEvent:
		mm.GroupId, mm.UserId = e.GroupID, e.UserID
		if e.GroupID != "" {
			mm.Target = constant.GroupMessage
		} else if e.UserID != "" {
			mm.Target = constant.PrivateMessage
		}
	case *RobotEvent:
		mm.GroupId = e.GroupID
		mm.Target = constant.GroupMessage
	case *AuditEvent:
		mm.Target = constant.GroupMessage
	case *InteractionEvent:
		mm.EventId = e.EventID
		mm.GroupId, mm.UserId = e.GroupID, e.UserID
		if e.IsPrivate() {
			mm.Target = constant.PrivateMessage
		} else {
			mm.Target = constant.GroupMessage
		}
	}
}
