package context

import (
	"errors"
	"github.com/KasumiYuku/Aurorix/lib/contract"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"strings"
	"sync"
)

// MsgBuilder 消息链：链式拼实体部件，Send 时分拣合并发送。
type MsgBuilder struct {
	m          *MessageManager
	parts      []message.Part
	kb         contract.CanMarshal
	quote      string
	initiative bool
}

// Msg 开启一条消息链。
func (manager *MessageManager) Msg() *MsgBuilder {
	return &MsgBuilder{m: manager}
}

// Add 追加消息部件（ctx.Text/Markdown/Image 等构造的实体）。
func (b *MsgBuilder) Add(parts ...message.Part) *MsgBuilder {
	b.parts = append(b.parts, parts...)
	return b
}

func (b *MsgBuilder) Text(s string) *MsgBuilder {
	return b.Add(b.m.Text(s))
}

func (b *MsgBuilder) At(openid string, newline ...bool) *MsgBuilder {
	return b.Add(b.m.At(openid, newline...))
}

// AtQuote 追加"引用块里的艾特"部件（见 MessageManager.AtQuote）。
func (b *MsgBuilder) AtQuote(openid string, newline ...bool) *MsgBuilder {
	return b.Add(b.m.AtQuote(openid, newline...))
}

func (b *MsgBuilder) AtAll(newline ...bool) *MsgBuilder {
	return b.Add(b.m.AtAll(newline...))
}

func (b *MsgBuilder) Markdown(s string) *MsgBuilder {
	return b.Add(b.m.Markdown(s))
}

// Image 追加图片；src 支持 string(路径/data/base64/公网URL) 与 []byte。
func (b *MsgBuilder) Image(src any, summary string, size ...int) *MsgBuilder {
	return b.Add(b.m.Image(src, summary, size...))
}

func (b *MsgBuilder) Voice(src any) *MsgBuilder {
	return b.Add(b.m.Voice(src))
}

func (b *MsgBuilder) Video(src any) *MsgBuilder {
	return b.Add(b.m.Video(src))
}

func (b *MsgBuilder) File(src any, name string) *MsgBuilder {
	return b.Add(b.m.File(src, name))
}

// Keyboard 挂按钮板，仅随 markdown 消息发送。
func (b *MsgBuilder) Keyboard(kb contract.CanMarshal) *MsgBuilder {
	b.kb = kb
	return b
}

// Quote 引用回复指定消息。
func (b *MsgBuilder) Quote(messageID string) *MsgBuilder {
	b.quote = messageID
	return b
}

// Initiative 标记本次发送为主动推送。
func (b *MsgBuilder) Initiative() *MsgBuilder {
	b.initiative = true
	return b
}

// Send 分拣发送：文字/艾特/图床图/markdown 合成一条主消息
func (b *MsgBuilder) Send() error {
	_, err := b.SendWithID()
	return err
}

// SendWithID 同 Send，额外返回主消息（发送列表第一条）的平台 ID，供撤回使用；
func (b *MsgBuilder) SendWithID() (string, error) {
	if len(b.parts) == 0 && b.kb == nil {
		return "", nil
	}
	out := b.plan()
	if b.initiative {
		markInitiative(out)
	}
	mainID := ""
	var errs []error
	for i, p := range out {
		id, err := p.SendWithID()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if i == 0 {
			mainID = id
		}
	}
	return mainID, errors.Join(errs...)
}

type messageInitiative interface{ SetInitiativeMessage() }

func markInitiative(msgs []Sender) {
	for _, m := range msgs {
		if i, ok := m.(messageInitiative); ok {
			i.SetInitiativeMessage()
		}
	}
}

func (b *MsgBuilder) plan() []Sender {
	host := b.m.Qapi.Assets
	globalMD := b.m.Qapi != nil && b.m.Qapi.GlobalMarkdown

	useMarkdown := b.kb != nil
	hasText, hasImage := false, false
	for _, p := range b.parts {
		switch p.(type) {
		case *message.MarkdownMessage:
			useMarkdown = true
		case *message.TextMessage:
			hasText = true
		case *message.ImageMessage:
			hasImage = true
		}
	}
	if globalMD && (hasText || hasImage) {
		useMarkdown = true
	}

	var imgSlots []imgResult
	var imgPos []int
	if useMarkdown {
		for i, p := range b.parts {
			if _, ok := p.(*message.ImageMessage); ok {
				imgPos = append(imgPos, i)
				imgSlots = append(imgSlots, imgResult{})
			}
		}
		if len(imgSlots) > 0 {
			var wg sync.WaitGroup
			wg.Add(len(imgSlots))
			for n, i := range imgPos {
				im := b.parts[i].(*message.ImageMessage)
				go func(n int, im *message.ImageMessage) {
					defer wg.Done()
					imgSlots[n].frag, imgSlots[n].err = im.Fragment(host)
				}(n, im)
			}
			wg.Wait()
		}
	}

	var inline strings.Builder
	var media []Sender

	next := 0
	for _, p := range b.parts {
		switch v := p.(type) {
		case *message.TextMessage:
			inline.WriteString(v.TextContent)
		case *message.AtMessage:
			if v.All {
				if useMarkdown {
					inline.WriteString("<qqbot-at-everyone />")
				} else {
					inline.WriteString("@everyone")
				}
			} else {
				if v.Quote && useMarkdown {
					inline.WriteString("> ")
				}
				inline.WriteString("<@")
				inline.WriteString(v.OpenID)
				inline.WriteString(">")
			}
			if v.Newline {
				inline.WriteByte('\n')
			}
		case *message.MarkdownMessage:
			inline.WriteString(v.Markdown.Content)
		case *message.ImageMessage:
			if useMarkdown {
				r := imgSlots[next]
				next++
				if r.err == nil && r.frag != "" {
					inline.WriteByte('\n')
					inline.WriteString(strings.TrimRight(r.frag, "\n"))
					inline.WriteString("\n\n")
					continue
				}
				v.ForceMarkdown = false
				v.ForceMedia = true
			}
			media = append(media, v)
		case *message.UploadMessage:
			media = append(media, v)
		case *message.MediaMessage:
			media = append(media, v)
		}
	}

	out := make([]Sender, 0, 1+len(media))
	switch {
	case useMarkdown && (inline.Len() > 0 || b.kb != nil):
		content := inline.String()
		if strings.TrimSpace(content) == "" {
			content = " "
		}
		mdMsg := b.m.Markdown(content)
		if b.kb != nil {
			mdMsg.Keyboard(b.kb)
		}
		if b.quote != "" {
			mdMsg.QuoteTo(b.quote)
		}
		out = append(out, mdMsg)
	case inline.Len() > 0:
		msg := b.m.Text(inline.String())
		if b.quote != "" {
			msg.QuoteTo(b.quote)
		}
		out = append(out, msg)
	}
	out = append(out, media...)
	return out
}

type imgResult struct {
	frag string
	err  error
}
