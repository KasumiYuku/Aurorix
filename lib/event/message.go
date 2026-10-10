package event

import (
	"encoding/json"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

// MessageEvent 群/私聊消息事件公共视图。Content 为清洗后文本, RawContent 为原文。
type MessageEvent struct {
	eventType   constant.EventType
	EventID     string
	MessageID   string
	GroupID     string
	UserID      string
	Origin      constant.MessageOrigin
	Content     string
	RawContent  string
	Mentions    []structers.Mention
	Attachments []message.Attachment
	Quote       *structers.Quote
	MsgType     int
	Ark         *structers.ArkData
	Elements    []structers.MsgElement
	Raw         json.RawMessage
}

func (e *MessageEvent) Type() constant.EventType { return e.eventType }
