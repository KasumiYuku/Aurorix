package message

import (
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/images"
	"github.com/KasumiYuku/Aurorix/lib/templates"
	"strings"
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
}

func (*ImageMessage) part() {}

// Send 发送：markdown 内嵌优先，失败降级独立媒体。
func (msg *ImageMessage) Send() error {
	_, err := msg.SendWithID()
	return err
}

// SendWithID 同 Send, 额外返回平台消息 ID。
func (msg *ImageMessage) SendWithID() (string, error) {
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
	resolved, err := host.ResolveTo(msg.Src, msg.GroupId, msg.UserId)
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

func (msg *ImageMessage) sendMedia() error {
	_, err := msg.sendMediaWithID()
	return err
}

func (msg *ImageMessage) sendMediaWithID() (string, error) {
	up := MediaUploadFor(1, msg.Src, msg.Summary)
	fileInfo, err := msg.Qapi.UploadMedia(msg.Target, msg.GroupId, msg.UserId, up)
	if err != nil {
		return "", err
	}
	media := &MediaMessage{Message: msg.Message, Media: MediaContent{FileInfo: fileInfo}}
	media.Init()
	return media.SendWithID()
}
