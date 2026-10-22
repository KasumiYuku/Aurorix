package message

import (
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"os"
	"path"
	"strings"
)

// MediaMessage 已上传媒体（file_info 已知），直接发送。
type MediaMessage struct {
	*Message
	Media MediaContent `json:"media"`
}

type MediaContent struct {
	FileInfo string `json:"file_info"`
}

func (msg *MediaMessage) Marshal() ([]byte, error) {
	return json.Marshal(msg)
}

// Init 初始化消息结构体, 并把消息类型钉成媒体。
func (msg *MediaMessage) Init() {
	if msg.Message == nil {
		msg.Message = (&Message{}).InitRef()
	}
	msg.Message.Type = constant.Media
	msg.Message.MarshalInterface = msg
}

func (*MediaMessage) part() {}

// UploadMessage 语音/视频/文件：Send 时先上传 QQ 拿 file_info 再发媒体消息。
type UploadMessage struct {
	*Message
	FileType int    `json:"-"`
	Src      any    `json:"-"`
	Name     string `json:"-"`
}

func (*UploadMessage) part() {}

// Send 上传后发送媒体消息。
func (msg *UploadMessage) Send() error {
	_, err := msg.SendWithID()
	return err
}

// SendWithID 同 Send, 额外返回平台消息 ID。
//
// 发媒体走哪条路统一在这里定, 调用方只负责把东西递过来(字节或直链):
//   - 直链: QQ 自己去下载, 不占我们的上行, 是最快的一条。给的是字节且够大时, 先问图床换直链。
//   - 字节直传: 没有图床、图床不认这种媒体、或者直链那条不通(QQ 抓取超时 850027)时走这条。
func (msg *UploadMessage) SendWithID() (string, error) {
	up := MediaUploadFor(msg.FileType, msg.Src, msg.Name)
	link := up.URL
	if link == "" && msg.worthLink(len(up.Data)) {
		link = msg.publishLink(up.Data)
	}
	fileInfo, err := msg.upload(up, link)
	if err != nil && link != "" && len(up.Data) > 0 {
		// 直链那条不通 (QQ 抓取超时 850027): 字节还在手里, 换直传再试一次。
		logger.Warnf("直链没通, 改走直传(%d 字节): %v", len(up.Data), err)
		fileInfo, err = msg.upload(up, "")
	}
	if err != nil {
		return "", err
	}
	media := &MediaMessage{Message: msg.Message, Media: MediaContent{FileInfo: fileInfo}}
	media.Init()
	return media.SendWithID()
}

// upload 递交一次: 有直链就给链接, 否则给字节(超过阈值时 api 层自己分片)。
func (msg *UploadMessage) upload(base api.MediaUpload, link string) (string, error) {
	base.URL = link
	if link != "" {
		base.Data = nil
	}
	return msg.Qapi.UploadMedia(msg.Target, msg.GroupId, msg.UserId, base)
}

// worthLink 值不值得多跑一趟图床: 小文件内联上传比"先传图床再让 QQ 下载"快。
func (msg *UploadMessage) worthLink(n int) bool { return n > msg.Qapi.Threshold() }

// publishLink 把字节转存图床换直链; 没有图床、或图床不收这类媒体时给空串, 调用方回落直传。
func (msg *UploadMessage) publishLink(data []byte) string {
	kind, ok := mediaKind(msg.FileType)
	if !ok || msg.Qapi.Assets == nil {
		return ""
	}
	got, err := msg.Qapi.Assets.ResolveMedia(data, kind,
		assets.UploadOptions{GroupID: msg.GroupId, UserID: msg.UserId})
	if err != nil || got.URL == "" {
		logger.Warnf("图床转存%v失败, 改走直传: %v", kind, err)
		return ""
	}
	return got.URL
}

// mediaKind 把 QQ 的文件类型翻成图床的媒体大类; 普通文件没有对应的大类, 图床不收。
func mediaKind(fileType int) (assets.MediaKind, bool) {
	switch fileType {
	case constant.MediaVideo:
		return assets.KindVideo, true
	case constant.MediaVoice:
		return assets.KindAudio, true
	case constant.MediaFile:
		return 0, false
	}
	return assets.KindImage, true
}

// MediaUploadFor 构造 QQ 上传参数：公网 URL 直传，本地/data/base64/字节走智能解码。
func MediaUploadFor(fileType int, src any, name string) api.MediaUpload {
	up := api.MediaUpload{FileType: fileType, Filename: name}
	if up.Filename == "" {
		up.Filename = mediaName(fileType, src)
	}
	switch v := src.(type) {
	case []byte:
		up.Data = v
	case string:
		switch {
		case strings.HasPrefix(v, "http://"), strings.HasPrefix(v, "https://"):
			up.URL = v
		case strings.HasPrefix(v, "file://"):
			up.Data, _ = os.ReadFile(strings.TrimPrefix(v, "file://"))
		default:
			if in, err := assets.Decode(v); err == nil && len(in.Data) > 0 {
				up.Data = in.Data
			} else if in.URL != "" {
				up.URL = in.URL
			}
		}
	}
	return up
}

func mediaName(fileType int, src any) string {
	switch fileType {
	case constant.MediaVideo:
		return "video"
	case constant.MediaVoice:
		return "voice"
	case constant.MediaFile:
		return "file"
	}
	s, _ := src.(string)
	if name := path.Base(strings.TrimPrefix(s, "file://")); name != "" && name != "." {
		return name
	}
	return "image"
}
