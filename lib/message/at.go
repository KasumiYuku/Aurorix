package message

import (
	"encoding/json"
	"strings"
)

// AtMessage 艾特部件：All 为 true 时艾特所有人。
type AtMessage struct {
	*Message
	OpenID  string `json:"-"`
	All     bool   `json:"-"`
	Newline bool   `json:"-"`
	Quote   bool   `json:"-"`
}

func (*AtMessage) part() {}

func (msg *AtMessage) Marshal() ([]byte, error) {
	content := "@everyone"
	if !msg.All {
		content = "<@" + msg.OpenID + ">"
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
