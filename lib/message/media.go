package message

import (
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/assets"
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

func (msg *MediaMessage) Init() {
	if msg.Message == nil {
		msg.Message = (&Message{}).InitRef()
	}
	msg.Message.MarshalInterface = msg
}

func (*MediaMessage) part() {}

// UploadMessage 语音/视频/文件：Send 时先上传 QQ 拿 file_info 再发媒体消息。
type UploadMessage struct {
	*Message
	FileType int    `json:"-"` // 2视频 3语音 4文件
	Src      any    `json:"-"` // string(路径/URL/data/base64) 或 []byte
	Name     string `json:"-"`
}

func (*UploadMessage) part() {}

// Send 上传后发送媒体消息。
func (msg *UploadMessage) Send() error {
	fileInfo, err := msg.Qapi.UploadMedia(msg.Target, msg.GroupId, msg.UserId, MediaUploadFor(msg.FileType, msg.Src, msg.Name))
	if err != nil {
		return err
	}
	media := &MediaMessage{Message: msg.Message, Media: MediaContent{FileInfo: fileInfo}}
	media.Init()
	return media.Send()
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
			// 本地路径或 data/base64 统一走 assets 智能解码
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
