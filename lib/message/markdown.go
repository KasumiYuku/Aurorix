package message

import (
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/constant"
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

func (msg *MarkdownMessage) Marshal() ([]byte, error) {
	if msg.Target.Scene.Guild() {
		return msg.marshalGuild()
	}
	originalInput := ""
	skipFooter := false
	if msg.Message != nil {
		originalInput = msg.OriginalInput
		skipFooter = msg.NoFooter
	}
	keyboard := msg.keyboard
	if !skipFooter {
		keyboard = applyFooter(msg.keyboard, originalInput, msg.Target.Scene)
	}
	if keyboard != nil {
		keyboardData, err := keyboard.Marshal()
		if err != nil {
			return nil, &JSONMarshalError{Err: err}
		}
		msg.KeyboardRaw = keyboardData
	}
	return json.Marshal(msg)
}

func (msg *MarkdownMessage) marshalGuild() ([]byte, error) {
	_, rest := SplitImages(msg.Markdown.Content)
	plain := &TextMessage{Message: msg.Message, TextContent: GuildText(rest)}
	plain.Type = constant.PlainText
	plain.MarkdownMode = false
	return json.Marshal(plain)
}

// SetAssets 注入图床聚合器，Send 时自动处理图片内嵌。
func (msg *MarkdownMessage) SetAssets(host *assets.ImageHost) {
	msg.assets = host
}

func (msg *MarkdownMessage) Content(content string) {
	msg.Markdown = templates.Markdown{
		Content: content,
	}
}

func (msg *MarkdownMessage) Keyboard(keyboard contract.CanMarshal) {
	msg.keyboard = keyboard
}

// Init 初始化消息结构体并做 markdown 预处理; 消息类型在这里钉成 markdown, 漏了就是一份"msg_type=0 却没有 content"的报文。
func (msg *MarkdownMessage) Init() {
	var metamsg *Message
	if msg.Message == nil {
		metamsg = &Message{}
		metamsg.InitRef()
	} else {
		metamsg = msg.Message
	}
	metamsg.MarshalInterface = msg
	metamsg.Type = constant.Markdown
	msg.Message = metamsg
}

func (*MarkdownMessage) part() {}

// Prepare 实现 contract.PreSend: 发送前处理 @ 保护、图片内嵌与指令标签编码; 重试也会走, 每步必须幂等。
func (msg *MarkdownMessage) Prepare() {
	content := ProtectMarkdownAt(msg.Markdown.Content)
	if msg.assets != nil {
		content = msg.assets.ProcessMarkdownWith(content, assets.UploadOptions{
			Target:  msg.Target,
			Trace:   msg.Trace,
			Command: msg.Command,
		})
	}
	content = msg.fitImages(content)
	msg.Markdown.Content = encodeCmdTagAttrs(content)
}
