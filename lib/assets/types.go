package assets

import (
	"context"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// MediaKind 是媒体大类; provider 用它声明自己能收什么。
type MediaKind uint8

const (
	KindImage MediaKind = 1 << iota
	KindVideo
	KindAudio
)

// Has 判断是否包含某一大类。
func (k MediaKind) Has(want MediaKind) bool { return k&want != 0 }

// String 给日志与报错用。
func (k MediaKind) String() string {
	switch k {
	case KindImage:
		return "图片"
	case KindVideo:
		return "视频"
	case KindAudio:
		return "音频"
	}
	return "媒体"
}

// KindOf 从 MIME 归到某个大类; 认不出时按图片算, 与历史行为一致。
func KindOf(mime string) MediaKind {
	switch {
	case len(mime) >= 6 && mime[:6] == "video/":
		return KindVideo
	case len(mime) >= 6 && mime[:6] == "audio/":
		return KindAudio
	default:
		return KindImage
	}
}

// ProviderInput 上传输入：字节流或公网 URL（二选一）。
type ProviderInput struct {
	Buffer   []byte
	URL      string
	Filename string
	MimeType string
	Kind     MediaKind
	Origin   string
	// Target 这次转换是为哪个会话做的。会话级图床(官方)靠它取上传目标, 频道域没有这种目标。
	Target constant.Target
	// Trace 与 Command 只为日志服务: 一次图床转换要能说清"是哪条消息、哪个指令触发的"。
	Trace   string
	Command string
}

func (in ProviderInput) source() string {
	parts := make([]string, 0, 4)
	if !in.Target.Empty() {
		parts = append(parts, "会话="+in.Target.String())
	}
	if in.Target.User != "" {
		parts = append(parts, "user="+in.Target.User)
	}
	if in.Command != "" {
		parts = append(parts, "cmd="+in.Command)
	}
	if len(parts) == 0 {
		return "来源未知"
	}
	return strings.Join(parts, " ")
}

// ResolvedImage 图床转换结果。
type ResolvedImage struct {
	URL           string
	Width         int
	Height        int
	Provider      string
	SessionScoped bool
}

// ResolvedMedia 媒体转存结果; 视频没有宽高, 所以不复用 ResolvedImage。
type ResolvedMedia struct {
	URL      string
	Provider string
	Kind     MediaKind
}

// ImageProvider 图床契约：收媒体，返回公网 URL。
type ImageProvider interface {
	Name() string
	Upload(ctx context.Context, in ProviderInput) (string, error)
}

// MediaProvider 在 ImageProvider 之上声明能传哪些大类、各自多大上限; 不实现它的 provider 按"只收图片、不限大小"处理。
type MediaProvider interface {
	ImageProvider
	Kinds() MediaKind
	MaxSize(kind MediaKind) int64
}

// ProviderFactory 图床工厂：注入 HTTP 客户端与配置实例化 provider。
type ProviderFactory func(cl *Client, config map[string]any) (ImageProvider, error)

// UploadOptions 是一次图床转存的选项。
type UploadOptions struct {
	// Provider 指定只用这个图床；空表示按配置优先级轮询，指定了就不回落。
	Provider string
	// Target 这次转换服务的会话; 会话级图床(官方)只能发在群/单聊, 频道域会被跳过。
	Target constant.Target
	// Trace 一次触发的短标识; 图床日志靠它跟触发它的那条消息对齐。
	Trace string
	// Command 触发这次转换的指令路径(可选), 只用于日志。
	Command string
}

func (opt UploadOptions) stamp(in ProviderInput) ProviderInput {
	in.Target = opt.Target
	in.Trace = opt.Trace
	in.Command = opt.Command
	return in
}
