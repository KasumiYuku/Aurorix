package assets

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/images"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/requests"
)

var logAssets = logx.New("assets")

var imgRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

var dimRe = regexp.MustCompile(`#\d+px\s*#\d+px`)

// NewMultipart 便捷构造 multipart 请求体。
func NewMultipart() *requests.Multipart { return requests.NewMultipart() }

// Client 图床 HTTP 客户端，包装 requests.Client。
type Client struct {
	*requests.Client
}

// NewClient 创建图床专用客户端。
func NewClient(timeout int) *Client {
	return &Client{Client: requests.Init(timeout)}
}

// ImageHost 图床聚合器：按配置顺序（优先级从高到低）依次尝试，失败自动切换下一个。
type ImageHost struct {
	providers []ImageProvider
	skipProbe map[string]bool
	whitelist []string
	uploaded  int64
	fetch     *http.Client
	head      *http.Client
}

const maxFetchBytes = 20 << 20

// HostConfig 图床配置，对应独立 assets.json（不入 git）。
type HostConfig struct {
	Providers []ProviderItem `json:"providers"`
	Whitelist []string       `json:"whitelist"`
}

// ProviderItem 单个图床 provider 配置项。Priority 越大越优先，同级保持配置顺序。
type ProviderItem struct {
	Name     string         `json:"name"`
	Enabled  *bool          `json:"enabled,omitempty"`
	Priority int            `json:"priority"`
	Probe    *bool          `json:"probe,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
}

// NewHost 从配置创建图床聚合器：过滤未启用的 provider
func NewHost(cfg HostConfig, cl *Client) *ImageHost {
	h := &ImageHost{
		skipProbe: make(map[string]bool),
		whitelist: cfg.Whitelist,
		fetch:     &http.Client{Timeout: 15 * time.Second},
		head: &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
	enabled := make([]ProviderItem, 0, len(cfg.Providers))
	for _, item := range cfg.Providers {
		if item.Enabled != nil && !*item.Enabled {
			continue
		}
		enabled = append(enabled, item)
	}
	slices.SortStableFunc(enabled, func(a, b ProviderItem) int {
		return cmp.Compare(b.Priority, a.Priority)
	})
	for _, item := range enabled {
		p, err := Instantiate(item.Name, cl, item.Config)
		if err != nil {
			continue
		}
		switch {
		case item.Probe != nil && !*item.Probe:
			h.skipProbe[item.Name] = true
		case item.Probe == nil && ProbeSkippedByDefault(item.Name):
			h.skipProbe[item.Name] = true
		}
		h.providers = append(h.providers, p)
	}

	configured := make(map[string]bool, len(cfg.Providers))
	for _, item := range cfg.Providers {
		configured[item.Name] = true
	}
	for _, name := range Names() {
		if configured[name] || !IsDefaultOn(name) {
			continue
		}
		p, err := Instantiate(name, cl, nil)
		if err != nil {
			continue
		}
		if ProbeSkippedByDefault(name) {
			h.skipProbe[name] = true
		}
		h.providers = append(h.providers, p)
	}
	return h
}

func newHostWithProviders(providers []ImageProvider, whitelist []string) *ImageHost {
	return &ImageHost{
		providers: providers,
		skipProbe: make(map[string]bool),
		whitelist: whitelist,
		fetch:     &http.Client{Timeout: 15 * time.Second},
		head: &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
}

// Size 已配置的 provider 数量（0 表示未启用图床）。
func (h *ImageHost) Size() int { return len(h.providers) }

// Stats 返回本次进程累计上传数。
func (h *ImageHost) Stats() int64 { return h.uploaded }

// Resolve 任意图片输入转公网 URL（[]byte/路径/data/base64），公网 URL 直通。
func (h *ImageHost) Resolve(src any) (ResolvedImage, error) {
	return h.ResolveTo(src, "", "")
}

// ResolveTo 同 Resolve，额外带上会话目标 openid。
func (h *ImageHost) ResolveTo(src any, groupID, userID string) (ResolvedImage, error) {
	return h.ResolveWith(src, UploadOptions{GroupID: groupID, UserID: userID})
}

// ResolveWith 同 ResolveTo，另外可以指定图床（见 UploadOptions.Provider）。
func (h *ImageHost) ResolveWith(src any, opt UploadOptions) (ResolvedImage, error) {
	if s, ok := src.(string); ok {
		return h.ImgToURLWith(s, opt)
	}
	in, err := DecodeImage(src)
	if err != nil {
		return ResolvedImage{}, err
	}
	if in.MimeType == "" {
		if m := images.ProbeMime(in.Data); m != "" {
			in.MimeType = m
			if in.Filename == "" || !hasExt(in.Filename) {
				in.Filename = "inline." + images.ExtFromMime(m)
			}
		}
	}
	url, provider, providerSessionScoped, err := h.uploadTo(ProviderInput{
		Buffer: in.Data, Filename: in.Filename, MimeType: in.MimeType,
		Origin: "内存字节", GroupID: opt.GroupID, UserID: opt.UserID,
	}, opt.Provider)
	if err != nil {
		return ResolvedImage{}, err
	}
	width, height := 0, 0
	if sz := images.Probe(in.Data); sz != nil {
		width, height = sz.Width, sz.Height
	}
	return ResolvedImage{URL: url, Width: width, Height: height, Provider: provider, SessionScoped: providerSessionScoped}, nil
}

func hasExt(name string) bool {
	return strings.LastIndex(name, ".") > strings.LastIndex(name, "/")
}

// ImgToURL 把图片 src 转成公网 URL。
func (h *ImageHost) ImgToURL(src string) (ResolvedImage, error) {
	return h.ImgToURLTo(src, "", "")
}

// ImgToURLTo 同 ImgToURL，额外带上会话目标 openid（见 ResolveTo）。
func (h *ImageHost) ImgToURLTo(src string, groupID, userID string) (ResolvedImage, error) {
	return h.ImgToURLWith(src, UploadOptions{GroupID: groupID, UserID: userID})
}

// ImgToURLWith 同 ImgToURLTo，另外可以指定图床（见 UploadOptions.Provider）。
func (h *ImageHost) ImgToURLWith(src string, opt UploadOptions) (ResolvedImage, error) {
	for _, prefix := range h.whitelist {
		if strings.HasPrefix(src, prefix) {
			return ResolvedImage{URL: src}, nil
		}
	}
	if len(h.providers) == 0 || !isLocal(src) {
		return ResolvedImage{URL: src}, nil
	}

	input, err := h.load(src, opt.GroupID, opt.UserID)
	if err != nil {
		return ResolvedImage{URL: src}, nil
	}

	url, provider, providerSessionScoped, err := h.uploadTo(input, opt.Provider)
	if err != nil {
		return ResolvedImage{URL: src}, err
	}

	width, height := 0, 0
	if sz := images.Probe(input.Buffer); sz != nil {
		width, height = sz.Width, sz.Height
	}
	return ResolvedImage{URL: url, Width: width, Height: height, Provider: provider, SessionScoped: providerSessionScoped}, nil
}

// ProcessMarkdown 处理 markdown 中的图片引用：local/base64 上传图床，替换为
func (h *ImageHost) ProcessMarkdown(input string) string {
	return h.ProcessMarkdownTo(input, "", "")
}

// ProcessMarkdownTo 同 ProcessMarkdown，额外带上会话目标 openid（见 ResolveTo）。
func (h *ImageHost) ProcessMarkdownTo(input string, groupID, userID string) string {
	if h == nil || len(h.providers) == 0 {
		return input
	}
	matched := imgRe.FindAllStringSubmatchIndex(input, -1)
	if len(matched) == 0 {
		return input
	}
	var b strings.Builder
	b.Grow(len(input))
	last := 0
	for _, loc := range matched {
		start, end := loc[0], loc[1]
		b.WriteString(input[last:start])
		alt, src := input[loc[2]:loc[3]], input[loc[4]:loc[5]]
		b.WriteString(h.replaceImage(alt, src, groupID, userID))
		last = end
	}
	b.WriteString(input[last:])
	return b.String()
}

func (h *ImageHost) replaceImage(alt, src, groupID, userID string) string {
	resolved, err := h.ImgToURLTo(src, groupID, userID)
	if err != nil {
		return fmt.Sprintf("![%s](%s)", alt, src)
	}
	if resolved.URL == src {
		return fmt.Sprintf("![%s](%s)", alt, src)
	}
	if resolved.Width > 0 && resolved.Height > 0 && !dimRe.MatchString(alt) {
		return fmt.Sprintf("![%s #%dpx #%dpx](%s)", alt, resolved.Width, resolved.Height, resolved.URL)
	}
	return fmt.Sprintf("![%s](%s)", alt, resolved.URL)
}

func isLocal(src string) bool {
	if strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "file://") {
		return true
	}
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		return true
	}
	u, err := url.Parse(src)
	if err != nil {
		return true
	}
	host := u.Hostname()
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified()
}

func (h *ImageHost) load(src, groupID, userID string) (ProviderInput, error) {
	in, err := DecodeImage(src)
	if err != nil {
		return ProviderInput{}, err
	}
	if in.URL != "" {
		return h.fetchRemote(in.URL, groupID, userID)
	}
	return ProviderInput{
		Buffer:   in.Data,
		Filename: in.Filename,
		MimeType: in.MimeType,
		Origin:   originOf(src),
		GroupID:  groupID,
		UserID:   userID,
	}, nil
}

func originOf(src string) string {
	switch {
	case strings.HasPrefix(src, "data:"):
		return "base64 data URL"
	case strings.HasPrefix(src, "file://"):
		return "本地文件 " + strings.TrimPrefix(src, "file://")
	default:
		return "本地文件 " + src
	}
}

func (h *ImageHost) fetchRemote(src, groupID, userID string) (ProviderInput, error) {
	resp, err := h.fetch.Get(src)
	if err != nil {
		return ProviderInput{}, fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ProviderInput{}, fmt.Errorf("download image: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
	if err != nil {
		return ProviderInput{}, fmt.Errorf("read image body: %w", err)
	}
	if len(data) > maxFetchBytes {
		return ProviderInput{}, fmt.Errorf("image exceeds %d bytes", maxFetchBytes)
	}

	return ProviderInput{
		Buffer:   data,
		Filename: path.Base(src),
		MimeType: resp.Header.Get("Content-Type"),
		Origin:   "公网 URL " + src,
		GroupID:  groupID,
		UserID:   userID,
	}, nil
}

func (h *ImageHost) upload(input ProviderInput) (string, string, bool, error) {
	return h.uploadTo(input, "")
}

// uploadTo 只试 candidates 挑出来的图床。
func (h *ImageHost) uploadTo(input ProviderInput, provider string) (string, string, bool, error) {
	providers, err := h.candidates(provider)
	if err != nil {
		return "", "", false, err
	}
	var errs []error
	ctx := context.Background()
	desc := describeInput(input)
	for _, p := range providers {
		start := time.Now()
		url, err := p.Upload(ctx, input)
		upCost := time.Since(start)
		if err != nil {
			logAssets.Warnf("图床转换失败: provider=%s, %s, 上传=%s, err=%v",
				p.Name(), desc, upCost.Round(time.Millisecond), err)
			errs = append(errs, fmt.Errorf("[%s] %w", p.Name(), err))
			continue
		}
		original := url
		var resolved, ct string
		var headOK bool
		var probeCost time.Duration
		skipProbe := h.skipProbe[p.Name()]
		if !skipProbe {
			probeStart := time.Now()
			resolved, ct, headOK = h.probeURL(url)
			probeCost = time.Since(probeStart)
			if resolved != "" {
				url = resolved
			}
			if headOK && !isImageContentType(ct) {
				logAssets.Warnf("图床 Content-Type 不匹配, 视为失败: provider=%s, %s, url=%s, content-type=%q, 上传=%s, 探测=%s",
					p.Name(), desc, url, ct,
					upCost.Round(time.Millisecond), probeCost.Round(time.Millisecond))
				errs = append(errs, fmt.Errorf("[%s] final url content-type %q is not image/*", p.Name(), ct))
				continue
			}
		}
		h.uploaded++
		state := probeState(headOK, ct)
		if skipProbe {
			state = "已按配置跳过探测"
		}
		logAssets.Infof("图床转换成功: provider=%s, %s, 上传=%s, 探测=%s(%s), 合计=%s, 返回 URL=%s%s",
			p.Name(), desc,
			upCost.Round(time.Millisecond), probeCost.Round(time.Millisecond), state,
			(upCost + probeCost).Round(time.Millisecond), original, followedNote(original, url))
		if !skipProbe && !headOK {
			logAssets.Warnf("最终 URL HEAD 不可达, 原样使用: url=%s", url)
		}
		return url, p.Name(), sessionScoped(p), nil
	}
	return "", "", false, fmt.Errorf("all providers failed: %w", errors.Join(errs...))
}

// candidates 挑出这一次要试的图床；provider 为空取全部，指定了就必须命中。
func (h *ImageHost) candidates(provider string) ([]ImageProvider, error) {
	if len(h.providers) == 0 {
		return nil, errors.New("这台实例没配图床")
	}
	if provider == "" {
		return h.providers, nil
	}
	for _, p := range h.providers {
		if p.Name() == provider {
			return []ImageProvider{p}, nil
		}
	}
	return nil, fmt.Errorf("没有 %q 这个图床，可用的是 %s", provider, strings.Join(h.ProviderNames(), "、"))
}

// ProviderNames 返回实际启用的图床名，顺序即尝试顺序。
func (h *ImageHost) ProviderNames() []string {
	out := make([]string, 0, len(h.providers))
	for _, p := range h.providers {
		out = append(out, p.Name())
	}
	return out
}

func sessionScoped(p ImageProvider) bool {
	if s, ok := p.(interface{ SessionScoped() bool }); ok {
		return s.SessionScoped()
	}
	return false
}

func describeInput(in ProviderInput) string {
	origin := in.Origin
	if origin == "" {
		origin = "未标注来源"
	}
	mime := strings.TrimSpace(in.MimeType)
	if mime == "" {
		mime = "未标注"
	}
	return fmt.Sprintf("来源=%s, 字节=%d, 类型=%s", origin, len(in.Buffer), mime)
}

func probeState(ok bool, ct string) string {
	switch {
	case !ok:
		return "HEAD 不可达"
	case ct == "":
		return "HEAD 200, 无 content-type"
	default:
		return "HEAD 200, " + ct
	}
}

func followedNote(original, final string) string {
	if final == original {
		return ""
	}
	return ", 跟随后 URL=" + final
}

func (h *ImageHost) probeURL(raw string) (string, string, bool) {
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return "", "", false
	}
	resp, err := h.head.Head(raw)
	if err != nil || resp == nil {
		return "", "", false
	}
	resp.Body.Close()
	final := raw
	if resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	return final, resp.Header.Get("Content-Type"), true
}

func isImageContentType(ct string) bool {
	ct = strings.TrimSpace(strings.ToLower(ct))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return strings.HasPrefix(ct, "image/")
}
