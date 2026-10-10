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
}

func (ctx *MessageContext) Init(messageId, eventId string, client *api.BotAPI) {
	ctx.Context = &Context{}
	ctx.Context.Init(messageId, eventId, client)
}
