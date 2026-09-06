package context

import (
	"Plrx/lib/api"
)

type ApplyJoinGroupContext struct {
	*Context
	Answer  string // 回答答案
	groupId string
	userId  string
	eventId string
}

func (ctx *ApplyJoinGroupContext) Init(requestId, groupId, userId string, client *api.BotAPI) {
	ctx.Context = &Context{}
	ctx.Context.Init("", requestId, client)
	ctx.eventId = requestId
	ctx.groupId = groupId
	ctx.userId = userId
}

// 同意入群
func (ctx *ApplyJoinGroupContext) Accept() error {
	return ctx.Context.Qapi.AcceptGroupJoinRequest(ctx.eventId, ctx.groupId, ctx.userId)
}

// 以理由拒绝入群
func (ctx *ApplyJoinGroupContext) Deny(reason string) error {
	return ctx.Context.Qapi.RejectGroupJoinRequest(ctx.eventId, ctx.groupId, ctx.userId, reason)
}

// 拒绝并加入黑名单
func (ctx *ApplyJoinGroupContext) DenyAndAddToBlacklist(reanson string) error {
	return ctx.Context.Qapi.RejectGroupJoinRequestAndAddToBlacklist(ctx.eventId, ctx.groupId, ctx.userId, reanson)
}
