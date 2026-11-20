package providers

import (
	"context"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"time"
)

func init() {
	assets.Register("qqgame", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		return &qqgameProvider{cl: cl, uid: str(cfg, "opensopUid", "A")}, nil
	}, []assets.ConfigField{
		{Key: "opensopUid", Label: "opensop_uid", Type: "text", Default: "A"},
	})
}

type qqgameProvider struct {
	cl  *assets.Client
	uid string
}

func (p *qqgameProvider) Name() string { return "qqgame" }

func (p *qqgameProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	auth := struct {
		Credentials *struct {
			SessionToken string `json:"sessionToken"`
			TmpSecretID  string `json:"tmpSecretId"`
			TmpSecretKey string `json:"tmpSecretKey"`
		} `json:"credentials"`
	}{}
	err := p.cl.Get("https://game.q.qq.com/cgi-bin/cosTmpAuth", &auth, map[string]string{
		"Host":        "game.q.qq.com",
		"Cookie":      "opensop_uid=" + p.uid,
		"traceparent": "00-" + nowUUID() + "-" + nowUUID()[:16] + "-01",
		"Referer":     "https://game.q.qq.com/setting/game-management?type=create",
		"User-Agent":  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	})
	if err != nil {
		return "", err
	}
	if auth.Credentials == nil {
		return "", fmt.Errorf("qqgame: 获取临时凭证失败")
	}
	cred := auth.Credentials

	ext := mimeExt(in.MimeType)
	now := time.Now().Unix()
	cosPath := fmt.Sprintf("tqgame/%s/%d%d.%s", p.uid, now, time.Now().UnixMilli()%1e6, ext)
	host := "sopstatic-1251316161.cos.ap-nanjing.myqcloud.com"
	expire := now + 300
	headers := map[string]string{
		"content-length":      fmt.Sprint(len(in.Buffer)),
		"host":                host,
		"x-cos-storage-class": "STANDARD",
	}
	sig, qKeyTime := cosSign("put", "/"+cosPath, headers, cred.TmpSecretID, cred.TmpSecretKey, now, expire)
	authHeader := fmt.Sprintf("q-sign-algorithm=sha1&q-ak=%s&q-sign-time=%s&q-key-time=%s&q-header-list=content-length;host;x-cos-storage-class&q-url-param-list=&q-signature=%s",
		cred.TmpSecretID, qKeyTime, qKeyTime, sig)

	h := make(map[string]string, len(headers)+3)
	for k, v := range headers {
		h[k] = v
	}
	h["Authorization"] = authHeader
	h["x-cos-security-token"] = cred.SessionToken
	h["User-Agent"] = "Mozilla/5.0"
	h["Referer"] = "https://game.q.qq.com/"
	if err := p.cl.Put("https://"+host+"/"+cosPath, in.Buffer, nil, h); err != nil {
		return "", err
	}
	return "https://sopstatic.gtimg.cn/" + cosPath, nil
}
