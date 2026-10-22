package assets

import (
	"encoding/base64"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/images"
	"os"
	"path"
	"strings"
)

// MediaInput 解码结果。
type MediaInput struct {
	Data     []byte
	URL      string
	Filename string
	MimeType string
}

// ProbeMediaMime 探测媒体 MIME: 先按图片魔数, 再按视频容器。
func ProbeMediaMime(buf []byte) string {
	if mime := images.ProbeMime(buf); mime != "" {
		return mime
	}
	return probeVideoMime(buf)
}

// probeVideoMime 认 ISO-BMFF(mp4/mov) 与 Matroska(webm) 的常见容器魔数。
func probeVideoMime(buf []byte) string {
	if len(buf) >= 12 && string(buf[4:8]) == "ftyp" {
		switch string(buf[8:12]) {
		case "qt  ":
			return "video/quicktime"
		case "heic", "heix", "mif1", "msf1":
			return "image/heic"
		default:
			return "video/mp4"
		}
	}
	if len(buf) >= 4 && buf[0] == 0x1a && buf[1] == 0x45 && buf[2] == 0xdf && buf[3] == 0xa3 {
		return "video/webm"
	}
	return ""
}

// mediaExt 是 MIME → 扩展名; 图片沿用 images 包, 视频与音频在这里补齐。
func mediaExt(mime string) string {
	if ext := images.ExtFromMime(mime); ext != "" {
		return ext
	}
	switch mime {
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "video/quicktime":
		return "mov"
	case "audio/mpeg":
		return "mp3"
	case "audio/ogg":
		return "ogg"
	}
	return ""
}

// Decode 通用媒体输入归一化：[]byte/data URL/裸 base64/本地路径/公网 URL。
func Decode(src any) (MediaInput, error) {
	switch v := src.(type) {
	case []byte:
		if len(v) == 0 {
			return MediaInput{}, fmt.Errorf("空媒体数据")
		}
		return MediaInput{Data: v, Filename: "inline"}, nil
	case string:
		return decodeString(v, false)
	default:
		return MediaInput{}, fmt.Errorf("不支持的媒体类型 %T", src)
	}
}

// DecodeImage 图片输入归一化，魔数校验确保为图片。
func DecodeImage(src any) (MediaInput, error) {
	switch v := src.(type) {
	case []byte:
		if !images.IsImage(v) {
			return MediaInput{}, fmt.Errorf("字节非图片")
		}
		return MediaInput{Data: v, Filename: "inline"}, nil
	case string:
		return decodeString(v, true)
	default:
		return MediaInput{}, fmt.Errorf("不支持的图片类型 %T", src)
	}
}

func decodeString(src string, wantImage bool) (MediaInput, error) {
	s := strings.TrimSpace(src)
	switch {
	case strings.HasPrefix(s, "data:"):
		return decodeDataURL(s, wantImage)
	case strings.HasPrefix(s, "http://"), strings.HasPrefix(s, "https://"):
		return MediaInput{URL: s, Filename: path.Base(s)}, nil
	}
	s = strings.TrimPrefix(s, "file://")
	if b, ok := decodeBase64Lenient(s, wantImage); ok {
		return MediaInput{Data: b, Filename: "inline"}, nil
	}
	b, err := os.ReadFile(s)
	if err != nil {
		return MediaInput{}, fmt.Errorf("读取媒体失败: %w", err)
	}
	if wantImage && !images.IsImage(b) {
		return MediaInput{}, fmt.Errorf("文件非图片")
	}
	return MediaInput{Data: b, Filename: path.Base(s)}, nil
}

func decodeDataURL(src string, wantImage bool) (MediaInput, error) {
	comma := strings.IndexByte(src, ',')
	if comma < 0 || comma == len(src)-1 {
		return MediaInput{}, fmt.Errorf("非法 data URL")
	}
	meta := src[len("data:"):comma]
	payload := src[comma+1:]
	mime := strings.SplitN(meta, ";", 2)[0]
	if strings.Contains(meta, ";base64") {
		b, ok := decodeBase64Lenient(payload, wantImage)
		if !ok {
			return MediaInput{}, fmt.Errorf("data URL base64 解码失败")
		}
		return MediaInput{Data: b, Filename: "inline", MimeType: mime}, nil
	}
	return MediaInput{Data: []byte(payload), Filename: "inline", MimeType: mime}, nil
}

func decodeBase64Lenient(s string, wantImage bool) ([]byte, bool) {
	s = strings.TrimPrefix(s, "base64,")
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, s)
	if len(s) == 0 {
		return nil, false
	}
	ok := func(b []byte) bool {
		if wantImage {
			return images.IsImage(b)
		}
		return len(b) > 0
	}
	encs := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, enc := range encs {
		b, err := enc.DecodeString(s)
		if err != nil || !ok(b) {
			continue
		}
		return b, true
	}
	if pad := len(s) % 4; pad != 0 {
		b, err := base64.StdEncoding.DecodeString(s + strings.Repeat("=", 4-pad))
		if err == nil && ok(b) {
			return b, true
		}
	}
	return nil, false
}
