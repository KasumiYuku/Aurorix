package event

import (
	"encoding/json"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

// MessageEvent 收到一条消息的公共视图。Content 为清洗后文本, RawContent 为原文。
type MessageEvent struct {
	eventType constant.EventType
	EventID   string
	MessageID string
	// Target 这条消息来自哪个会话: 场景 + 会话 ID + 触发者。
	Target constant.Target
	// Mentioned 正文里点名了机器人。频道私域能收到全部消息, 只有被点名的才该当指令处理。
	Mentioned   bool
	Username    string
	Content     string
	RawContent  string
	Mentions    []structers.Mention
	Attachments []message.Attachment
	Quote       *structers.Quote
	MsgType     int
	Ark         *structers.ArkData
	Elements    []structers.MsgElement
	// Searchable 这条消息里所有可能有链接的文本(正文/卡片/各消息元素), 已去重, 每个来源都附一份反转义副本。
	Searchable []string
	Raw        json.RawMessage
}

func (e *MessageEvent) Type() constant.EventType { return e.eventType }

// GuildMemberEvent 频道成员变动。
type GuildMemberEvent struct {
	eventType constant.EventType
	GuildID   string
	UserID    string
	Username  string
	Nick      string
	Added     bool
}

func (e *GuildMemberEvent) Type() constant.EventType { return e.eventType }

// GuildEvent 频道本身或子频道的新增/变更/删除。
type GuildEvent struct {
	eventType  constant.EventType
	GuildID    string
	ChannelID  string
	Name       string
	OperatorID string
}

func (e *GuildEvent) Type() constant.EventType { return e.eventType }
