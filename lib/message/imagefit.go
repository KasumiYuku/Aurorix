package message

import "github.com/KasumiYuku/Aurorix/lib/images"

// fitImages 把正文里每张 markdown 图片等比收进客户端配置的显示框; 没配客户端时原样返回。
func (msg *Message) fitImages(content string) string {
	if msg.Qapi == nil {
		return content
	}
	return images.ClampMarkdown(content, msg.Qapi.ImageBox)
}
