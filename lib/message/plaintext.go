package message

import (
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/templates"
)

type TextMessage struct {
	*Message
	TextContent string `json:"content"`
	// MarkdownMode 全局 markdown 开启时，文本以 markdown 消息类型发送。
	MarkdownMode bool `json:"-"`
}

// 设置内容
func (msg *TextMessage) Content(content string) *TextMessage {
	msg.TextContent = content
	return msg
}

// 实现CanMarshal
func (msg *TextMessage) Marshal() ([]byte, error) {
	if msg.MarkdownMode {
		content := ProtectMarkdownAt(msg.TextContent)
		// globalMarkdown 模式下文本中的图片同样过图床（与 MarkdownMessage 一致）
		if msg.Qapi != nil && msg.Qapi.Assets != nil {
			content = msg.Qapi.Assets.ProcessMarkdown(content)
		}
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
