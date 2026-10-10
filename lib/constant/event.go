package constant

type EventType string

// 群聊平台全部事件 (不含频道域)。
const (
	GROUP_AT_MESSAGE_CREATE EventType = "GROUP_AT_MESSAGE_CREATE"
	GROUP_MESSAGE_CREATE    EventType = "GROUP_MESSAGE_CREATE"
	C2C_MESSAGE_CREATE      EventType = "C2C_MESSAGE_CREATE"
	INTERACTION_CREATE      EventType = "INTERACTION_CREATE"
	GROUP_JOIN_REQUEST      EventType = "GROUP_JOIN_REQUEST"
	MESSAGE_AUDIT_PASS      EventType = "MESSAGE_AUDIT_PASS"
	MESSAGE_AUDIT_REJECT    EventType = "MESSAGE_AUDIT_REJECT"
	GROUP_MEMBER_ADD        EventType = "GROUP_MEMBER_ADD"
	GROUP_MEMBER_REMOVE     EventType = "GROUP_MEMBER_REMOVE"
	GROUP_ADD_ROBOT         EventType = "GROUP_ADD_ROBOT"
	GROUP_DEL_ROBOT         EventType = "GROUP_DEL_ROBOT"
	GROUP_MSG_RECEIVE       EventType = "GROUP_MSG_RECEIVE"
	GROUP_MSG_REJECT        EventType = "GROUP_MSG_REJECT"
	C2C_MSG_RECEIVE         EventType = "C2C_MSG_RECEIVE"
	C2C_MSG_REJECT          EventType = "C2C_MSG_REJECT"
	FRIEND_ADD              EventType = "FRIEND_ADD"
	FRIEND_DEL              EventType = "FRIEND_DEL"
)

// 交互事件的会话类型 (chat_type)
type ChatType int

const (
	ChatTypeGroup   ChatType = 1
	ChatTypeDirect  ChatType = 2
	ChatTypeChannel ChatType = 3
)

// 交互事件的场景标记 (scene); 与 chat_type 同义, 平台按场景下发其中一个。
const (
	SceneC2C   = "c2c"
	SceneGroup = "group"
	SceneGuild = "guild"
)
