package event

// 事件总线: 解析注册表 + 订阅注册表 + 内置分发钩子。
// 订阅者并发执行(复用中间件任务池), 内置分发最后兜底业务语义。
// 订阅全部在 init/启动期完成, 运行期 handlers 只读, 分发路径零锁。

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
	handlerMu sync.RWMutex
	handlers  = make(map[constant.EventType][]Handler)

	builtinHook BuiltinHook
)

// On 订阅事件。回调签名 func(ctx, *具体事件类型):
// ctx 提供指令同款消息 API, 事件数据直接作为参数, 无需类型断言。
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

	notify(ev, client)
	if builtinHook != nil {
		builtinHook(payload, client)
	}
}

// notify 并发通知订阅者, 不阻塞 Ingest 主路径。
func notify(ev Event, client *api.BotAPI) {
	handlerMu.RLock()
	subs := handlers[ev.Type()]
	handlerMu.RUnlock()
	if len(subs) == 0 {
		return
	}
	for _, h := range subs {
		h := h
		// 每个订阅者独立上下文: MessageManager 的 ref 懒初始化非原子, 共享会竞态
		ctx := newContext(client, ev)
		dispatchTask(ctx, h, ev)
	}
}

// parse 载荷 → 事件; 无解析器或解析出错时降级 UnknownEvent。
func parse(payload structers.Payload) Event {
	t := payload.EventType
	if t == "" {
		t = constant.EventType(payload.T)
	}
	spec := SpecOf(t)
	if spec == nil || spec.Parse == nil {
		return &UnknownEvent{RawType: string(t), RawBody: rawOf(payload)}
	}
	return spec.Parse(payload)
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
