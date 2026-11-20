package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/assets"
)

func init() {
	assets.Register("mihoyo", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		cookie, ok := strPtr(cfg, "cookie")
		if !ok {
			return nil, fmt.Errorf("mihoyo: cookie 必填")
		}
		return &mihoyoProvider{cl: cl, cookie: cookie}, nil
	}, []assets.ConfigField{
		{Key: "cookie", Label: "米游社登录 Cookie", Type: "password", Required: true, Description: "含 stuid / stoken / ltoken，抓包网页端或 App 已登录请求的请求头 Cookie"},
	})
}

type mihoyoOss struct {
	Host        string            `json:"host"`
	Policy      string            `json:"policy"`
	Callback    string            `json:"callback"`
	AccessID    string            `json:"accessid"`
	Signature   string            `json:"signature"`
	ObjectACL   string            `json:"object_acl"`
	OSSType     string            `json:"x_oss_content_type"`
	CallbackVar map[string]string `json:"callback_var"`
}

type mihoyoParams struct {
	FileName string     `json:"file_name"`
	Oss      *mihoyoOss `json:"oss"`
}

type mihoyoProvider struct {
	cl     *assets.Client
	cookie string
}

func (p *mihoyoProvider) Name() string { return "mihoyo" }

func (p *mihoyoProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	ext := mimeExt(in.MimeType)
	params, err := p.getUploadParams(md5Hex(in.Buffer), ext)
	if err != nil {
		return "", err
	}
	oss := params.Oss
	if oss == nil || oss.Host == "" || params.FileName == "" {
		return "", fmt.Errorf("mihoyo: 获取上传参数失败，缺少 OSS 信息")
	}

	mp := assets.NewMultipart()
	mp.AddField("name", path.Base(params.FileName))
	mp.AddField("key", params.FileName)
	mp.AddField("callback", oss.Callback)
	mp.AddField("success_action_status", "200")
	mp.AddField("x:extra", oss.CallbackVar["x:extra"])
	mp.AddField("x-oss-content-type", oss.OSSType)
	mp.AddField("OSSAccessKeyId", oss.AccessID)
	mp.AddField("policy", oss.Policy)
	mp.AddField("signature", oss.Signature)
	acl := oss.ObjectACL
	if acl == "" {
		acl = "default"
	}
	mp.AddField("x-oss-object-acl", acl)
	for k, v := range mihoyoEqFields(oss.Policy) {
		if !mihoyoBaseField(k) {
			mp.AddField(k, v)
		}
	}
	mp.AddFile("file", path.Base(params.FileName), mimeOr(in.MimeType), in.Buffer)
	mp.Close()

	var result struct {
		Data *struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := p.cl.PostMultipart(oss.Host, mp, &result, map[string]string{
		"Referer": "https://dby.miyoushe.com/",
	}); err != nil {
		return "", err
	}
	if result.Data == nil || result.Data.URL == "" {
		return "", fmt.Errorf("mihoyo: 上传失败，未返回 URL")
	}
	return result.Data.URL, nil
}

func (p *mihoyoProvider) getUploadParams(md5sum, ext string) (*mihoyoParams, error) {
	var result struct {
		Retcode int           `json:"retcode"`
		Message string        `json:"message"`
		Data    *mihoyoParams `json:"data"`
	}
	err := p.cl.Post("https://api-takumi.miyoushe.com/upload/outer/getParamsByAccount", map[string]any{
		"md5": md5sum, "ext": ext, "biz": "community",
		"extra":                   `{"upload_source":"villa_chat"}`,
		"support_content_type":    true,
		"support_extra_form_data": true,
	}, &result, map[string]string{
		"cookie":     p.cookie,
		"Origin":     "https://dby.miyoushe.com",
		"Referer":    "https://dby.miyoushe.com/",
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36",
	})
	if err != nil {
		return nil, err
	}
	if result.Retcode != 0 || result.Data == nil {
		if result.Retcode == -100 || strings.Contains(result.Message, "登录") {
			return nil, fmt.Errorf("mihoyo: Cookie 已失效/过期 (retcode=%d %s)，请更新后重试", result.Retcode, result.Message)
		}
		return nil, fmt.Errorf("mihoyo: 获取上传参数失败 (retcode=%d %s)", result.Retcode, result.Message)
	}
	return result.Data, nil
}

func mihoyoEqFields(policy string) map[string]string {
	out := map[string]string{}
	decoded, err := base64.StdEncoding.DecodeString(policy)
	if err != nil {
		return out
	}
	var pol struct {
		Conditions []json.RawMessage `json:"conditions"`
	}
	if json.Unmarshal(decoded, &pol) != nil {
		return out
	}
	for _, c := range pol.Conditions {
		var parts []any
		if json.Unmarshal(c, &parts) != nil || len(parts) < 3 {
			continue
		}
		op, _ := parts[0].(string)
		field, _ := parts[1].(string)
		if op != "eq" || !strings.HasPrefix(field, "$") {
			continue
		}
		out[strings.TrimPrefix(field, "$")] = fmt.Sprint(parts[2])
	}
	return out
}

func mihoyoBaseField(key string) bool {
	switch key {
	case "name", "key", "callback", "success_action_status", "x:extra",
		"x-oss-content-type", "OSSAccessKeyId", "policy", "signature", "x-oss-object-acl":
		return true
	}
	return false
}
