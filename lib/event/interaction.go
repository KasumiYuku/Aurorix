package event

import (
	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// InteractionEvent 按钮交互事件。
type InteractionEvent struct {
	EventID   string
	ButtonID  string
	Data      string
	MessageID string
	// Target 交互发生在哪个会话; 频道里的按钮带 channel_id, 群/单聊带 openid。
	Target constant.Target
}

func (e *InteractionEvent) Type() constant.EventType { return constant.INTERACTION_CREATE }

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
