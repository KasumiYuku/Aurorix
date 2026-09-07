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

// decodeDataURL 解析 data:image/png;base64,xxx 与 data:,xxx 两种形式。
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

// decodeBase64Lenient 智能解码：容忍残留前缀、空白、url-safe、缺 padding。
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
		// 媒体解码：至少通过魔数或长度合理性兜底
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
