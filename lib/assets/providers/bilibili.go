package providers

import (
	"context"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"strings"
)

func init() {
	assets.Register("bilibili", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		sess, ok1 := strPtr(cfg, "sessdata")
		jct, ok2 := strPtr(cfg, "biliJct")
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("bilibili: SESSDATA 与 bili_jct 必填")
		}
		return &biliProvider{cl: cl, sess: sess, jct: jct}, nil
	}, []assets.ConfigField{
		{Key: "sessdata", Label: "SESSDATA", Type: "password", Required: true},
		{Key: "biliJct", Label: "bili_jct", Type: "password", Required: true},
	})
}

type biliProvider struct {
	cl   *assets.Client
	sess string
	jct  string
}

func (p *biliProvider) Name() string { return "bilibili" }

func (p *biliProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	mp := assets.NewMultipart()
	mp.AddField("bucket", "album")
	mp.AddField("csrf", p.jct)
	mp.AddFile("file", in.Filename, in.MimeType, in.Buffer)
	mp.Close()
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Location string `json:"location"`
		} `json:"data"`
	}
	err := p.cl.PostMultipart("https://api.bilibili.com/x/upload/web/image", mp, &resp, map[string]string{
		"Cookie":     "SESSDATA=" + p.sess + "; bili_jct=" + p.jct,
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	})
	if err != nil {
		return "", err
	}
	if resp.Code != 0 || resp.Data.Location == "" {
		return "", fmt.Errorf("bilibili: %s", resp.Message)
	}
	return strings.Replace(resp.Data.Location, "http://", "https://", 1), nil
}
