package message

import (
	"encoding/json"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// AtMessage 艾特部件; All 为 true 时艾特所有人。AvatarURL 非空时会在艾特前垫一张圆形头像, 只在 markdown 消息里生效。
type AtMessage struct {
	*Message
	OpenID  string `json:"-"`
	All     bool   `json:"-"`
	Newline bool   `json:"-"`
	Quote   bool   `json:"-"`
	// AvatarURL 头像直链; AvatarPx 是客户端里显示的边长; Heading 是头像那行的标题级别(0 表示不套标题)。
	AvatarURL string `json:"-"`
	AvatarPx  int    `json:"-"`
	Heading   int    `json:"-"`
}

func (*AtMessage) part() {}

// Init 把消息类型钉成纯文本并登记序列化入口: 单独发一个艾特时也不该拼出没有 content 的报文。
func (msg *AtMessage) Init() {
	if msg.Message == nil {
		msg.Message = (&Message{}).InitRef()
	}
	msg.Message.Type = constant.PlainText
	msg.Message.MarshalInterface = msg
}

func (msg *AtMessage) Marshal() ([]byte, error) {
	content := "@everyone"
	if !msg.All {
		content = Mention(msg.Target.Scene, msg.OpenID)
	}
	type txt struct {
		*Message
		Content string `json:"content"`
	}
	return json.Marshal(txt{
		Message: msg.Message,
		Content: content,
	})
}

// Mention 把用户 ID 写成这个场景认的艾特语法: 频道用 <@!id>, 群与单聊用 <@openid>。
func Mention(scene constant.Scene, userID string) string {
	if scene.Guild() {
		return "<@!" + userID + ">"
	}
	return "<@" + userID + ">"
}

// ProtectMarkdownAt 在 markdown 中对裸 @ 加零宽空格 \u200b，防止被 QQ 误解析为艾特。
func ProtectMarkdownAt(s string) string {
	var buf strings.Builder
	buf.Grow(len(s) + len(s)/4)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '@' && i+1 < len(s) && !isSpace(s[i+1]) && (i == 0 || (!isWordChar(s[i-1]) && s[i-1] != '<')) {
			buf.WriteByte('@')
			buf.WriteString("\u200b")
			continue
		}
		buf.WriteByte(c)
	}
	return buf.String()
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '>'
}
