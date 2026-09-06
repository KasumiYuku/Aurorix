package context

import (
	"Plrx/lib/api"
	"Plrx/lib/message"
	"Plrx/lib/structers"
)

type MessageContext struct {
	*Context
	message.UserMessage
	Raw    string // 原始消息
	Parsed any    // 解析后

	// 入站解析增强
	Mentions        []structers.Mention // @提及列表
	Quote           *structers.Quote    // 引用消息
	Emojis          []string            // 解码后的表情文本
	AttachmentTypes []string            // 附件分类
	AvatarURL       string              // 发送者头像
}

func (ctx *MessageContext) Init(messageId, eventId string, client *api.BotAPI) {
	ctx.Context = &Context{}
	ctx.Context.Init(messageId, eventId, client)
}
