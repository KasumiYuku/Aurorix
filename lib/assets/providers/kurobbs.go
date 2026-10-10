package providers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KasumiYuku/Aurorix/lib/assets"
)

func init() {
	assets.RegisterNoProbe("kurobbs", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		token, ok := strPtr(cfg, "token")
		if !ok {
			return nil, fmt.Errorf("kurobbs: token 必填")
		}
		return &kurobbsProvider{cl: cl, token: token}, nil
	}, []assets.ConfigField{
		{Key: "token", Label: "库街区 Token", Type: "password", Required: true, Description: "库街区登录凭证，抓包任意已登录请求的请求头 Token 字段"},
	})
}

type kurobbsProvider struct {
	cl    *assets.Client
	token string
}

func (p *kurobbsProvider) Name() string { return "kurobbs" }

func (p *kurobbsProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	name := in.Filename
	if name == "" {
		name = "image." + mimeExt(in.MimeType)
	}
	mp := assets.NewMultipart()
	mp.AddFile("files", name, mimeOr(in.MimeType), in.Buffer)
	mp.Close()

	raw, err := p.cl.DoBytes("POST", "https://api.kurobbs.com/forum/uploadForumImgForH5", mp, map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36",
		"source":     "h5",
		"Referer":    "http://www.kurobbs.com/",
		"Token":      p.token,
	})
	if err != nil {
		return "", err
	}

	var resp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("kurobbs: 返回非 JSON: %s", truncate(string(raw), 200))
	}
	var url string
	if err := json.Unmarshal(resp.Data, &url); err != nil {
		var arr []string
		if err2 := json.Unmarshal(resp.Data, &arr); err2 != nil || len(arr) == 0 {
			return "", fmt.Errorf("kurobbs: 响应无图片地址(code=%d %s): %s", resp.Code, resp.Msg, truncate(string(raw), 200))
		}
		url = arr[0]
	}
	if url == "" {
		return "", fmt.Errorf("kurobbs: 响应无图片地址(code=%d %s): %s", resp.Code, resp.Msg, truncate(string(raw), 200))
	}
	return url, nil
}
