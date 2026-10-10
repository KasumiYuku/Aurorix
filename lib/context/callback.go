package context

import "github.com/KasumiYuku/Aurorix/lib/api"

// 按钮回调上下文
type CallbackContext struct {
	*Context
	Data          string
	ButtonId      string
	InteractionID string
	MessageID     string
}

func (ctx *CallbackContext) Init(eventId string, client *api.BotAPI) {
	ctx.Context = &Context{}
	ctx.Context.Init("", eventId, client)
}

// Done 回执按钮交互，终止客户端按钮 loading。优先用平台交互 ID，缺省回退到事件 ID。
func (ctx *CallbackContext) Done() error {
	interactionID := ctx.InteractionID
	if interactionID == "" {
		interactionID = ctx.EventId
	}
	return ctx.Context.Qapi.InteracteCallback(interactionID)
}
