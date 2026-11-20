package message

import (
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/templates"
)

type TextMessage struct {
	*Message
	TextContent  string `json:"content"`
	MarkdownMode bool   `json:"-"`
}

func (msg *TextMessage) Content(content string) *TextMessage {
	msg.TextContent = content
	return msg
}

// Marshal 序列化。MarkdownMode 是全局 markdown 开关的产物, 频道域没有 markdown 通道, 这个开关在那里不生效。
func (msg *TextMessage) Marshal() ([]byte, error) {
	if msg.MarkdownMode && !msg.Target.Scene.Guild() {
		content := ProtectMarkdownAt(msg.TextContent)
		if msg.Qapi != nil && msg.Qapi.Assets != nil {
			content = msg.Qapi.Assets.ProcessMarkdownTo(content, msg.Target)
		}
		content = msg.fitImages(content)
		type mdMsg struct {
			*Message
			Type     constant.MessageType `json:"msg_type"`
			Markdown templates.Markdown   `json:"markdown"`
		}
		return json.Marshal(mdMsg{
			Message:  msg.Message,
			Type:     constant.Markdown,
			Markdown: templates.Markdown{Content: content},
		})
	}
	return json.Marshal(msg)
}

func (*TextMessage) part() {}

// Init 初始化 Message 结构体。
func (msg *TextMessage) Init() {
	var metamsg *Message
	if msg.Message == nil {
		metamsg = &Message{}
		metamsg.InitRef()
	} else {
		metamsg = msg.Message
	}
	metamsg.MarshalInterface = msg
	msg.Message = metamsg
}

func NewTextMessage() *TextMessage {
	var msg *TextMessage = &TextMessage{}
	msg.Init()
	return msg
}
