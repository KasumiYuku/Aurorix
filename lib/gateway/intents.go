package gateway

const defaultIntents = intentUserMessage | intentInteractions | intentGroupMembers | intentMessageAudit

const (
	intentGuilds         = 1 << 0
	intentGuildMembers   = 1 << 1
	intentGuildMessages  = 1 << 9
	intentDirectMessage  = 1 << 12
	intentGroupMembers   = 1 << 24
	intentUserMessage    = 1 << 25
	intentInteractions   = 1 << 26
	intentMessageAudit   = 1 << 27
	intentPublicMessages = 1 << 30
)

const guildIntents = intentGuilds | intentGuildMembers | intentGuildMessages | intentDirectMessage | intentPublicMessages

var eventIntentMap = map[string]int{
	"GROUP_AT_MESSAGE_CREATE": intentUserMessage,
	"GROUP_MESSAGE_CREATE":    intentUserMessage,
	"C2C_MESSAGE_CREATE":      intentUserMessage,
	"INTERACTION_CREATE":      intentInteractions,
	"GROUP_JOIN_REQUEST":      intentUserMessage,
	"GROUP_MEMBER_ADD":        intentGroupMembers,
	"GROUP_MEMBER_REMOVE":     intentGroupMembers,
	"GROUP_ADD_ROBOT":         intentUserMessage,
	"GROUP_DEL_ROBOT":         intentUserMessage,
	"GROUP_MSG_RECEIVE":       intentUserMessage,
	"GROUP_MSG_REJECT":        intentUserMessage,
	"C2C_MSG_RECEIVE":         intentUserMessage,
	"C2C_MSG_REJECT":          intentUserMessage,
	"FRIEND_ADD":              intentUserMessage,
	"FRIEND_DEL":              intentUserMessage,
	"MESSAGE_AUDIT_PASS":      intentMessageAudit,
	"MESSAGE_AUDIT_REJECT":    intentMessageAudit,

	"GUILD_CREATE":          intentGuilds,
	"GUILD_UPDATE":          intentGuilds,
	"GUILD_DELETE":          intentGuilds,
	"CHANNEL_CREATE":        intentGuilds,
	"CHANNEL_UPDATE":        intentGuilds,
	"CHANNEL_DELETE":        intentGuilds,
	"GUILD_MEMBER_ADD":      intentGuildMembers,
	"GUILD_MEMBER_REMOVE":   intentGuildMembers,
	"AT_MESSAGE_CREATE":     intentPublicMessages,
	"MESSAGE_CREATE":        intentGuildMessages,
	"DIRECT_MESSAGE_CREATE": intentDirectMessage,
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
		"FRIEND_ADD",
		"FRIEND_DEL",
		"MESSAGE_AUDIT_PASS",
		"MESSAGE_AUDIT_REJECT",
		"AT_MESSAGE_CREATE",
		"MESSAGE_CREATE",
		"DIRECT_MESSAGE_CREATE",
		"GUILD_CREATE",
		"GUILD_UPDATE",
		"GUILD_DELETE",
		"CHANNEL_CREATE",
		"CHANNEL_UPDATE",
		"CHANNEL_DELETE",
		"GUILD_MEMBER_ADD",
		"GUILD_MEMBER_REMOVE",
	}
}

// Intents 将配置的事件名列表编译成 intent 位掩码。
func Intents(events []string) int {
	bits := 0
	for _, e := range events {
		bits |= eventIntentMap[e]
	}
	if bits == 0 {
		bits = defaultIntents
	}
	return bits
}

// RequiresWebSocket 订阅里是否含频道域事件; webhook 模式下等于"收得到、发不出", 启动校验据此直接拒掉。
func RequiresWebSocket(events []string) bool {
	for _, e := range events {
		if eventIntentMap[e]&guildIntents != 0 {
			return true
		}
	}
	return false
}
