package event

import (
	"encoding/json"
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/role"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

var busLog = logx.New("event")

// Handler 通用事件订阅者 (内部形态), 由 On 从具体类型包装而来。
type Handler func(*Context, Event)

// BuiltinHook 内置分发策略 (指令路由/按钮回调/审计 resolve), 由 middleware 注入。
type BuiltinHook func(payload structers.Payload, client *api.BotAPI)

type subscriber struct {
	pluginID string
	handler  Handler
}

var (
	handlerMu   sync.RWMutex
	handlers    = make(map[constant.EventType][]subscriber)
	anyHandlers []subscriber

	builtinHook BuiltinHook
)

func subscribe(t constant.EventType, pluginID string, h Handler) {
	handlerMu.Lock()
	if t == "" {
		anyHandlers = append(anyHandlers, subscriber{pluginID: pluginID, handler: h})
	} else {
		handlers[t] = append(handlers[t], subscriber{pluginID: pluginID, handler: h})
	}
	handlerMu.Unlock()
}

func wrap[E Event](h func(*Context, E)) Handler {
	return func(ctx *Context, ev Event) {
		e, ok := ev.(E)
		if !ok {
			return
		}
		h(ctx, e)
	}
}

func On[E Event](t constant.EventType, h func(*Context, E)) {
	subscribe(t, "", wrap(h))
}

// OnFor 同 On, 额外绑定插件归属: 插件被停用后它的订阅者不再被调用。
func OnFor[E Event](pluginID string, t constant.EventType, h func(*Context, E)) {
	subscribe(t, pluginID, wrap(h))
}

func OnAny(h func(*Context, Event)) {
	subscribe("", "", h)
}

// SetBuiltinHook 注入内置分发钩子, 幂等, 最后一次生效。
func SetBuiltinHook(h BuiltinHook) { builtinHook = h }

// Ingest 事件入口: 解析 → 订阅者 → 内置分发。未知/解析失败事件不吞, 透传 UnknownEvent。
func Ingest(payload structers.Payload, client *api.BotAPI) {
	ev := parse(payload)

	if unknown, ok := ev.(*UnknownEvent); ok {
		busLog.Debugf("未知事件 %s 已透传, 载荷: %s", unknown.RawType, compact(unknown.RawBody))
	}

	notify(ev, client)
	if builtinHook != nil {
		builtinHook(payload, client)
	}
}

func notify(ev Event, client *api.BotAPI) {
	handlerMu.RLock()
	subs := make([]subscriber, 0, len(handlers[ev.Type()])+len(anyHandlers))
	subs = append(subs, handlers[ev.Type()]...)
	subs = append(subs, anyHandlers...)
	handlerMu.RUnlock()
	for _, sub := range subs {
		if sub.pluginID != "" && !allowListener(sub.pluginID, ev) {
			continue
		}
		ctx := newContext(client, ev)
		ctx.PluginId = sub.pluginID
		ctx.Trace = logx.NewTrace(eventMessageID(ev))
		runSubscriber(ctx, sub.handler, ev)
	}
}

func allowListener(pluginID string, ev Event) bool {
	disabled, maintenance := plugin.StateOf(pluginID)
	if disabled {
		return false
	}
	if maintenance == "" && !plugin.HasListenerRule(pluginID) {
		return true
	}
	userID, conversationID := eventActor(ev)
	return plugin.CanUse(plugin.AccessQuery{
		PluginID:       pluginID,
		UserID:         userID,
		ConversationID: conversationID,
		Role:           role.Resolve(userID, conversationID, ""),
	})
}

func eventMessageID(ev Event) string {
	switch e := ev.(type) {
	case *MessageEvent:
		return e.MessageID
	case *InteractionEvent:
		return e.MessageID
	case *JoinRequestEvent:
		return e.MessageID
	case *MemberEvent:
		return e.MessageID
	}
	return ""
}

func eventActor(ev Event) (userID, conversationID string) {
	switch e := ev.(type) {
	case *MessageEvent:
		return e.Target.User, e.Target.ID
	case *InteractionEvent:
		return e.Target.User, e.Target.ID
	case *JoinRequestEvent:
		return e.UserID, e.GroupID
	case *MemberEvent:
		return e.UserID, e.GroupID
	case *GuildMemberEvent:
		return e.UserID, e.GuildID
	case *GuildEvent:
		return e.OperatorID, e.GuildID
	}
	return "", ""
}

func parse(payload structers.Payload) Event {
	t := payload.Type()
	payload.EventType = t
	spec := SpecOf(t)
	if spec == nil || spec.Parse == nil {
		return &UnknownEvent{
			RawType:   string(t),
			EventID:   payload.ID,
			MessageID: firstNonEmpty(payload.Data.MessageId, payload.Data.Id),
			GroupID:   payload.Data.GroupOpenID,
			UserID:    payload.ActorID(),
			RawBody:   rawOf(payload),
		}
	}
	return spec.Parse(payload)
}

func rawOf(payload structers.Payload) json.RawMessage {
	if len(payload.RawEvent) > 0 {
		return payload.RawEvent
	}
	b, err := json.Marshal(payload.Data)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

func compact(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	return string(raw)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
