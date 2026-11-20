package event

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
	byType   atomic.Value
)

func init() {
	specs := []Spec{
		{constant.GROUP_AT_MESSAGE_CREATE, parseMessageEvent(constant.SceneGroup)},
		{constant.GROUP_MESSAGE_CREATE, parseMessageEvent(constant.SceneGroup)},
		{constant.C2C_MESSAGE_CREATE, parseMessageEvent(constant.SceneC2C)},
		{constant.AT_MESSAGE_CREATE, parseMessageEvent(constant.SceneChannel)},
		{constant.MESSAGE_CREATE, parseMessageEvent(constant.SceneChannel)},
		{constant.DIRECT_MESSAGE_CREATE, parseMessageEvent(constant.SceneDirect)},
		{constant.INTERACTION_CREATE, parseInteraction},
		{constant.GROUP_JOIN_REQUEST, parseJoinRequest},
		{constant.MESSAGE_AUDIT_PASS, parseAudit(true)},
		{constant.MESSAGE_AUDIT_REJECT, parseAudit(false)},
		{constant.GROUP_MEMBER_ADD, parseMember(true)},
		{constant.GROUP_MEMBER_REMOVE, parseMember(false)},
		{constant.GUILD_MEMBER_ADD, parseGuildMember(true)},
		{constant.GUILD_MEMBER_REMOVE, parseGuildMember(false)},
		{constant.GUILD_CREATE, parseGuildEvent(constant.GUILD_CREATE)},
		{constant.GUILD_UPDATE, parseGuildEvent(constant.GUILD_UPDATE)},
		{constant.GUILD_DELETE, parseGuildEvent(constant.GUILD_DELETE)},
		{constant.CHANNEL_CREATE, parseGuildEvent(constant.CHANNEL_CREATE)},
		{constant.CHANNEL_UPDATE, parseGuildEvent(constant.CHANNEL_UPDATE)},
		{constant.CHANNEL_DELETE, parseGuildEvent(constant.CHANNEL_DELETE)},
		{constant.GROUP_ADD_ROBOT, parseRobot(true)},
		{constant.GROUP_DEL_ROBOT, parseRobot(false)},
		{constant.GROUP_MSG_RECEIVE, parseGroupReceive(true)},
		{constant.GROUP_MSG_REJECT, parseGroupReceive(false)},
		{constant.C2C_MSG_RECEIVE, parsePrivateReceive(true)},
		{constant.C2C_MSG_REJECT, parsePrivateReceive(false)},
		{constant.FRIEND_ADD, parseFriend(true)},
		{constant.FRIEND_DEL, parseFriend(false)},
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
