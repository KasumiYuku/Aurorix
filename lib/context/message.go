package context

import (
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

type MessageContext struct {
	*Context
	message.UserMessage
	Raw    string
	Parsed any

	Mentions        []structers.Mention
	Quote           *structers.Quote
	Emojis          []string
	AttachmentTypes []string
	AvatarURL       string
	// Username 说话人昵称。平台只在部分事件里给(群消息有, 早期版本没有), 空串表示没给。
	//
	// 有了它就不必再拿 openid 当名字给人看 —— 那串十六进制对谁都没意义。
	Username string
}

func (ctx *MessageContext) Init(messageId, eventId string, client *api.BotAPI) {
	ctx.Context = &Context{}
	ctx.Context.Init(messageId, eventId, client)
}
