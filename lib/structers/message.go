package structers

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/message"
)

// Mention 消息中的 @ 提及。
type Mention struct {
	Scope  string `json:"scope"`
	ID     string `json:"id,omitempty"`
	IsYou  bool   `json:"is_you,omitempty"`
	Member string `json:"member_openid,omitempty"`
}

// MsgElement 消息元素。
type MsgElement struct {
	MsgIdx      string               `json:"msg_idx,omitempty"`
	Author      *MsgAuthor           `json:"author,omitempty"`
	Content     string               `json:"content,omitempty"`
	MessageType int                  `json:"message_type,omitempty"`
	Attachments []message.Attachment `json:"attachments,omitempty"`
	ArkData     *ArkData             `json:"ark_data,omitempty"`
	MsgElements []MsgElement         `json:"msg_elements,omitempty"`
}

// MentionsYou 这条消息是否点名了机器人。
func MentionsYou(list []Mention) bool {
	for _, m := range list {
		if m.IsYou {
			return true
		}
	}
	return false
}

// StripMentions 去掉点名机器人的艾特标记(群写 <@openid>, 频道写 <@!user_id>); 不剥掉的话指令解析拿到的第一个词永远是它。
func StripMentions(content string, list []Mention) string {
	for _, m := range list {
		if !m.IsYou {
			continue
		}
		if m.ID == "" {
			content = stripLeadingMention(content)
			continue
		}
		content = strings.ReplaceAll(content, "<@!"+m.ID+">", "")
		content = strings.ReplaceAll(content, "<@"+m.ID+">", "")
	}
	return strings.TrimSpace(content)
}

// MentionsUser 正文是否点了指定的某个用户。平台没给 is_you 时靠这个兜底(频道里有这种载荷)。
func MentionsUser(list []Mention, userID string) bool {
	if userID == "" {
		return false
	}
	for _, m := range list {
		if m.ID == userID {
			return true
		}
	}
	return false
}

// StripLeadingMention 去掉正文开头那一个艾特标记。频道公域 @ 事件里它就是机器人。
func StripLeadingMention(content string) string {
	return strings.TrimSpace(stripLeadingMention(content))
}

func stripLeadingMention(content string) string {
	if !strings.HasPrefix(content, "<@") {
		return content
	}
	if i := strings.IndexByte(content, '>'); i >= 0 {
		return content[i+1:]
	}
	return content
}

// FlattenElements 递归展开消息元素, 按出现顺序返回(含嵌套的全部层级)。
func FlattenElements(elements []MsgElement) []MsgElement {
	out := make([]MsgElement, 0, len(elements))
	var walk func([]MsgElement)
	walk = func(list []MsgElement) {
		for _, el := range list {
			out = append(out, el)
			walk(el.MsgElements)
		}
	}
	walk(elements)
	return out
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
	// Attachments 是引用那条消息里的附件(图/视频/文件), 光有 Content 时是一串空的占位。
	Attachments []message.Attachment
	// MsgIdx 是被引用那条消息在 msg_elements 里的编号, 想再查别的字段时靠它对号。
	MsgIdx string
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

// MsgIdxFromScene 从 message_scene.ext 的 "key=value" 串里取出某一项的值; 引用一条消息时会多一项 ref_msg_idx。
func MsgIdxFromScene(ext []string, key string) string {
	prefix := key + "="
	for _, item := range ext {
		if value, ok := strings.CutPrefix(item, prefix); ok {
			return value
		}
	}
	return ""
}

// QuoteElementType 是 msg_elements 里"引用消息"那一类的 message_type; 同类还有 0 文本、3 结构化卡片、101 并行消息、102 合并转发。
const QuoteElementType = 103

// ExtractQuote 从 msg_elements 里挑出被引用的那条: 不能按位置取 [0](那正是合并转发的排法), 顺序是 ref_msg_idx → message_type=103 → 数组恰好一条。
func ExtractQuote(elements []MsgElement, sceneExt []string) *Quote {
	if len(elements) == 0 {
		return nil
	}
	for _, key := range []string{"ref_msg_idx", "msg_idx"} {
		if want := MsgIdxFromScene(sceneExt, key); want != "" {
			for _, el := range FlattenElements(elements) {
				if el.MsgIdx == want {
					return quoteFrom(el)
				}
			}
		}
	}
	for _, el := range FlattenElements(elements) {
		if el.MessageType == QuoteElementType {
			return quoteFrom(el)
		}
	}
	if len(elements) == 1 {
		return quoteFrom(elements[0])
	}
	return nil
}

func quoteFrom(el MsgElement) *Quote {
	quote := &Quote{Content: el.Content, Attachments: el.Attachments, MsgIdx: el.MsgIdx}
	if el.Author != nil {
		quote.Author = el.Author.Username
	}
	if quote.Content == "" && len(quote.Attachments) == 0 {
		for _, nested := range el.MsgElements {
			inner := quoteFrom(nested)
			if inner == nil {
				continue
			}
			if quote.Author == "" {
				quote.Author = inner.Author
			}
			quote.Content = inner.Content
			quote.Attachments = inner.Attachments
			break
		}
	}
	if quote.Content == "" && len(quote.Attachments) == 0 && quote.Author == "" {
		return nil
	}
	return quote
}

// DecodeEmoji 将内容里的 <faceType...> 表情标记解码为可见文本。
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
	return AvatarURLSize(appID, openID, 640)
}

// AvatarURLSize 生成指定档位的 QQ 用户头像 URL。
func AvatarURLSize(appID, openID string, size int) string {
	if size <= 0 {
		size = 640
	}
	return "https://thirdqq.qlogo.cn/qqapp/" + appID + "/" + openID + "/" + strconv.Itoa(size)
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
