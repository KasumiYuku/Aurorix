package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/requests"
)

func init() {
	assets.Register("homework", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		cookie, ok := strPtr(cfg, "cookie")
		if !ok {
			return nil, fmt.Errorf("homework: cookie 必填")
		}
		return &homeworkProvider{cl: cl, cookie: cookie, skey: skeyFromCookie(cookie)}, nil
	}, []assets.ConfigField{
		{Key: "cookie", Label: "qun.qq.com Cookie（含 skey）", Type: "password", Required: true},
	})
}

type homeworkProvider struct {
	cl     *assets.Client
	cookie string
	skey   string
}

func (p *homeworkProvider) Name() string { return "homework" }

func (p *homeworkProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	bkn := getBkn(p.skey)
	form := urlValues(map[string]string{
		"pic":         b64(in.Buffer),
		"client_type": "1",
		"bkn":         bkn,
	})
	raw, err := p.cl.DoBytes("POST", "https://qun.qq.com/cgi-bin/hw/util/image", requests.Form(form), map[string]string{
		"Cookie":       p.cookie,
		"Content-Type": "application/x-www-form-urlencoded",
		"Accept":       "*/*",
		"Origin":       "https://qun.qq.com",
		"Referer":      "https://qun.qq.com/homework/p/features/index.html",
		"User-Agent":   homeworkUA,
	})
	if err != nil {
		return "", err
	}
	return parseHomeworkResponse(raw)
}

const homeworkUA = "Mozilla/5.0 (Windows NT 6.2; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) " +
	"QQ/9.7.1.28934 Chrome/43.0.2357.134 Safari/537.36 QBCore/3.43.1298.400 QQBrowser/9.0.2524.400"

func parseHomeworkResponse(raw []byte) (string, error) {
	var resp struct {
		CgiCode int             `json:"cgicode"`
		Retcode int             `json:"retcode"`
		Msg     string          `json:"msg"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("homework: 响应解析失败: %s", truncate(string(raw), 200))
	}
	if resp.CgiCode != 0 || resp.Retcode != 0 {
		return "", fmt.Errorf("homework: 上传失败(%d): %s", resp.CgiCode, resp.Msg)
	}
	var data struct {
		URL *struct {
			Origin string `json:"origin"`
		} `json:"url"`
	}
	if len(resp.Data) > 0 && resp.Data[0] == '{' {
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			return "", fmt.Errorf("homework: 响应解析失败: %s", truncate(string(raw), 200))
		}
	}
	if data.URL == nil || data.URL.Origin == "" {
		return "", fmt.Errorf("homework: 响应缺少图片 URL: %s", truncate(string(raw), 200))
	}
	return strings.Replace(data.URL.Origin, "p.qpic.cn", "p.qlogo.cn", 1), nil
}
