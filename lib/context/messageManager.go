package context

import (
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/templates"
)

// Sender 可发送的消息对象（Text/Markdown/Media 均实现）。
type Sender interface {
	Send() error
}

type MessageManager struct {
	MessageId string
	EventId   string
	GroupId   string
	UserId    string
	Target    constant.MessageOrigin
	Qapi      *api.BotAPI
	ref       *message.MsgRef
	// PluginId 当前处理链路的插件归属; 模板解析时优先插件命名空间。
	PluginId string
}

// 生成包含元信息的消息结构
func (manager *MessageManager) baseStruct() *message.Message {
	msg := &message.Message{
		EventId: manager.EventId,
		MsgId:   manager.MessageId,
		Qapi:    manager.Qapi,
		GroupId: manager.GroupId,
		UserId:  manager.UserId,
		Target:  manager.Target,
	}
	// 确保引用的是同一个计数器
	if manager.ref == nil {
		msg.InitRef()
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
	msg.Markdown = templates.Markdown{
		Content: content,
	}
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

// At 构造艾特部件; newline 为 true 时 markdown 中 @ 后补换行独占一行。
func (manager *MessageManager) At(openid string, newline ...bool) *message.AtMessage {
	msg := &message.AtMessage{
		Message: manager.baseStruct(),
		OpenID:  openid,
		Newline: len(newline) > 0 && newline[0],
	}
	msg.Type = constant.PlainText
	return msg
}

// AtAll 构造 @所有人 部件; newline 同上。
func (manager *MessageManager) AtAll(newline ...bool) *message.AtMessage {
	msg := &message.AtMessage{
		Message: manager.baseStruct(),
		All:     true,
		Newline: len(newline) > 0 && newline[0],
	}
	msg.Type = constant.PlainText
	return msg
}

// Image 构造图片部件；src 支持 string(路径/data/base64/公网URL) 与 []byte。
// size 可选：传 2 个值 (宽,高) 时作为显式尺寸标注；不传则按图床探测/URL 探测自动补齐。
// 发送形态（markdown 内嵌 / 独立媒体）由 Send 时按上下文决策。
func (manager *MessageManager) Image(src any, summary string, size ...int) *message.ImageMessage {
	msg := &message.ImageMessage{
		Message: manager.baseStruct(),
		Src:     src,
		Summary: summary,
	}
	if len(size) >= 2 {
		msg.Width, msg.Height = size[0], size[1]
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
// 插件命名空间优先, 未命中回落全局。
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
