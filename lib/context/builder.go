package context

import (
	"errors"
	"fmt"
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
	noFooter   bool
	imageOnly  bool
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

// AtAvatar 追加"带头像的艾特"部件（见 MessageManager.AtAvatar）。
func (b *MsgBuilder) AtAvatar(openid string, px ...int) *MsgBuilder {
	return b.Add(b.m.AtAvatar(openid, px...))
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

// NoFooter 让这条消息不带框架级底部按钮, 用于指令级豁免之外的逐条覆盖。
func (b *MsgBuilder) NoFooter() *MsgBuilder {
	b.noFooter = true
	return b
}

// ImageOnly 让这条链在频道里"有图就只发图"; 这一页没配图时照常发正文, 不会变成一条空消息。
func (b *MsgBuilder) ImageOnly() *MsgBuilder {
	b.imageOnly = true
	return b
}

// Send 分拣发送：文字/艾特/图床图/markdown 合成一条主消息
func (b *MsgBuilder) Send() error {
	_, err := b.SendWithID()
	return err
}

// SendWithID 同 Send, 额外返回主消息的平台 ID, 供撤回使用; 一条链可能落成好几条, 要拿全部 ID 的场合用 SendAllWithID。
func (b *MsgBuilder) SendWithID() (string, error) {
	ids, err := b.SendAllWithID()
	if len(ids) == 0 {
		return "", err
	}
	return ids[0], err
}

// SendAll 发送这条消息链, 返回全部 ID 与"这次是不是主动投递"; 结果同时记进 MessageManager.LatestDelivery。
func (b *MsgBuilder) SendAll() (message.Delivery, error) {
	if len(b.parts) == 0 && b.kb == nil {
		return message.Delivery{}, nil
	}
	out := b.plan()
	if b.initiative {
		markInitiative(out)
	}
	ids := make([]string, 0, len(out))
	active := false
	var errs []error
	for _, p := range out {
		id, err := p.SendWithID()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if id != "" {
			ids = append(ids, id)
		}
		if m, ok := p.(deliveredActive); ok && m.DeliveredActive() {
			active = true
		}
	}
	got := message.Delivery{IDs: ids, Active: active}
	b.m.noteDelivery(got)
	return got, errors.Join(errs...)
}

// SendAllWithID 只要 ID 的那一版。
func (b *MsgBuilder) SendAllWithID() ([]string, error) {
	got, err := b.SendAll()
	return got.IDs, err
}

// Supersede 发这条新消息, 然后把 olds 撤掉; 顺序、"主动投递不撤"与异步全在 message.Replace, 这里只负责把链交给它。返回全部平台 ID。
func (b *MsgBuilder) Supersede(olds ...message.Sent) ([]string, error) {
	got, err := message.Replace(b.m.Qapi, b.m.Target, olds, b.SendAll)
	return got.IDs, err
}

type messageInitiative interface{ SetInitiativeMessage() }

type deliveredActive interface{ DeliveredActive() bool }

func markInitiative(msgs []Sender) {
	for _, m := range msgs {
		if i, ok := m.(messageInitiative); ok {
			i.SetInitiativeMessage()
		}
	}
}

func (b *MsgBuilder) plan() []Sender {
	if b.m.Target.Scene.Guild() {
		return b.planGuild()
	}
	return b.planChat()
}

func (b *MsgBuilder) planChat() []Sender {
	host := b.m.Qapi.Assets
	globalMD := b.m.Qapi != nil && b.m.Qapi.GlobalMarkdown

	useMarkdown := b.kb != nil
	hasText, hasImage := false, false
	for _, p := range b.parts {
		switch v := p.(type) {
		case *message.MarkdownMessage:
			useMarkdown = true
		case *message.TextMessage:
			hasText = true
		case *message.ImageMessage:
			hasImage = true
		case *message.AtMessage:
			if v.AvatarURL != "" {
				useMarkdown = true
			}
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
				if useMarkdown {
					if v.Quote {
						inline.WriteString("> ")
					}
					if v.Heading > 0 {
						inline.WriteString(strings.Repeat("#", v.Heading))
						inline.WriteByte(' ')
					}
					if v.AvatarURL != "" {
						fmt.Fprintf(&inline, "![cornerRadius=100 #%dpx #%dpx](%s) ", v.AvatarPx, v.AvatarPx, v.AvatarURL)
					}
				}
				inline.WriteString(message.Mention(b.m.Target.Scene, v.OpenID))
			}
			if v.Newline {
				inline.WriteByte('\n')
			}
		case *message.MarkdownMessage:
			inline.WriteString(v.Markdown.Content)
		case *message.ImageMessage:
			if useMarkdown {
				r := imgSlots[next]
				if r.err == nil && r.frag != "" {
					inline.WriteString(strings.TrimRight(r.frag, "\n"))
					next++
					continue
				}
				next++
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
		if b.noFooter {
			mdMsg.NoFooter = true
		}
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

func (b *MsgBuilder) planGuild() []Sender {
	var inline strings.Builder
	var media []Sender
	for _, p := range b.parts {
		switch v := p.(type) {
		case *message.TextMessage:
			inline.WriteString(v.TextContent)
		case *message.MarkdownMessage:
			inline.WriteString(v.Markdown.Content)
		case *message.AtMessage:
			if v.All {
				inline.WriteString("@everyone")
			} else {
				inline.WriteString(message.Mention(b.m.Target.Scene, v.OpenID))
			}
			if v.Newline {
				inline.WriteByte('\n')
			}
		case *message.ImageMessage:
			media = append(media, v)
		case *message.UploadMessage:
			media = append(media, v)
		case *message.MediaMessage:
			media = append(media, v)
		}
	}

	urls, rest := message.SplitImages(inline.String())
	if len(urls) > 0 {
		imgs := make([]Sender, 0, len(urls))
		for _, u := range urls {
			imgs = append(imgs, b.m.Image(u, ""))
		}
		media = append(imgs, media...)
	}
	caption := message.GuildText(rest)
	if b.imageOnly && len(media) > 0 {
		caption = ""
	}
	if len(media) > 0 {
		if im, ok := media[0].(*message.ImageMessage); ok {
			im.Caption = caption
			caption = ""
		}
	}
	out := make([]Sender, 0, 1+len(media))
	if caption != "" {
		msg := b.m.Text(caption)
		if b.quote != "" {
			msg.QuoteTo(b.quote)
		}
		out = append(out, msg)
	}
	return append(out, media...)
}

type imgResult struct {
	frag string
	err  error
}
