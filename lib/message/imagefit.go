package message

import "github.com/KasumiYuku/Aurorix/lib/images"

func (msg *Message) fitImages(content string) string {
	if msg.Qapi == nil {
		return content
	}
	return images.ClampMarkdown(content, msg.Qapi.ImageBox)
}
