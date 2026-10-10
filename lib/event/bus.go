package event

import (
	"encoding/json"
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

var busLog = logx.New("event")

// Handler 通用事件订阅者 (内部形态), 由 On 从具体类型包装而来。
type Handler func(*Context, Event)

// BuiltinHook 内置分发策略 (指令路由/按钮回调/审计 resolve), 由 middleware 注入。
type BuiltinHook func(payload structers.Payload, client *api.BotAPI)

var (
	handlerMu   sync.RWMutex
	handlers    = make(map[constant.EventType][]Handler)
	anyHandlers []Handler

	builtinHook BuiltinHook
)

// On 订阅事件。
func On[E Event](t constant.EventType, h func(*Context, E)) {
	handlerMu.Lock()
	handlers[t] = append(handlers[t], func(ctx *Context, ev Event) {
		e, ok := ev.(E)
		if !ok {
			return
		}
		h(ctx, e)
	})
	handlerMu.Unlock()
}

// OnFor 同 On, 额外把插件归属写进上下文。
func OnFor[E Event](pluginID string, t constant.EventType, h func(*Context, E)) {
	if pluginID == "" {
		On(t, h)
		return
	}
	On(t, func(ctx *Context, ev E) {
		ctx.PluginId = pluginID
		h(ctx, ev)
	})
}

// OnAny 订阅所有事件。
func OnAny(h func(*Context, Event)) {
	handlerMu.Lock()
	anyHandlers = append(anyHandlers, h)
	handlerMu.Unlock()
}

// SetBuiltinHook 注入内置分发钩子, 幂等, 最后一次生效。
func SetBuiltinHook(h BuiltinHook) { builtinHook = h }

// Ingest 事件入口: 解析 → 订阅者 → 内置分发。未知/解析失败事件不吞, 透传 UnknownEvent。
func Ingest(payload structers.Payload, client *api.BotAPI) {
	ev := parse(payload)

	if unknown, ok := ev.(*UnknownEvent); ok {
		busLog.Warnf("未知事件 %s 已透传, 载荷: %s", unknown.RawType, compact(unknown.RawBody))
	}

	notify(ev, client)
	if builtinHook != nil {
		builtinHook(payload, client)
	}
}

func notify(ev Event, client *api.BotAPI) {
	handlerMu.RLock()
	subs := append([]Handler{}, handlers[ev.Type()]...)
	subs = append(subs, anyHandlers...)
	handlerMu.RUnlock()
	if len(subs) == 0 {
		return
	}
	for _, h := range subs {
		h := h
		ctx := newContext(client, ev)
		dispatchTask(ctx, h, ev)
	}
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
