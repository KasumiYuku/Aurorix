package api

// GatewayAPI 网关域: 拉取 WebSocket 地址与会话启动限额。

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
		ResetAfter    int64 `json:"reset_after"`     // 毫秒
		MaxConcurrent int   `json:"max_concurrency"` // 建议的并发连接数
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

// GatewayURL 获取网关 WebSocket 地址。
func (g *GatewayAPI) GatewayURL() (string, error) {
	var result struct {
		URL string `json:"url"`
	}
	raw, err := g.api.do("GET", fmt.Sprintf("%v/gateway", g.api.ProxyAPI), nil)
	if err != nil {
		return "", fmt.Errorf("get gateway url: %w", err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	if result.URL == "" {
		return "", fmt.Errorf("gateway url is empty")
	}
	return result.URL, nil
}
