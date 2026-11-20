package context

import (
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/structers"
	"github.com/KasumiYuku/Aurorix/lib/templates"
)

// Sender 可发送的消息对象（Text/Markdown/Media 均实现）。
type Sender interface {
	Send() error
	SendWithID() (string, error)
}

type MessageManager struct {
	MessageId string
	EventId   string
	// Target 这次回复落在哪个会话: 场景 + 会话 ID + 触发者。
	Target   constant.Target
	Qapi     *api.BotAPI
	ref      *message.MsgRef
	PluginId string
	// OriginalInput 触发这条回复的原消息文本(主动发送/主动推送时为空); 底部「复读」按钮用它回填。
	OriginalInput string
	// NoFooter 为 true 时这条回复不追加框架级底部按钮(指令声明豁免时由框架置上)。
	NoFooter bool
	// Trace 一次触发的短标识, 连同 Command 一起传给下游(图床/降级日志), 让一条消息的链路能串起来。
	Trace   string
	Command string

	deliveryMu sync.Mutex
	delivery   message.Delivery
}

func (manager *MessageManager) noteDelivery(d message.Delivery) {
	manager.deliveryMu.Lock()
	manager.delivery = d
	manager.deliveryMu.Unlock()
}

// LatestDelivery 最近一次发送的结果; 判断这次算不算替换成功用它, 主动投递可能已经落到私聊。
func (manager *MessageManager) LatestDelivery() message.Delivery {
	manager.deliveryMu.Lock()
	defer manager.deliveryMu.Unlock()
	return manager.delivery
}

// CanReply 表示当前上下文带有平台给的被动回复锚点。
func (manager *MessageManager) CanReply() bool {
	return manager != nil && (manager.MessageId != "" || manager.EventId != "")
}

func (manager *MessageManager) baseStruct() *message.Message {
	msg := &message.Message{
		EventId:       manager.EventId,
		MsgId:         manager.MessageId,
		Qapi:          manager.Qapi,
		Target:        manager.Target,
		OriginalInput: manager.OriginalInput,
		NoFooter:      manager.NoFooter,
		Trace:         manager.Trace,
		Command:       manager.Command,
	}
	msg.InitRef()
	if manager.ref == nil {
		manager.ref = msg.MsgRef
	} else {
		msg.MsgRef = manager.ref
	}
	return msg
}

// Text 生成纯文本回复消息；全局 markdown 开启时自动转 markdown。
func (manager *MessageManager) Text(content string) *message.TextMessage {
	metamsg := manager.baseStruct()
	msg := message.TextMessage{
		Message: metamsg,
	}
	msg.Type = constant.PlainText
	msg.Content(content)
	msg.Init()
	if manager.Qapi != nil && manager.Qapi.GlobalMarkdown {
		msg.MarkdownMode = true
	}
	return &msg
}

// Markdown 生成 Markdown 回复消息。
func (manager *MessageManager) Markdown(content string) *message.MarkdownMessage {
	metamsg := manager.baseStruct()
	msg := message.MarkdownMessage{
		Message: metamsg,
	}
	msg.Init()
	msg.Type = constant.Markdown
	msg.Content(content)
	if manager.Qapi != nil {
		msg.SetAssets(manager.Qapi.Assets)
	}
	return &msg
}

// Media 构造已上传媒体消息（file_info 来自 QQ 上传接口）。
func (manager *MessageManager) Media(fileInfo string) *message.MediaMessage {
	msg := &message.MediaMessage{
		Message: manager.baseStruct(),
		Media: message.MediaContent{
			FileInfo: fileInfo,
		},
	}
	msg.Type = constant.Media
	msg.Init()
	return msg
}

// At 构造艾特部件, newline 为 true 时 @ 后补换行; 传 union_openid 即可, 频道与群用两套 ID 的事由框架换。
func (manager *MessageManager) At(openid string, newline ...bool) *message.AtMessage {
	msg := &message.AtMessage{
		Message: manager.baseStruct(),
		OpenID:  manager.mentionIDFor(openid),
		Newline: len(newline) > 0 && newline[0],
	}
	msg.Init()
	return msg
}

func (manager *MessageManager) mentionIDFor(openid string) string {
	if manager.Target.MentionID != "" && openid == manager.Target.User {
		return manager.Target.MentionID
	}
	return openid
}

// AtQuote 构造"引用块里的艾特"部件: markdown 里写成 "> <@openid>"。
func (manager *MessageManager) AtQuote(openid string, newline ...bool) *message.AtMessage {
	msg := manager.At(openid, newline...)
	msg.Quote = true
	return msg
}

// AtAll 构造 @所有人 部件; newline 同上。
func (manager *MessageManager) AtAll(newline ...bool) *message.AtMessage {
	msg := &message.AtMessage{
		Message: manager.baseStruct(),
		All:     true,
		Newline: len(newline) > 0 && newline[0],
	}
	msg.Init()
	return msg
}

const (
	avatarFetchPx = 140
	avatarShowPx  = 30
)

// AtAvatar 构造"带头像的艾特"部件; px 是头像显示边长, 不传按 30。
func (manager *MessageManager) AtAvatar(openid string, px ...int) *message.AtMessage {
	msg := manager.AtQuote(openid)
	msg.Newline = true
	msg.AvatarPx = avatarShowPx
	if len(px) > 0 && px[0] > 0 {
		msg.AvatarPx = px[0]
	}
	if manager.Qapi != nil && manager.Qapi.AppID != "" {
		msg.AvatarURL = structers.AvatarURLSize(manager.Qapi.AppID, openid, avatarFetchPx)
		msg.Heading = 3
	}
	return msg
}

// Image 构造图片部件；src 支持 string(路径/data/base64/公网URL) 与 []byte。
func (manager *MessageManager) Image(src any, summary string, size ...int) *message.ImageMessage {
	msg := &message.ImageMessage{
		Message: manager.baseStruct(),
		Src:     src,
		Summary: summary,
	}
	if len(size) >= 2 {
		msg.Size(size[0], size[1])
	}
	return msg
}

// Voice 构造语音部件；src 支持路径/URL/data/base64/[]byte。
func (manager *MessageManager) Voice(src any) *message.UploadMessage {
	return manager.upload(constant.MediaVoice, src, "voice")
}

// Video 构造视频部件。
func (manager *MessageManager) Video(src any) *message.UploadMessage {
	return manager.upload(constant.MediaVideo, src, "video")
}

// File 构造文件部件。
func (manager *MessageManager) File(src any, name string) *message.UploadMessage {
	return manager.upload(constant.MediaFile, src, name)
}

func (manager *MessageManager) upload(fileType int, src any, name string) *message.UploadMessage {
	msg := &message.UploadMessage{
		Message:  manager.baseStruct(),
		FileType: fileType,
		Src:      src,
		Name:     name,
	}
	return msg
}

// MarkdownTemplate 填充 Markdown 模板并构造消息。
func (manager *MessageManager) MarkdownTemplate(id string, args *templates.Args) (*message.MarkdownMessage, error) {
	var content string
	var err error
	if args == nil {
		content, err = templates.FillFor(manager.PluginId, id, templates.Args{})
	} else {
		content, err = templates.FillFor(manager.PluginId, id, *args)
	}
	if err != nil {
		return nil, err
	}
	return manager.Markdown(content), nil
}

// UnsafeMarkdownTemplate 填充失败时 panic。
func (manager *MessageManager) UnsafeMarkdownTemplate(id string, args *templates.Args) *message.MarkdownMessage {
	content, err := templates.FillFor(manager.PluginId, id, *args)
	if err != nil {
		panic(err)
	}
	return manager.Markdown(content)
}
