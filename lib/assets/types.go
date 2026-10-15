package assets

import "context"

// ProviderInput 上传输入：图片字节流或公网 URL（二选一）。
type ProviderInput struct {
	Buffer   []byte
	URL      string
	Filename string
	MimeType string
	Origin   string
	GroupID  string
	UserID   string
}

// ResolvedImage 图床转换结果。
type ResolvedImage struct {
	URL           string
	Width         int
	Height        int
	Provider      string
	SessionScoped bool
}

// ImageProvider 图床契约：收图片，返回公网 URL。
type ImageProvider interface {
	Name() string
	Upload(ctx context.Context, in ProviderInput) (string, error)
}

// ProviderFactory 图床工厂：注入 HTTP 客户端与配置实例化 provider。
type ProviderFactory func(cl *Client, config map[string]any) (ImageProvider, error)

// UploadOptions 是一次图床转存的选项。
type UploadOptions struct {
	// Provider 指定只用这个图床；空表示按配置优先级轮询，指定了就不回落。
	Provider string
	GroupID  string
	UserID   string
}
