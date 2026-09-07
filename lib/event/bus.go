package event

// 事件总线: 解析注册表 + 订阅注册表 + 内置分发钩子。
// 订阅者并发执行(复用中间件任务池), 内置分发最后兜底业务语义。

import (
	"encoding/json"
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

var busLog = logx.New("event")

// Handler 事件订阅者。
type Handler func(*Context)

// BuiltinHook 内置分发策略 (指令路由/按钮回调/审计 resolve), 由 middleware 注入。
type BuiltinHook func(payload structers.Payload, client *api.BotAPI)

var (
	handlerMu sync.RWMutex
	handlers  = make(map[constant.EventType][]Handler)

	parserMu sync.RWMutex
	parsers  = make(map[constant.EventType]func(structers.Payload) Event)

	builtinHook BuiltinHook
)

// Subscribe 注册事件订阅。同类型多订阅者均触发, 顺序不保证。
func Subscribe(t constant.EventType, h Handler) {
	handlerMu.Lock()
	handlers[t] = append(handlers[t], h)
	handlerMu.Unlock()
}

// RegisterParser 注册事件解析器, 覆盖内置解析。返回前一个解析器(可能为 nil)。
func RegisterParser(t constant.EventType, p func(structers.Payload) Event) {
	parserMu.Lock()
	parsers[t] = p
	parserMu.Unlock()
}

// SetBuiltinHook 注入内置分发钩子, 幂等, 最后一次生效。
func SetBuiltinHook(h BuiltinHook) { builtinHook = h }

// Ingest 事件入口: 解析 → 订阅者 → 内置分发。未知/解析失败事件不吞, 透传 UnknownEvent。
// 订阅者先跑(观察), 内置分发后跑(业务), 互不阻塞。
func Ingest(payload structers.Payload, client *api.BotAPI) {
	ev := parse(payload)

	// 未知事件是异常信号, 必须可见; 已知事件由订阅者自行记录, 总线不打日志防刷屏
	if unknown, ok := ev.(*UnknownEvent); ok {
		busLog.Warnf("未知事件 %s 已透传, 载荷: %s", unknown.RawType, compact(unknown.RawBody))
	}

	notify(payload, ev, client)
	if builtinHook != nil {
		builtinHook(payload, client)
	}
}

// notify 并发通知订阅者, 不阻塞 Ingest 主路径。
func notify(payload structers.Payload, ev Event, client *api.BotAPI) {
	handlerMu.RLock()
	subs := handlers[ev.Type()]
	handlerMu.RUnlock()
	if len(subs) == 0 {
		return
	}
	ctx := &Context{Client: client, Event: ev}
	for _, h := range subs {
		h := h
		dispatchTask(ctx, h)
	}
}

// parse 载荷 → 事件; 无解析器或解析出错时降级 UnknownEvent。
func parse(payload structers.Payload) Event {
	t := payload.EventType
	if t == "" {
		t = constant.EventType(payload.T)
	}
	parserMu.RLock()
	p, ok := parsers[t]
	parserMu.RUnlock()
	if !ok {
		return &UnknownEvent{RawType: string(t), RawBody: rawOf(payload)}
	}
	return p(payload)
}

// rawOf 导出事件载荷字节: 优先平台原始 d, 缺失时以 PrasedData 重编码兜底。
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
