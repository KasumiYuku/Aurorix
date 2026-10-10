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
func (msg *UploadMessage) SendWithID() (string, error) {
	fileInfo, err := msg.Qapi.UploadMedia(msg.Target, msg.GroupId, msg.UserId, MediaUploadFor(msg.FileType, msg.Src, msg.Name))
	if err != nil {
		return "", err
	}
	media := &MediaMessage{Message: msg.Message, Media: MediaContent{FileInfo: fileInfo}}
	media.Init()
	return media.SendWithID()
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
	case 2:
		return "video"
	case 3:
		return "voice"
	case 4:
		return "file"
	}
	s, _ := src.(string)
	if name := path.Base(strings.TrimPrefix(s, "file://")); name != "" && name != "." {
		return name
	}
	return "image"
}
