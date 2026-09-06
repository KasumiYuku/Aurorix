package context

import "Plrx/lib/api"

// 按钮回调上下文
type CallbackContext struct {
	*Context
	Data          string
	PluginId      string
	ButtonId      string
	InteractionID string // 平台交互 ID（事件 d.id），回执专用
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
