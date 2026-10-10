package api

import (
	"encoding/json"
	"fmt"
)

// GroupBotState 机器人在某个群的运行状态。
type GroupBotState struct {
	MemberOpenID      string `json:"member_openid"`
	JoinedAt          string `json:"joined_at"`
	AllowProactiveMsg bool   `json:"allow_proactive_msg"`
	RecvMsgSetting    string `json:"recv_msg_setting"`
	MemberRole        string `json:"member_role"`
}

type GroupAPI struct {
	api *BotAPI
}

// GetBotState 查询机器人在指定群的状态。
func (g *GroupAPI) GetBotState(groupID string) (*GroupBotState, error) {
	url := fmt.Sprintf("%v/v2/groups/%v/bot_state", g.api.ProxyAPI, groupID)
	raw, err := g.api.do("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("get group bot state: %w", err)
	}
	var state GroupBotState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return &state, nil
}
