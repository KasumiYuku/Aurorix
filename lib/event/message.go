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
	// Searchable 是这条消息里所有可能有链接的文本(正文 / 卡片 / 各消息元素), 已去重,
	// 且每个来源都附了一份反转义后的副本 —— 直接拿去跑正则就行, 不必关心它原本放在哪。
	Searchable []string
	Raw        json.RawMessage
}

func (e *MessageEvent) Type() constant.EventType { return e.eventType }
