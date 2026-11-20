package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"strings"
)

func init() {
	assets.Register("announce", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		cookie, ok := strPtr(cfg, "cookie")
		if !ok {
			return nil, fmt.Errorf("announce: cookie 必填")
		}
		return &announceProvider{cl: cl, cookie: cookie, skey: skeyFromCookie(cookie)}, nil
	}, []assets.ConfigField{
		{Key: "cookie", Label: "qun.qq.com Cookie（含 skey）", Type: "password", Required: true},
	})
}

type announceProvider struct {
	cl     *assets.Client
	cookie string
	skey   string
}

func (p *announceProvider) Name() string { return "announce" }

func (p *announceProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	bkn := getBkn(p.skey)
	mp := assets.NewMultipart()
	mp.AddFile("pic_up", in.Filename, in.MimeType, in.Buffer)
	mp.AddFile("file", in.Filename, in.MimeType, in.Buffer)
	mp.AddField("bkn", bkn)
	mp.Close()
	raw, err := p.cl.DoBytes("POST", "https://web.qun.qq.com/cgi-bin/announce/upload_img", mp, map[string]string{
		"Cookie":           p.cookie,
		"Host":             "web.qun.qq.com",
		"X-Requested-With": "XMLHttpRequest",
	})
	if err != nil {
		return "", err
	}
	return parseAnnounceResponse(raw)
}

func parseAnnounceResponse(raw []byte) (string, error) {
	var outer struct {
		EC int    `json:"ec"`
		EM string `json:"em"`
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &outer); err != nil {
		return "", fmt.Errorf("announce: 响应解析失败: %s", truncate(string(raw), 200))
	}
	if outer.EC != 0 {
		return "", fmt.Errorf("announce: 上传失败(%d): %s", outer.EC, outer.EM)
	}
	if outer.ID == "" {
		return "", fmt.Errorf("announce: 响应缺少 id: %s", truncate(string(raw), 200))
	}

	var inner struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(strings.ReplaceAll(outer.ID, "&quot;", `"`)), &inner); err != nil || inner.ID == "" {
		return "", fmt.Errorf("announce: 无法解析图片 ID: %s", truncate(outer.ID, 200))
	}
	return "https://p.qlogo.cn/gdynamic/" + inner.ID + "/0", nil
}
