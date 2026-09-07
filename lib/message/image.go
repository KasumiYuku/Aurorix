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
	Src     any    `json:"-"` // string(路径/data/base64/URL) 或 []byte
	Summary string `json:"-"`
	// Width/Height 显式尺寸，零值表示未指定；发送时优先生效，其次图床探测，最后 URL 探测。
	Width  int `json:"-"`
	Height int `json:"-"`
	// ForceMarkdown 由构造器按上下文设置：true 强制内嵌 markdown。
	ForceMarkdown bool `json:"-"`
	// ForceMedia 由构造器在降级路径设置：true 跳过 markdown 直接走 QQ 媒体。
	ForceMedia bool `json:"-"`
}

func (*ImageMessage) part() {}

// Send 发送：markdown 内嵌优先，失败降级独立媒体。
func (msg *ImageMessage) Send() error {
	if !msg.ForceMedia {
		useMD := msg.ForceMarkdown || (msg.Qapi != nil && msg.Qapi.GlobalMarkdown)
		if useMD && msg.Qapi != nil {
			if frag, err := msg.Fragment(msg.Qapi.Assets); err == nil && frag != "" {
				md := &MarkdownMessage{Message: msg.Message, Markdown: templates.Markdown{Content: frag}}
				md.Init()
				return md.Send()
			}
		}
	}
	return msg.sendMedia()
}

// Fragment 解析为 markdown 图片语法；公网 URL 直通，其余走图床。
// 尺寸标注优先级：显式 Width/Height > URL 自动探测 > 图床探测 > 不带。
func (msg *ImageMessage) Fragment(host *assets.ImageHost) (string, error) {
	summary := msg.Summary
	if summary == "" {
		summary = "图片"
	}
	dims := msg.dimsMarkup()

	// 公网 URL 直通：无论图床是否配置都不经图床，尺寸缺失时自动探测
	if s, ok := msg.Src.(string); ok && isPublicURL(s) {
		if dims == "" {
			dims = probeDims(s)
		}
		return fmt.Sprintf("![%s%s](%s)\n", summary, dims, s), nil
	}

	// 本地文件/base64/[]byte：必须走图床，上传后探测尺寸
	if host == nil || host.Size() == 0 {
		return "", fmt.Errorf("图床未配置且图片非公网 URL")
	}
	resolved, err := host.Resolve(msg.Src)
	if err != nil || resolved.URL == "" {
		return "", fmt.Errorf("图床转换失败: %v", err)
	}
	if dims == "" && resolved.Width > 0 && resolved.Height > 0 {
		dims = fmt.Sprintf(" #%dpx #%dpx", resolved.Width, resolved.Height)
	}
	return fmt.Sprintf("![%s%s](%s)\n", summary, dims, resolved.URL), nil
}

// isPublicURL 判断是否为公网 http(s) 链接。
func isPublicURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// dimsMarkup 显式尺寸转 markdown 标注；未指定返回空串。
func (msg *ImageMessage) dimsMarkup() string {
	if msg.Width > 0 && msg.Height > 0 {
		return fmt.Sprintf(" #%dpx #%dpx", msg.Width, msg.Height)
	}
	return ""
}

// probeDims 公网 URL 自动探测尺寸；失败静默返回空（不阻塞发送）。
func probeDims(url string) string {
	w, h, err := images.GetImageDimensions(url)
	if err != nil || w <= 0 || h <= 0 {
		return ""
	}
	return fmt.Sprintf(" #%dpx #%dpx", w, h)
}

// sendMedia 上传 QQ 后以媒体消息发送。
func (msg *ImageMessage) sendMedia() error {
	up := MediaUploadFor(1, msg.Src, msg.Summary)
	fileInfo, err := msg.Qapi.UploadMedia(msg.Target, msg.GroupId, msg.UserId, up)
	if err != nil {
		return err
	}
	media := &MediaMessage{Message: msg.Message, Media: MediaContent{FileInfo: fileInfo}}
	media.Init()
	return media.Send()
}
