package providers

import (
	"context"
	"github.com/KasumiYuku/Aurorix/lib/assets"
)

func init() {
	assets.Register("chinaunicom", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		return &unicomProvider{cl: cl, cfg: cfg}, nil
	}, []assets.ConfigField{
		{Key: "appVersion", Label: "客户端版本", Type: "text", Default: "7.8.2"},
		{Key: "appChannel", Label: "渠道", Type: "text", Default: "oppo_woapp"},
	})
}

type unicomProvider struct {
	cl  *assets.Client
	cfg map[string]any
}

func (p *unicomProvider) Name() string { return "chinaunicom" }

func (p *unicomProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	mp := assets.NewMultipart()
	mp.AddFile("file", in.Filename, in.MimeType, in.Buffer)
	mp.Close()
	var resp struct {
		RSP struct {
			DATA string `json:"DATA"`
		} `json:"RSP"`
	}
	err := p.cl.PostMultipart("https://iotpservice.smartont.net/api-user/upload.do", mp, &resp, map[string]string{
		"User-Agent":   "okhttp/4.9.0",
		"Connection":   "keep-alive",
		"Accept":       "*/*",
		"clientsign":   "MapleLeaf dominate the world.",
		"appchannel":   str(p.cfg, "appChannel", "oppo_woapp"),
		"platform":     "1",
		"cacherefresh": "1",
		"appversion":   str(p.cfg, "appVersion", "7.8.2"),
		"accesstoken":  nowUUID(),
	})
	if err != nil {
		return "", err
	}
	if resp.RSP.DATA == "" {
		return "", errNoData("chinaunicom")
	}
	return resp.RSP.DATA, nil
}
