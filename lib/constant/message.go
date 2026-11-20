package constant

// DefaultMaintenanceMessage 维护开关打开但没写文案时的兜底话术, 全框架只此一份。
const DefaultMaintenanceMessage = "当前该功能模块正在维护中，请等待维护完成再重试"

// DefaultDeniedMessage 名单拦下、且调用方要求回话但没写文案时的兜底话术; 与维护话术分开: 维护是"暂时关掉", 名单是"你不在范围内"。
const DefaultDeniedMessage = "你没有权限使用该指令"

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

// DefaultUploadThreshold 字节直传与分片上传的分界(3MB); 超过它走 upload_prepare 分片, 也是"值不值得先换直链"的判据。
const DefaultUploadThreshold = 3 << 20

// 宽度顶到上限、高度封顶, 长截图既不会刷屏, 也不会被挤成细长柱子。
const (
	DefaultImageMaxWidth  = 350
	DefaultImageMaxHeight = 438
)
