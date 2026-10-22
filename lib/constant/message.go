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

// DefaultUploadThreshold 是字节直传与分片上传的分界: 媒体超过它改走 upload_prepare 分片,
// 不超过则一次内联(省掉一轮 prepare/分片往返)。QQ 对单次内联的 body 有大小限制, 3MB 留足余量。
//
// 它同时是"值不值得先换直链"的判据 —— 反正都要分片了, 不如先让 QQ 自己去下载。
const DefaultUploadThreshold = 3 << 20

// 默认的 markdown 图片显示框: 350px 顶满主流手机聊天气泡宽度, 438px 约 5:4。
// 宽度顶到上限、高度封顶, 长截图既不会刷屏, 也不会被挤成细长柱子。
const (
	DefaultImageMaxWidth  = 350
	DefaultImageMaxHeight = 438
)
