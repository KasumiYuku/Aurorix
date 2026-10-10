package assets

import (
	"context"
	"fmt"
)

func init() {
	RegisterDefault("qqbot", func(_ *Client, _ map[string]any) (ImageProvider, error) {
		return &qqbotProvider{}, nil
	}, nil)
}

type qqbotProvider struct{}

func (*qqbotProvider) Name() string { return "qqbot" }

// SessionScoped 声明这条 provider 签出来的直链**只在与签发时相同的会话里有效**。
func (*qqbotProvider) SessionScoped() bool { return true }

// Upload 走官方富媒体上传，取回预签名直链。
func (*qqbotProvider) Upload(_ context.Context, in ProviderInput) (string, error) {
	up := OfficialUploadFn()
	if up == nil {
		return "", fmt.Errorf("官方上传未注入（当前实例不是官方机器人运行时）")
	}
	if in.GroupID == "" && in.UserID == "" {
		return "", fmt.Errorf("缺少会话目标 openid")
	}
	resolved, err := up(in)
	if err != nil {
		return "", err
	}
	if resolved.URL == "" {
		return "", fmt.Errorf("官方上传未返回直链")
	}
	return resolved.URL, nil
}
