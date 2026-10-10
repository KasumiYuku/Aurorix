package providers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/requests"
)

var logCNB = logx.New("assets.cnb")

func init() {
	assets.RegisterNoProbe("cnb", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		token, ok := strPtr(cfg, "token")
		if !ok {
			return nil, fmt.Errorf("cnb: token 必填")
		}
		repo, ok := strPtr(cfg, "defaultRepo")
		if !ok {
			return nil, fmt.Errorf("cnb: 默认仓库必填")
		}
		return &cnbProvider{
			cl:         cl,
			token:      token,
			repo:       repo,
			baseURL:    str(cfg, "baseUrl", "https://api.cnb.cool"),
			autodelete: intOr(cfg, "autodelete", 30),
		}, nil
	}, []assets.ConfigField{
		{Key: "token", Label: "CNB 令牌", Type: "password", Required: true, Description: "CNB 个人访问令牌 (Personal Access Token)，需带仓库读写权限"},
		{Key: "defaultRepo", Label: "默认仓库", Type: "text", Required: true, Placeholder: "用户名/仓库名"},
		{Key: "baseUrl", Label: "API 地址", Type: "text", Default: "https://api.cnb.cool"},
		{Key: "autodelete", Label: "自动删除(秒)", Type: "number", Default: 30, Description: "上传成功 N 秒后删除远端文件，0 表示不删除"},
	})
}

type cnbProvider struct {
	cl         *assets.Client
	token      string
	repo       string
	baseURL    string
	autodelete int
}

func (p *cnbProvider) Name() string { return "cnb" }

type cnbUploadInfo struct {
	UploadURL string            `json:"upload_url"`
	Token     string            `json:"token"`
	Form      map[string]string `json:"form"`
	Assets    *cnbAsset         `json:"assets"`
}

type cnbAsset struct {
	URL  string `json:"url"`
	Path string `json:"path"`
}

func (p *cnbProvider) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	name := in.Filename
	if name == "" {
		name = nowUUID() + "." + mimeExt(in.MimeType)
	}
	kind := "files"
	if cnbIsImage(name) {
		kind = "imgs"
	}

	info, err := p.requestUploadURL(name, len(in.Buffer), kind)
	if err != nil {
		return "", err
	}
	if err := p.putBytes(info, in.Buffer); err != nil {
		return "", err
	}

	assetURL := ""
	if info.Assets != nil {
		assetURL = info.Assets.URL
		if assetURL == "" {
			assetURL = info.Assets.Path
		}
	}
	if assetURL == "" {
		return "", fmt.Errorf("cnb: 返回 URL 为空")
	}
	if !strings.HasPrefix(assetURL, "http") {
		assetURL = "https://cnb.cool" + assetURL
	}

	if p.autodelete > 0 && info.Assets != nil && info.Assets.Path != "" {
		pathToDelete := info.Assets.Path
		seconds := p.autodelete
		go func() {
			time.Sleep(time.Duration(seconds) * time.Second)
			if err := p.deleteFile(pathToDelete); err != nil {
				logCNB.Errorf("cnb: 自动删除失败 %s: %v", pathToDelete, err)
			}
		}()
	}
	return assetURL, nil
}

func (p *cnbProvider) requestUploadURL(name string, size int, kind string) (*cnbUploadInfo, error) {
	var info cnbUploadInfo
	err := p.cl.Post(fmt.Sprintf("%s/%s/-/upload/%s", p.baseURL, p.repo, kind), map[string]any{
		"name": name, "size": size,
	}, &info, map[string]string{
		"accept":        "application/json",
		"Authorization": "Bearer " + p.token,
		"Content-Type":  "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("cnb: 获取上传地址失败: %w", err)
	}
	if info.UploadURL == "" {
		return nil, fmt.Errorf("cnb: 未返回上传地址")
	}
	return &info, nil
}

func (p *cnbProvider) putBytes(info *cnbUploadInfo, data []byte) error {
	uploadURL := info.UploadURL
	if len(info.Form) > 0 {
		u, err := url.Parse(uploadURL)
		if err != nil {
			return fmt.Errorf("cnb: 上传地址无效: %w", err)
		}
		q := u.Query()
		for k, v := range info.Form {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		uploadURL = u.String()
	}
	headers := map[string]string{"Content-Type": "application/octet-stream"}
	if info.Token != "" {
		headers["Authorization"] = "Bearer " + info.Token
	}
	if err := p.cl.Put(uploadURL, data, nil, headers); err != nil {
		return fmt.Errorf("cnb: 上传失败: %w", err)
	}
	return nil
}

func (p *cnbProvider) deleteFile(cnbPath string) error {
	delURL := cnbPath
	if !strings.HasPrefix(delURL, "http") {
		delURL = p.baseURL + "/" + strings.TrimPrefix(cnbPath, "/")
	}
	if _, err := p.cl.DoBytes("DELETE", delURL, nil, map[string]string{
		"Authorization": "Bearer " + p.token,
	}); err != nil {
		var se *requests.StatusError
		if errors.As(err, &se) && se.Code == 404 {
			return nil
		}
		return err
	}
	return nil
}

func cnbIsImage(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		return true
	}
	return false
}

func intOr(cfg map[string]any, key string, def int) int {
	switch v := cfg[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
