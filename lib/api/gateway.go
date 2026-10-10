package api

import (
	"encoding/json"
	"fmt"
)

// GatewayBotInfo /gateway/bot 响应: 网关地址 + 会话启动限额。
type GatewayBotInfo struct {
	URL    string `json:"url"`
	Shards int    `json:"shards"`
	Limit  struct {
		Total         int64 `json:"total"`
		Remaining     int64 `json:"remaining"`
		ResetAfter    int64 `json:"reset_after"`
		MaxConcurrent int   `json:"max_concurrency"`
	} `json:"session_start_limit"`
}

type GatewayAPI struct {
	api *BotAPI
}

// GatewayBot 获取网关地址与会话启动限额。remaining 耗尽时需等待 reset_after 再建连。
func (g *GatewayAPI) GatewayBot() (*GatewayBotInfo, error) {
	var result GatewayBotInfo
	raw, err := g.api.do("GET", fmt.Sprintf("%v/gateway/bot", g.api.ProxyAPI), nil)
	if err != nil {
		return nil, fmt.Errorf("get gateway bot: %w", err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result.URL == "" {
		return nil, fmt.Errorf("gateway bot url is empty")
	}
	return &result, nil
}
