package middleware

import (
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

func blockForMaintenance(payload structers.Payload, client *api.BotAPI) bool {
	text := config.Current().MaintenanceText()
	if text == "" {
		return false
	}
	maintenanceNotifier(payload, client, text)
	return true
}

var maintenanceNotifier = func(payload structers.Payload, client *api.BotAPI, text string) {
	switch payload.Type() {
	case constant.INTERACTION_CREATE:
		ackInteraction(payload, client)
	case constant.GROUP_AT_MESSAGE_CREATE, constant.GROUP_MESSAGE_CREATE:
		replyMaintenanceOnPayload(payload, client, payload.Target(constant.SceneGroup), text)
	case constant.C2C_MESSAGE_CREATE:
		replyMaintenanceOnPayload(payload, client, payload.Target(constant.SceneC2C), text)
	case constant.AT_MESSAGE_CREATE, constant.MESSAGE_CREATE, constant.DIRECT_MESSAGE_CREATE:
		if payload.Type() == constant.MESSAGE_CREATE && !mentionsBot(payload, client) {
			return
		}
		replyMaintenanceOnPayload(payload, client, payload.Target(channelScene(payload)), text)
	}
}

func replyMaintenanceOnPayload(payload structers.Payload, client *api.BotAPI, target constant.Target, text string) {
	if target.ID == "" {
		return
	}
	ctx := &context.MessageContext{
		UserMessage: message.UserMessage{Content: payload.Data.Content, Attachments: payload.Data.Attachments},
		Username:    payload.Data.Author.Username,
	}
	ctx.Init(payload.Data.Id, payload.ID, client)
	ctx.SetTarget(target)
	replyMaintenance(ctx, text)
}

func ackInteraction(payload structers.Payload, client *api.BotAPI) {
	ctx := &context.CallbackContext{}
	ctx.Init(payload.ID, client)
	ctx.InteractionID = payload.Data.Id
	if err := ctx.Done(); err != nil {
		messageLog.Debugf("维护期间回执交互失败: %v", err)
	}
}
