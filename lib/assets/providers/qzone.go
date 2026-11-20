package providers

import (
	"context"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/requests"
)

func init() {
	assets.Register("qzone", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		cookie, ok1 := strPtr(cfg, "cookie")
		pskey, ok2 := strPtr(cfg, "pSkey")
		skey, ok3 := strPtr(cfg, "skey")
		pUin, ok4 := strPtr(cfg, "pUin")
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return nil, fmt.Errorf("qzone: cookie/p_skey/skey/p_uin 必填")
		}
		return &qzoneProvider{cl: cl, cookie: cookie, pSkey: pskey, skey: skey, pUin: pUin}, nil
	}, []assets.ConfigField{
		{Key: "cookie", Label: "qzone.qq.com Cookie", Type: "password", Required: true},
		{Key: "pSkey", Label: "QQ p_skey", Type: "password", Required: true},
		{Key: "skey", Label: "QQ skey", Type: "password", Required: true},
		{Key: "pUin", Label: "QQ p_uin（数字QQ号）", Type: "text", Required: true},
	})
}

type qzoneProvider struct {
	cl     *assets.Client
	cookie string
	pSkey  string
	skey   string
	pUin   string
}

func (p *qzoneProvider) Name() string { return "qzone" }

func (p *qzoneProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	gTk := getBkn(p.pSkey)
	form := urlValues(map[string]string{
		"filename":          "filename",
		"uin":               p.pUin,
		"skey":              p.skey,
		"zzpaneluin":        p.pUin,
		"zzpanelkey":        "",
		"p_uin":             p.pUin,
		"p_skey":            p.pSkey,
		"qzonetoken":        "",
		"uploadtype":        "1",
		"albumtype":         "7",
		"exttype":           "0",
		"refer":             "shuoshuo",
		"output_type":       "jsonhtml",
		"charset":           "utf-8",
		"output_charset":    "utf-8",
		"upload_hd":         "1",
		"hd_width":          "2048",
		"hd_height":         "10000",
		"hd_quality":        "96",
		"backUrls":          "",
		"url":               "",
		"base64":            "1",
		"jsonhtml_callback": "callback",
		"picfile":           b64(in.Buffer),
		"qzreferrer":        "https%3A%2F%2Fuser.qzone.qq.com%2F" + p.pUin + "%2Finfocenter",
	})
	raw, err := p.cl.DoBytes("POST", "https://up.qzone.qq.com/cgi-bin/upload/cgi_upload_image?g_tk="+gTk,
		requests.Form(form), map[string]string{
			"Cookie":     p.cookie,
			"Referer":    "https://user.qzone.qq.com/",
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		})
	if err != nil {
		return "", err
	}
	match := regexpOriginURL(raw)
	if match == "" {
		return "", fmt.Errorf("qzone: 上传失败: %s", truncate(string(raw), 200))
	}
	return match, nil
}
