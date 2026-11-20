package message

import (
	"fmt"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/images"
	"github.com/KasumiYuku/Aurorix/lib/templates"
)

// ImageMessage 图片部件。
type ImageMessage struct {
	*Message
	Src           any    `json:"-"`
	Summary       string `json:"-"`
	Width         int    `json:"-"`
	Height        int    `json:"-"`
	dimsSet       bool
	ForceMarkdown bool `json:"-"`
	ForceMedia    bool `json:"-"`
	// Caption 频道域这条图片消息一并带上的正文; 群与单聊不用, 那边正文和图本来就在同一条 markdown 里。
	Caption string `json:"-"`
}

func (*ImageMessage) part() {}

// Send 发送：markdown 内嵌优先，失败降级独立媒体。
func (msg *ImageMessage) Send() error {
	_, err := msg.SendWithID()
	return err
}

// SendWithID 同 Send, 额外返回平台消息 ID; 频道域用平台自己的图片字段, 群与单聊走 markdown 内嵌, 不行再上传。
func (msg *ImageMessage) SendWithID() (string, error) {
	if msg.Target.Scene.Guild() {
		img, err := msg.guildImage()
		if err != nil {
			return "", err
		}
		msg.Message.guildImage = &img
		id, err := msg.Message.SendWithID()
		if err == nil || img.URL == "" {
			return id, err
		}
		retry, ferr := msg.guildImageRetry()
		if ferr != nil {
			logger.Warnf("频道发图: 平台转存失败(%v), 本地抓取也失败: %v", err, ferr)
			return "", err
		}
		logger.Warnf("频道发图: 平台转存失败(%v), 改用字节直传重试", err)
		msg.Message.guildImage = &retry
		return msg.Message.SendWithID()
	}
	if !msg.ForceMedia {
		useMD := msg.ForceMarkdown || (msg.Qapi != nil && msg.Qapi.GlobalMarkdown)
		if useMD && msg.Qapi != nil {
			if frag, err := msg.Fragment(msg.Qapi.Assets); err == nil && frag != "" {
				md := &MarkdownMessage{Message: msg.Message, Markdown: templates.Markdown{Content: frag}}
				md.Init()
				return md.SendWithID()
			}
		}
	}
	return msg.sendMediaWithID()
}

func (msg *ImageMessage) guildImage() (api.GuildImage, error) {
	img := api.GuildImage{Content: msg.guildCaption()}
	if s, ok := msg.Src.(string); ok && isPublicURL(s) {
		img.URL = s
		return img, nil
	}
	in, err := assets.DecodeImage(msg.Src)
	if err != nil {
		return api.GuildImage{}, err
	}
	if in.URL != "" {
		img.URL = in.URL
		return img, nil
	}
	fillGuildBytes(&img, in.Data, in.Filename)
	return img, nil
}

func (msg *ImageMessage) guildImageRetry() (api.GuildImage, error) {
	img := api.GuildImage{Content: msg.guildCaption()}
	in, err := assets.DecodeImage(msg.Src)
	if err != nil {
		return api.GuildImage{}, err
	}
	data := in.Data
	if in.URL != "" {
		buf, ferr := images.Fetch(in.URL)
		if ferr != nil {
			return api.GuildImage{}, ferr
		}
		if !images.IsImage(buf) {
			return api.GuildImage{}, fmt.Errorf("抓回来的不是图片")
		}
		data = buf
	}
	fillGuildBytes(&img, data, in.Filename)
	return img, nil
}

func (msg *ImageMessage) guildCaption() string {
	if msg.Caption != "" {
		return msg.Caption
	}
	return msg.Summary
}

func fillGuildBytes(img *api.GuildImage, data []byte, filename string) {
	img.Data = data
	img.MimeType = images.ProbeMime(data)
	img.Filename = filename
	if img.Filename == "" || !hasExt(img.Filename) {
		img.Filename = "image." + images.ExtFromMime(img.MimeType)
	}
}

// Fragment 解析为 markdown 图片语法；公网 URL 直通，其余走图床。
func (msg *ImageMessage) Fragment(host *assets.ImageHost) (string, error) {
	summary := msg.Summary
	if summary == "" {
		summary = "图片"
	}
	dims := msg.dimsMarkup()

	if s, ok := msg.Src.(string); ok && isPublicURL(s) {
		if dims == "" {
			dims = probeDims(s)
		}
		return fmt.Sprintf("![%s%s](%s)\n", summary, dims, s), nil
	}

	if host == nil || host.Size() == 0 {
		return "", fmt.Errorf("图床未配置且图片非公网 URL")
	}
	resolved, err := host.ResolveTo(msg.Src, msg.Target)
	if err != nil || resolved.URL == "" {
		return "", fmt.Errorf("图床转换失败: %v", err)
	}
	if dims == "" && resolved.Width > 0 && resolved.Height > 0 {
		dims = fmt.Sprintf(" #%dpx #%dpx", resolved.Width, resolved.Height)
	}
	return fmt.Sprintf("![%s%s](%s)\n", summary, dims, resolved.URL), nil
}

func isPublicURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func hasExt(name string) bool {
	return strings.LastIndex(name, ".") > strings.LastIndex(name, "/")
}

func (msg *ImageMessage) dimsMarkup() string {
	if !msg.dimsSet {
		return ""
	}
	return fmt.Sprintf(" #%dpx #%dpx", msg.Width, msg.Height)
}

// Size 显式指定显示尺寸，返回自身便于链式调用。
func (msg *ImageMessage) Size(width, height int) *ImageMessage {
	msg.Width, msg.Height, msg.dimsSet = width, height, true
	return msg
}

func probeDims(url string) string {
	w, h, err := images.GetImageDimensions(url)
	if err != nil || w <= 0 || h <= 0 {
		return ""
	}
	return fmt.Sprintf(" #%dpx #%dpx", w, h)
}

func (msg *ImageMessage) sendMediaWithID() (string, error) {
	up := MediaUploadFor(1, msg.Src, msg.Summary)
	fileInfo, err := msg.Qapi.Session(msg.Target).Upload(up)
	if err != nil {
		return "", err
	}
	media := &MediaMessage{Message: msg.Message, Media: MediaContent{FileInfo: fileInfo}}
	media.Init()
	return media.SendWithID()
}
