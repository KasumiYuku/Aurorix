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
		mm.MessageId, mm.Target = e.MessageID, e.Target
	case *InteractionEvent:
		mm.EventId, mm.MessageId, mm.Target = e.EventID, e.MessageID, e.Target
	case *JoinRequestEvent:
		mm.EventId = e.RequestID
		mm.Target = constant.Target{Scene: constant.SceneGroup, ID: e.GroupID, User: e.UserID}
	case *ReceiveEvent:
		mm.Target = constant.Target{Scene: constant.SceneGroup, ID: e.GroupID, User: e.UserID}
	case *FriendEvent:
		mm.Target = constant.Target{Scene: constant.SceneC2C, ID: e.UserID, User: e.UserID}
	case *MemberEvent:
		if e.Added {
			mm.EventId, mm.MessageId = e.EventID, e.MessageID
		}
		mm.Target = constant.Target{Scene: constant.SceneGroup, ID: e.GroupID, User: e.UserID}
	case *UnknownEvent:
		mm.Target = constant.Target{Scene: constant.SceneGroup, ID: e.GroupID, User: e.UserID}
		if e.GroupID == "" && e.UserID != "" {
			mm.Target = constant.Target{Scene: constant.SceneC2C, ID: e.UserID, User: e.UserID}
		}
	case *RobotEvent:
		mm.Target = constant.Target{Scene: constant.SceneGroup, ID: e.GroupID}
	case *AuditEvent:
		mm.Target = constant.Target{Scene: constant.SceneGroup}
	}
}
