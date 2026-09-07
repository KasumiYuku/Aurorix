package message

import (
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/contract"
	"github.com/KasumiYuku/Aurorix/lib/templates"
)

// MarkdownMessage 支持图片内嵌与按钮。
type MarkdownMessage struct {
	*Message
	Markdown    templates.Markdown  `json:"markdown"`
	KeyboardRaw json.RawMessage     `json:"keyboard,omitempty"`
	keyboard    contract.CanMarshal `json:"-"`
	assets      *assets.ImageHost   `json:"-"`
}

// 实现CanMarshal接口
func (msg *MarkdownMessage) Marshal() ([]byte, error) {
	if msg.keyboard != nil {
		keyboardData, err := msg.keyboard.Marshal()
		if err != nil {
			return nil, &JSONMarshalError{Err: err}
		}
		msg.KeyboardRaw = keyboardData
	}
	return json.Marshal(msg)
}

// SetAssets 注入图床聚合器，Send 时自动处理图片内嵌。
func (msg *MarkdownMessage) SetAssets(host *assets.ImageHost) {
	msg.assets = host
}

// 设置内容
func (msg *MarkdownMessage) Content(content string) {
	msg.Markdown = templates.Markdown{
		Content: content,
	}
}

// 设置按钮板
func (msg *MarkdownMessage) Keyboard(keyboard contract.CanMarshal) {
	msg.keyboard = keyboard
}

// Init 初始化消息结构体并做 markdown 预处理。
func (msg *MarkdownMessage) Init() {
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

func (*MarkdownMessage) part() {}

// Prepare 实现 contract.PreSend：发送前处理 @ 保护 + 图片内嵌（若有图床）。
func (msg *MarkdownMessage) Prepare() {
	content := ProtectMarkdownAt(msg.Markdown.Content)
	if msg.assets != nil {
		content = msg.assets.ProcessMarkdown(content)
	}
	msg.Markdown.Content = content
}
