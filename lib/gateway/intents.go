package gateway

// 事件 intent 位定义（与 QQ 开放平台一致）
const defaultIntents = intentUserMessage | intentInteractions | intentGroupMembers | intentMessageAudit

const (
	intentGuildMessages = 1 << 9
	intentGroupMembers  = 1 << 24
	intentUserMessage   = 1 << 25
	intentInteractions  = 1 << 26
	intentMessageAudit  = 1 << 27
)

// eventIntentMap 事件名 → intent 位
var eventIntentMap = map[string]int{
	"GROUP_AT_MESSAGE_CREATE": intentUserMessage,
	"GROUP_MESSAGE_CREATE":    intentUserMessage,
	"C2C_MESSAGE_CREATE":      intentUserMessage,
	"INTERACTION_CREATE":      intentInteractions,
	"GROUP_JOIN_REQUEST":      intentGroupMembers,
	"GROUP_MEMBER_ADD":        intentGroupMembers,
	"GROUP_MEMBER_REMOVE":     intentGroupMembers,
	"GROUP_ADD_ROBOT":         intentUserMessage,
	"GROUP_DEL_ROBOT":         intentUserMessage,
	"GROUP_MSG_RECEIVE":       intentUserMessage,
	"GROUP_MSG_REJECT":        intentUserMessage,
	"C2C_MSG_RECEIVE":         intentUserMessage,
	"C2C_MSG_REJECT":          intentUserMessage,
	"MESSAGE_AUDIT_PASS":      intentMessageAudit,
	"MESSAGE_AUDIT_REJECT":    intentMessageAudit,
}

// IntentEvents 可订阅事件全集(顺序固定), 供管理台枚举选择。
func IntentEvents() []string {
	return []string{
		"GROUP_AT_MESSAGE_CREATE",
		"GROUP_MESSAGE_CREATE",
		"C2C_MESSAGE_CREATE",
		"INTERACTION_CREATE",
		"GROUP_JOIN_REQUEST",
		"GROUP_MEMBER_ADD",
		"GROUP_MEMBER_REMOVE",
		"GROUP_ADD_ROBOT",
		"GROUP_DEL_ROBOT",
		"GROUP_MSG_RECEIVE",
		"GROUP_MSG_REJECT",
		"C2C_MSG_RECEIVE",
		"C2C_MSG_REJECT",
		"MESSAGE_AUDIT_PASS",
		"MESSAGE_AUDIT_REJECT",
	}
}

// Intents 将配置的事件名列表编译成 intent 位掩码。
func Intents(events []string) int {
	bits := 0
	for _, e := range events {
		bits |= eventIntentMap[e]
	}
	if bits == 0 {
		// 默认订阅群消息 + 按钮回调 + 成员变动
		bits = defaultIntents
	}
	return bits
}
