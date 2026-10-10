package providers

import (
	"context"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/assets"
)

func init() {
	assets.Register("qqstream", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		cookie, ok1 := strPtr(cfg, "cookie")
		skey, ok2 := strPtr(cfg, "skey")
		pskey, ok3 := strPtr(cfg, "pSkey")
		uin, ok4 := strPtr(cfg, "uin")
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return nil, fmt.Errorf("qqstream: cookie/skey/p_skey/uin 必填")
		}
		return &qqstreamProvider{cl: cl, cookie: cookie, skey: skey, pSkey: pskey, uin: uin}, nil
	}, []assets.ConfigField{
		{Key: "cookie", Label: "完整 Cookie（含 uin/skey/p_skey）", Type: "password", Required: true},
		{Key: "skey", Label: "QQ skey", Type: "password", Required: true},
		{Key: "pSkey", Label: "qvideo.qq.com p_skey", Type: "password", Required: true},
		{Key: "uin", Label: "QQ uin（数字QQ号）", Type: "text", Required: true},
	})
}

type qqstreamProvider struct {
	cl     *assets.Client
	cookie string
	skey   string
	pSkey  string
	uin    string
}

func (p *qqstreamProvider) Name() string { return "qqstream" }

func (p *qqstreamProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	bkn := getBkn(p.skey)
	mp := assets.NewMultipart()
	mp.AddField("type", "2")
	mp.AddField("detectface", "1")
	mp.AddFile("image", nowUUID(), in.MimeType, in.Buffer)
	mp.Close()
	var resp struct {
		Retcode int `json:"retcode"`
		Result  *struct {
			FileID string `json:"fileId"`
		} `json:"result"`
	}
	err := p.cl.PostMultipart("https://qvideo.qq.com/cgi-bin/videohub/upload_image?bkn="+bkn, mp, &resp, map[string]string{
		"Cookie":     p.cookie,
		"Referer":    "https://qvideo.qq.com/mixed/m/edit-profile.html",
		"User-Agent": "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36",
	})
	if err != nil {
		return "", err
	}
	if resp.Retcode != 0 {
		return "", fmt.Errorf("qqstream: 上传失败: retcode=%d", resp.Retcode)
	}
	if resp.Result == nil || resp.Result.FileID == "" {
		return "", fmt.Errorf("qqstream: 上传失败")
	}
	return "https://p.qpic.cn/qlove_pic/0/" + resp.Result.FileID + "/340", nil
}
