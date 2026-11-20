package middleware

import (
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/ban"
	"github.com/KasumiYuku/Aurorix/lib/buttons"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/event"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/parser"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/role"
	"github.com/KasumiYuku/Aurorix/lib/state"
	"github.com/KasumiYuku/Aurorix/lib/stats"
	"github.com/KasumiYuku/Aurorix/lib/structers"
	"github.com/KasumiYuku/Aurorix/lib/templates"
	"github.com/KasumiYuku/Aurorix/lib/utils"
	"strings"
	"time"
)

var messageLog = logx.New("message")

func init() {
	event.SetBuiltinHook(ProcessPayload)

}

type commandDispatchOpts struct {
	target       constant.Target
	raw          string
	checkPrivate bool
}

func dispatchCommand(payload structers.Payload, client *api.BotAPI, opts commandDispatchOpts) {
	tokens := strings.Fields(payload.Data.Content)
	if len(tokens) == 0 {
		return
	}
	rootCommand, afterRoot, ok := plugin.ResolveRoot(tokens)
	if !ok {
		return
	}
	if !plugin.Enabled(rootCommand.PluginId) {
		return
	}
	if text, hit := ban.Hit(payload.ActorID()); hit {
		replyBanned(opts.target, client, text)
		return
	}
	resolvedCommand, commandPath, rest := plugin.Resolve(rootCommand, afterRoot)
	if resolvedCommand == rootCommand && len(afterRoot) > 0 && rootCommand.SubCommandFallback != nil {
		fallback := *rootCommand
		fallback.Handle = rootCommand.SubCommandFallback
		resolvedCommand = &fallback
	}
	ctx := &context.MessageContext{
		UserMessage: message.UserMessage{
			Content:     payload.Data.Content,
			Attachments: payload.Data.Attachments,
		},
		Raw:      opts.raw,
		Username: payload.Data.Author.Username,
		Mentions: payload.Data.Mentions,
		Quote:    structers.ExtractQuote(payload.Data.MsgElements, payload.Data.MessageScene.Ext),
	}
	ctx.Init(payload.Data.Id, payload.ID, client)
	ctx.BindStorage(resolvedCommand.PluginId, commandPath)
	ctx.PluginId = resolvedCommand.PluginId
	ctx.MessageManager.Trace = logx.NewTrace(payload.Data.Id)
	ctx.MessageManager.Command = commandPath
	ctx.MessageManager.OriginalInput = payload.Data.Content
	ctx.MessageManager.NoFooter = rootCommand.NoFooter || resolvedCommand.NoFooter
	ctx.SetTarget(opts.target)

	authorRole := role.Resolve(opts.target.User, opts.target.ID, payload.Data.Author.Role)

	if !rootCommand.Role.CanUse(authorRole) {
		messageLog.Debugf("用户%v无权限使用%v指令", payload.Data.Author.Username, rootCommand.Prefix)
		permissionDenied(rootCommand, ctx)
		return
	}
	if resolvedCommand != rootCommand && !resolvedCommand.Role.CanUse(authorRole) {
		messageLog.Debugf("用户%v无权限使用%v指令", payload.Data.Author.Username, commandPath)
		permissionDenied(resolvedCommand, ctx)
		return
	}
	if opts.checkPrivate && (rootCommand.DisablePrivate || resolvedCommand.DisablePrivate) {
		permissionDenied(resolvedCommand, ctx)
		return
	}
	verdict := plugin.Check(plugin.AccessQuery{
		PluginID:       rootCommand.PluginId,
		CommandPath:    commandPath,
		UserID:         opts.target.User,
		ConversationID: opts.target.ID,
		Role:           authorRole,
	})
	if !verdict.Allowed {
		if verdict.Maintenance != "" {
			replyMaintenance(ctx, verdict.Maintenance)
			return
		}
		if verdict.Denied != "" {
			replyDenied(ctx, verdict.Denied)
			return
		}
		messageLog.Debugf("名单拦下 %s (用户 %s, 会话 %s), 按配置静默", commandPath, opts.target.User, opts.target.ID)
		permissionDenied(resolvedCommand, ctx)
		return
	}

	if resolvedCommand.Handle == nil {
		return
	}
	messageLog.WithTrace(ctx.MessageManager.Trace).Debugf("命中指令 %s (插件 %s, 身份 %s)", commandPath, resolvedCommand.PluginId, authorRole)

	if resolvedCommand.Args != nil {
		parsed, usage, err := parser.ParseArgs(commandPath, resolvedCommand.Args, rest)
		if err != nil {
			content, terr := templates.FillMarkdownTemplate("Card", templates.Args{
				"title": "❌ 指令参数错误",
				"fields": []any{
					map[string]any{"label": "原因", "content": err.Error()},
					map[string]any{"label": "用法", "content": usageOrPath(usage, commandPath)},
				},
			})
			if terr != nil {
				messageLog.Errorf("生成用法提示失败: %v", terr)
				return
			}
			msg := ctx.Msg()
			if opts.target.ID != "" && opts.target.User != "" {
				msg.At(opts.target.User, true)
			}
			if sendErr := msg.Markdown(content).Send(); sendErr != nil {
				messageLog.Errorf("发送用法提示失败: %v", sendErr)
			}
			return
		}
		ctx.Parsed = parsed
	} else {
		ctx.Parsed = rawArgs(payload.Data.Content, tokens, rest, rootCommand)
	}
	if ok, wait := plugin.RateAllowed(commandPath, opts.target.User, resolvedCommand.Rate); !ok {
		replyRateLimited(ctx, wait)
		return
	}
	pool.Go(func() { executeCommand(resolvedCommand, ctx) })
}

func rawArgs(content string, tokens, rest []string, rootCommand *plugin.Command) string {
	n := len(tokens) - len(rest)
	if n > 0 {
		pos := 0
		for _, tok := range tokens[:n] {
			i := strings.Index(content[pos:], tok)
			if i < 0 {
				return strings.Join(rest, " ")
			}
			pos += i + len(tok)
		}
		return strings.TrimLeft(content[pos:], " \t\n")
	}
	for _, p := range constant.PrefixChars() {
		if p != "" && strings.HasPrefix(tokens[0], p) {
			i := strings.Index(content, tokens[0])
			if i < 0 {
				return strings.Join(rest, " ")
			}
			return content[i+len(p)+len(rootCommand.Prefix):]
		}
	}
	return strings.Join(rest, " ")
}

func usageOrPath(usage, commandPath string) string {
	if strings.TrimSpace(usage) != "" {
		return usage
	}
	return usageText(commandPath)
}

func usageText(path string) string {
	prefix := ""
	for _, p := range constant.PrefixChars() {
		if p != "" {
			prefix = p
			break
		}
	}
	return prefix + path
}

func ProcessPayload(payload structers.Payload, client *api.BotAPI) {
	if payload.Type() == constant.INTERACTION_CREATE {
		if text, hit := ban.Hit(payload.ActorID()); hit {
			refuseBanned(payload, client, text)
			return
		}
	}
	switch payload.Type() {
	case constant.GROUP_AT_MESSAGE_CREATE, constant.GROUP_MESSAGE_CREATE:
		target := payload.Target(constant.SceneGroup)
		state.IncRecv()
		stats.Received(target, "")
		raw := payload.Data.Content
		payload.Data.Content = utils.FilterAt(payload.Data.Content)
		dispatchCommand(payload, client, commandDispatchOpts{target: target, raw: raw})
	case constant.C2C_MESSAGE_CREATE:
		target := payload.Target(constant.SceneC2C)
		state.IncRecv()
		stats.Received(target, payload.Data.Author.Username)
		raw := payload.Data.Content
		payload.Data.Content = strings.TrimSpace(payload.Data.Content)
		if payload.Data.Content == "" {
			return
		}
		dispatchCommand(payload, client, commandDispatchOpts{target: target, raw: raw, checkPrivate: true})
	case constant.AT_MESSAGE_CREATE, constant.MESSAGE_CREATE, constant.DIRECT_MESSAGE_CREATE:
		dispatchChannelMessage(payload, client)
	case constant.INTERACTION_CREATE:
		state.IncButton()
		stats.Button()
		ctx := &context.CallbackContext{}
		ctx.Init(payload.ID, client)
		ctx.InteractionID = payload.Data.Id
		ctx.ButtonId = payload.Data.Callback.Resolved.ButtonId
		ctx.Data = payload.Data.Callback.Resolved.ButtonData
		ctx.SetTarget(payload.InteractionTarget())
		if err := ctx.Done(); err != nil {
			messageLog.Errorf("回执按钮 %v 失败: %v", ctx.ButtonId, err)
		}
		callbackFunc, ok := buttons.GetCallbackFunc(ctx.ButtonId)
		if !ok {
			if ctx.ButtonId == "" {
				messageLog.Debugf("按钮交互没有 button_id(平台未解析, 交互=%s)", ctx.InteractionID)
				return
			}
			messageLog.Warnf("回调按钮: id=%q 未注册回调函数, 已回执交互 (data=%q 交互=%s)", ctx.ButtonId, ctx.Data, ctx.InteractionID)
			return
		}
		if allowed, wait := plugin.ClickRateAllowed(ctx.ButtonId, ctx.Target.User); !allowed {
			replyCallbackRateLimited(ctx, wait)
			return
		}
		pool.Go(func() { callbackHandleFunc(callbackFunc, ctx) })
	case constant.GROUP_JOIN_REQUEST:
		var answer string
		switch payload.Data.VerifyInfo.Method {
		case "verify_message":
			answer = payload.Data.VerifyInfo.VerifyMsg
		case "admin_review_qa":
			if len(payload.Data.VerifyInfo.AnswerList) < 1 {
				return
			}
			answer = payload.Data.VerifyInfo.AnswerList[0].Answer
		default:
			return
		}
		ctx := &context.ApplyJoinGroupContext{
			Answer: answer,
		}
		ctx.Init(payload.Data.JoinRequestId, payload.Data.GroupOpenID, payload.ActorID(), client)
		err := plugin.CallGlobalJoinGroupHandle(ctx)
		if err != nil {
		}
		return
	case constant.MESSAGE_AUDIT_PASS, constant.MESSAGE_AUDIT_REJECT:
		api.ResolveAudit(payload.Data.AuditID, payload.Data.MessageId, payload.EventType == constant.MESSAGE_AUDIT_PASS)
		return
	}
}

func replyMaintenance(ctx *context.MessageContext, text string) {
	replyText(ctx, text, "维护提示")
}

func replyDenied(ctx *context.MessageContext, text string) {
	replyText(ctx, text, "无权限提示")
}

func replyText(ctx *context.MessageContext, text, label string) {
	msg := ctx.Msg()
	if ctx.Target.ID != "" && ctx.Target.User != "" {
		msg.At(ctx.Target.Mention(), true)
	}
	if err := msg.Markdown(text).NoFooter().Send(); err != nil {
		messageLog.Errorf("发送%s失败: %v", label, err)
	}
}

func replyRateLimited(ctx *context.MessageContext, wait time.Duration) {
	secs := rateSeconds(wait)
	msg := ctx.Msg()
	if ctx.Target.ID != "" && ctx.Target.User != "" {
		msg.At(ctx.Target.Mention(), true)
	}
	if err := msg.Markdown(fmt.Sprintf("⏳ 触发太频繁了，%d 秒后再试", secs)).NoFooter().Send(); err != nil {
		messageLog.Errorf("发送限速提示失败: %v", err)
	}
}

func replyCallbackRateLimited(ctx *context.CallbackContext, wait time.Duration) {
	secs := rateSeconds(wait)
	msg := ctx.Msg()
	if ctx.Target.ID != "" && ctx.Target.User != "" {
		msg.At(ctx.Target.Mention(), true)
	}
	if err := msg.Markdown(fmt.Sprintf("⏳ 触发太频繁了，%d 秒后再试", secs)).NoFooter().Send(); err != nil {
		messageLog.Errorf("发送回调限速提示失败: %v", err)
	}
}

func rateSeconds(wait time.Duration) int {
	secs := int((wait + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return secs
}

func executeCommand(command *plugin.Command, ctx *context.MessageContext) {
	defer func() {
		if recovered := recover(); recovered != nil {
			messageLog.Errorf("在执行指令%v (插件: %v)时出现panic: %v", command.Prefix, command.PluginId, recovered)
			invokeErrorHook(command, ctx, fmt.Errorf("command panic: %v", recovered))
		}
	}()
	if err := command.Handle(ctx); err != nil {
		messageLog.Errorf("在执行指令%v (插件: %v)时出现error: %v", command.Prefix, command.PluginId, err)
		invokeErrorHook(command, ctx, err)
	}
}

func invokeErrorHook(command *plugin.Command, ctx *context.MessageContext, commandErr error) {
	if command.HandleError == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			messageLog.Errorf("在处理指令%v (插件: %v)的error时出现panic: %v", command.Prefix, command.PluginId, recovered)
		}
	}()
	if handleErr := command.HandleError(ctx, commandErr); handleErr != nil {
		messageLog.Errorf("在处理指令%v (插件: %v)的error时再次出现error: %v", command.Prefix, command.PluginId, handleErr)
	}
}

func permissionDenied(cmd *plugin.Command, ctx *context.MessageContext) {
	if cmd.PermissionDenied == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			messageLog.Errorf("在执行指令%v (插件: %v)的权限拒绝处理函数时出现panic: %v", cmd.Prefix, cmd.PluginId, recovered)
		}
	}()
	if err := cmd.PermissionDenied(ctx); err != nil {
		messageLog.Errorf("在执行指令%v (插件: %v)的权限拒绝处理函数时出现error: %v", cmd.Prefix, cmd.PluginId, err)
	}
}

func callbackHandleFunc(handle buttons.CallbackButtonHandleFunc, ctx *context.CallbackContext) {
	defer func() {
		if r := recover(); r != nil {
			messageLog.Errorf("在执行回调按钮: %v 处理函数时候出现panic: %v", ctx.ButtonId, r)
		}
	}()
	if err := handle(ctx); err != nil {
		messageLog.Errorf("在执行回调按钮: %v 处理函数时候出现error: %v", ctx.ButtonId, err)
	}
}

func channelScene(payload structers.Payload) constant.Scene {
	if payload.Type() == constant.DIRECT_MESSAGE_CREATE {
		return constant.SceneDirect
	}
	return constant.SceneChannel
}

func refuseBanned(payload structers.Payload, client *api.BotAPI, text string) {
	ctx := &context.CallbackContext{}
	ctx.Init(payload.ID, client)
	ctx.InteractionID = payload.Data.Id
	ctx.SetTarget(payload.InteractionTarget())
	if err := ctx.Done(); err != nil {
		messageLog.Errorf("回执被封用户的按钮交互失败: %v", err)
	}
	replyBanned(payload.InteractionTarget(), client, text)
}

func replyBanned(target constant.Target, client *api.BotAPI, text string) {
	if target.Empty() {
		return
	}
	if client == nil {
		messageLog.Warnf("封禁提示发不出去: 客户端为空 target=%s", target)
		return
	}
	mgr := &context.MessageManager{Target: target, Qapi: client}
	b := mgr.Msg()
	if target.Scene != constant.SceneC2C {
		b.At(target.Mention(), true)
	}
	if err := b.Markdown(text).NoFooter().Send(); err != nil {
		messageLog.Warnf("发送封禁提示失败 target=%s: %v", target, err)
	}
}

func dispatchChannelMessage(payload structers.Payload, client *api.BotAPI) {
	scene := channelScene(payload)
	if payload.Type() == constant.MESSAGE_CREATE && !mentionsBot(payload, client) {
		return
	}
	target := payload.Target(scene)
	state.IncRecv()
	stats.Received(target, "")
	raw := payload.Data.Content
	payload.Data.Content = channelContent(raw, payload, client)
	if payload.Data.Content == "" {
		return
	}
	dispatchCommand(payload, client, commandDispatchOpts{target: target, raw: raw, checkPrivate: scene == constant.SceneDirect})
}

func mentionsBot(payload structers.Payload, client *api.BotAPI) bool {
	if structers.MentionsYou(payload.Data.Mentions) {
		return true
	}
	botID := ""
	if client != nil {
		botID = client.BotUserID()
	}
	return structers.MentionsUser(payload.Data.Mentions, botID)
}

func channelContent(raw string, payload structers.Payload, client *api.BotAPI) string {
	content := structers.StripMentions(raw, payload.Data.Mentions)
	if content != raw {
		return content
	}
	if payload.Type() == constant.AT_MESSAGE_CREATE {
		return structers.StripLeadingMention(raw)
	}
	return content
}
