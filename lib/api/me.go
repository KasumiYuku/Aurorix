package api

// MeAPI 自身信息域: 拉取当前机器人档案。

import (
	"encoding/json"
	"fmt"
)

// BotProfile /v2/users/me 响应: 机器人自身信息。
// 平台返回扁平对象, 与 sendWithRetry 解包模式一致, 无 data 包装。
type BotProfile struct {
	ID               string `json:"id"`
	Username         string `json:"username"`
	Avatar           string `json:"avatar"`
	Bot              bool   `json:"bot"`
	UnionOpenID      string `json:"union_openid"`
	UnionUserAccount string `json:"union_user_account"`
	ShareURL         string `json:"share_url"`
	WelcomeMsg       string `json:"welcome_msg"`
}

// IsZero 档案为空时为 true, 表示未拉取到。
func (p BotProfile) IsZero() bool {
	return p.ID == "" && p.Username == "" && p.Avatar == ""
}

type MeAPI struct {
	api *BotAPI
}

// GetMe 获取机器人自身档案。失败时返回 error, 由调用方决定降级策略。
func (m *MeAPI) GetMe() (*BotProfile, error) {
	var result BotProfile
	raw, err := m.api.do("GET", fmt.Sprintf("%v/users/@me", m.api.ProxyAPI), nil)
	if err != nil {
		return nil, fmt.Errorf("get bot profile: %w", err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result.IsZero() {
		return nil, fmt.Errorf("bot profile is empty")
	}
	return &result, nil
}
