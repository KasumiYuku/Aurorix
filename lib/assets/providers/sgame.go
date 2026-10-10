package providers

import (
	"context"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"time"
)

func init() {
	assets.Register("sgame", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		secID, ok1 := strPtr(cfg, "secretId")
		secKey, ok2 := strPtr(cfg, "secretKey")
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("sgame: SecretId 与 SecretKey 必填")
		}
		return &sgameProvider{
			cl: cl, secretID: secID, secretKey: secKey,
			host:      str(cfg, "host", "sgame-data-service-1252931805.cos.ap-nanjing.myqcloud.com"),
			bucketURL: str(cfg, "bucketUrl", "https://download.nature.qq.com"),
		}, nil
	}, []assets.ConfigField{
		{Key: "secretId", Label: "腾讯云 SecretId", Type: "password", Required: true},
		{Key: "secretKey", Label: "腾讯云 SecretKey", Type: "password", Required: true},
		{Key: "host", Label: "COS Host", Type: "text"},
		{Key: "bucketUrl", Label: "Bucket URL", Type: "text"},
	})
}

type sgameProvider struct {
	cl        *assets.Client
	secretID  string
	secretKey string
	host      string
	bucketURL string
}

func (p *sgameProvider) Name() string { return "sgame" }

func (p *sgameProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	ext := mimeExt(in.MimeType)
	fileName := fmt.Sprintf("img_%d.%s", time.Now().UnixMilli(), ext)
	urlPath := "/SnsShare/SocialProfile/" + fileName
	md5sum := fmt.Sprintf("%X", md5sumBytes(in.Buffer))
	contentMD5 := b64(md5sumBytes(in.Buffer))
	now := time.Now().Unix()
	expire := now + 600
	headers := map[string]string{
		"content-length":   fmt.Sprint(len(in.Buffer)),
		"content-md5":      contentMD5,
		"content-type":     mimeOr(in.MimeType),
		"host":             p.host,
		"x-cos-meta-extra": "2971978106IOS",
		"x-cos-meta-md5":   md5sum,
	}
	sig, qKeyTime := cosSign("put", urlPath, headers, p.secretID, p.secretKey, now, expire)
	auth := fmt.Sprintf("q-sign-algorithm=sha1&q-ak=%s&q-sign-time=%d;%d&q-key-time=%s&q-header-list=content-length;content-md5;content-type;host;x-cos-meta-extra;x-cos-meta-md5&q-url-param-list=&q-signature=%s",
		p.secretID, now, expire, qKeyTime, sig)
	headers["Authorization"] = auth
	headers["User-Agent"] = "cos-xml-ios-sdk-v6.4.0"
	if err := p.cl.Put("https://"+p.host+urlPath, in.Buffer, nil, headers); err != nil {
		return "", err
	}
	return p.bucketURL + urlPath, nil
}
