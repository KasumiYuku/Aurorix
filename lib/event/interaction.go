package event

import (
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

// InteractionEvent 按钮交互事件。
type InteractionEvent struct {
	EventID   string
	ButtonID  string
	Data      string
	GroupID   string
	UserID    string
	MessageID string
	Scene     string
	ChatType  constant.ChatType
}

func (e *InteractionEvent) Type() constant.EventType { return constant.INTERACTION_CREATE }

// IsPrivate 交互是否发生在单聊会话。
func (e *InteractionEvent) IsPrivate() bool {
	return structers.InteractionIsPrivate(e.ChatType, e.Scene)
}

// JoinRequestEvent 入群申请事件。
type JoinRequestEvent struct {
	RequestID string
	GroupID   string
	UserID    string
	Method    string
	Answer    string
	MessageID string
}

func (e *JoinRequestEvent) Type() constant.EventType { return constant.GROUP_JOIN_REQUEST }

// AuditEvent 消息审核结果事件。
type AuditEvent struct {
	AuditID   string
	MessageID string
	Approved  bool
}

func (e *AuditEvent) Type() constant.EventType {
	if e.Approved {
		return constant.MESSAGE_AUDIT_PASS
	}
	return constant.MESSAGE_AUDIT_REJECT
}
