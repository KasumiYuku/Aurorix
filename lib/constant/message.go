package constant

// 消息来源/发送目标
type MessageOrigin uint8

const (
	GroupMessage MessageOrigin = iota
	PrivateMessage
)

type MessageType uint8

const (
	PlainText MessageType = 0
	Markdown  MessageType = 2
	Media     MessageType = 7
)

// QQ 媒体上传 file_type（与消息 msg_type 无关，纯上传通道标识）
const (
	MediaImage = 1
	MediaVideo = 2
	MediaVoice = 3
	MediaFile  = 4
)
