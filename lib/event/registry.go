package event

// 事件注册表: 平台事件名 → 解析器。
// 注册期构建后经 atomic 快照发布, 读路径零锁。

import (
	"sync"
	"sync/atomic"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

// Parser 把平台载荷解析为纯数据事件对象。
type Parser func(structers.Payload) Event

// Spec 单个事件的注册信息。
type Spec struct {
	Type  constant.EventType
	Parse Parser
}

var (
	regMu    sync.Mutex
	registry = make([]Spec, 0, 16)
	byType   atomic.Value // map[constant.EventType]*Spec
)

func init() {
	specs := []Spec{
		{constant.GROUP_AT_MESSAGE_CREATE, parseMessageEvent(constant.GroupMessage)},
		{constant.GROUP_MESSAGE_CREATE, parseMessageEvent(constant.GroupMessage)},
		{constant.C2C_MESSAGE_CREATE, parseMessageEvent(constant.PrivateMessage)},
		{constant.INTERACTION_CREATE, parseInteraction},
		{constant.GROUP_JOIN_REQUEST, parseJoinRequest},
		{constant.MESSAGE_AUDIT_PASS, parseAudit(true)},
		{constant.MESSAGE_AUDIT_REJECT, parseAudit(false)},
		{constant.GROUP_MEMBER_ADD, parseMember(true)},
		{constant.GROUP_MEMBER_REMOVE, parseMember(false)},
		{constant.GROUP_ADD_ROBOT, parseRobot(true)},
		{constant.GROUP_DEL_ROBOT, parseRobot(false)},
		{constant.GROUP_MSG_RECEIVE, parseGroupReceive(true)},
		{constant.GROUP_MSG_REJECT, parseGroupReceive(false)},
		{constant.C2C_MSG_RECEIVE, parsePrivateReceive(true)},
		{constant.C2C_MSG_REJECT, parsePrivateReceive(false)},
	}
	regMu.Lock()
	registry = specs
	regMu.Unlock()
	publish()
}

func publish() {
	m := make(map[constant.EventType]*Spec, len(registry))
	for i := range registry {
		m[registry[i].Type] = &registry[i]
	}
	byType.Store(m)
}

// SpecOf 按平台事件名取解析器, 未注册返回 nil。
func SpecOf(t constant.EventType) *Spec {
	m, _ := byType.Load().(map[constant.EventType]*Spec)
	return m[t]
}

// RegisteredTypes 全部已注册事件名, 供管理与文档展示。
func RegisteredTypes() []constant.EventType {
	m, _ := byType.Load().(map[constant.EventType]*Spec)
	out := make([]constant.EventType, 0, len(m))
	for t := range m {
		out = append(out, t)
	}
	return out
}

// Register 注册自定义事件解析器, 覆盖内置同名。
func Register(t constant.EventType, p Parser) {
	regMu.Lock()
	for i := range registry {
		if registry[i].Type == t {
			registry[i].Parse = p
			regMu.Unlock()
			publish()
			return
		}
	}
	registry = append(registry, Spec{t, p})
	regMu.Unlock()
	publish()
}
