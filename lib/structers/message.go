package structers

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/message"
)

// Mention 消息中的 @ 提及。
type Mention struct {
	Scope  string `json:"scope"`            // single | all
	ID     string `json:"id,omitempty"`     // 被提及者 openid
	IsYou  bool   `json:"is_you,omitempty"` // 是否提及机器人
	Member string `json:"member_openid,omitempty"`
}

// MsgElement 消息元素（引用消息时 [0] 为被引用的原始消息）。
type MsgElement struct {
	Author      *MsgAuthor           `json:"author,omitempty"`
	Content     string               `json:"content,omitempty"`
	MessageType int                  `json:"message_type,omitempty"`
	Attachments []message.Attachment `json:"attachments,omitempty"`
}

// MsgAuthor 引用消息作者。
type MsgAuthor struct {
	Username string `json:"username,omitempty"`
	ID       string `json:"id,omitempty"`
}

// Quote 引用消息信息。
type Quote struct {
	Author  string
	Content string
}

// ParseMentions 从 JSON 原始提及数组解析为结构化 Mentions。
func ParseMentions(raw json.RawMessage) []Mention {
	if len(raw) == 0 {
		return nil
	}
	var mentions []Mention
	if err := json.Unmarshal(raw, &mentions); err != nil {
		return nil
	}
	return mentions
}

// ExtractQuote 从 msg_elements 提取引用消息（[0] 为原始消息）。
func ExtractQuote(elements []MsgElement) *Quote {
	if len(elements) == 0 {
		return nil
	}
	first := elements[0]
	if first.Content == "" && len(first.Attachments) == 0 {
		return nil
	}
	q := &Quote{Content: first.Content}
	if first.Author != nil {
		q.Author = first.Author.Username
	}
	return q
}

// DecodeEmoji 将内容里的 <faceType...> 表情标记解码为可见文本。
// 返回解码后的内容与表情文本列表。
func DecodeEmoji(content string) (string, []string) {
	var emojis []string
	var b strings.Builder
	b.Grow(len(content))
	last := 0
	for i := 0; i < len(content); i++ {
		if content[i] != '<' {
			continue
		}
		end := strings.IndexByte(content[i:], '>')
		if end < 0 {
			continue
		}
		tag := content[i : i+end+1]
		if !strings.HasPrefix(tag, "<faceType=") {
			continue
		}
		b.WriteString(content[last:i])
		if faceName := decodeFaceText(tag); faceName != "" {
			emojis = append(emojis, faceName)
			b.WriteString(faceName)
		}
		last = i + end + 1
	}
	if last == 0 {
		return content, nil
	}
	b.WriteString(content[last:])
	return b.String(), emojis
}

func decodeFaceText(tag string) string {
	const prefix = `ext="`
	idx := strings.Index(tag, prefix)
	if idx < 0 {
		return ""
	}
	rest := tag[idx+len(prefix):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(rest[:end])
	if err != nil {
		return ""
	}
	var decoded struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &decoded) != nil || decoded.Text == "" {
		return ""
	}
	return decoded.Text
}

// AvatarURL 生成 QQ 用户头像 URL。
func AvatarURL(appID, openID string) string {
	return "https://q.qlogo.cn/qqapp/" + appID + "/" + openID + "/640"
}

// ClassifyAttachment 按 content_type 分类附件，返回分类名。
const (
	AttachImage = "image"
	AttachVideo = "video"
	AttachAudio = "audio"
	AttachFile  = "file"
	AttachOther = "other"
)

func ClassifyAttachment(ct string) string {
	if ct == "" {
		return AttachOther
	}
	switch {
	case strings.HasPrefix(ct, "image/"):
		return AttachImage
	case strings.HasPrefix(ct, "video/"):
		return AttachVideo
	case ct == "voice":
		return AttachAudio
	case strings.HasPrefix(ct, "audio/"):
		return AttachAudio
	case ct == "file":
		return AttachFile
	default:
		return AttachOther
	}
}
