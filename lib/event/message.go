package event

import (
	"Plrx/lib/constant"
	"Plrx/lib/message"
	"Plrx/lib/structers"
)

// MessageEvent 群/私聊消息事件公共视图。Content 为清洗后文本, RawContent 为原文。
type MessageEvent struct {
	eventType   constant.EventType
	EventID     string
	MessageID   string
	GroupID     string // 私聊为空
	UserID      string
	Origin      constant.MessageOrigin
	Content     string
	RawContent  string
	Mentions    []structers.Mention
	Attachments []message.Attachment
	Quote       *structers.Quote
	AvatarURL   string
}

func (e *MessageEvent) Type() constant.EventType { return e.eventType }
