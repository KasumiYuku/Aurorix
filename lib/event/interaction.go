package event

import "github.com/KasumiYuku/Aurorix/lib/constant"

// InteractionEvent 按钮交互事件。
type InteractionEvent struct {
	EventID   string
	ButtonID  string
	Data      string
	GroupID   string // 私聊场景为空
	UserID    string
	MessageID string
}

func (e *InteractionEvent) Type() constant.EventType { return constant.INTERACTION_CREATE }

// JoinRequestEvent 入群申请事件。
type JoinRequestEvent struct {
	RequestID string
	GroupID   string
	UserID    string
	Method    string // verify_message | admin_review_qa
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
