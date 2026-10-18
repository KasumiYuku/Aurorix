package api

import (
	"encoding/json"
	"fmt"
	"net/url"
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

// GroupInfo 群资料。
//
// 群名只有这个接口给: 群消息事件里只有 group_openid, 没有名字。
type GroupInfo struct {
	GroupOpenID     string   `json:"group_openid"`
	GroupName       string   `json:"group_name"`
	GroupFingerMemo string   `json:"group_finger_memo"`
	GroupClassText  string   `json:"group_class_text"`
	GroupTags       []string `json:"group_tags"`
	GroupMemberNum  int      `json:"group_member_num"`
}

// GetInfo 查群资料(含群名)。实测端点: GET /v2/groups/{group_openid}/info。
func (g *GroupAPI) GetInfo(groupID string) (*GroupInfo, error) {
	url := fmt.Sprintf("%v/v2/groups/%v/info", g.api.ProxyAPI, groupID)
	raw, err := g.api.do("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("get group info: %w", err)
	}
	var info GroupInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// GroupMember 群成员。username 是这个人自己起的昵称, 比 openid 有用得多。
type GroupMember struct {
	MemberOpenID string `json:"member_openid"`
	Username     string `json:"username"`
	MemberRole   string `json:"member_role"`
	Bot          bool   `json:"bot"`
	JoinedAt     string `json:"joined_at"`
	UnionOpenID  string `json:"union_openid"`
}

type groupMemberPage struct {
	Members    []GroupMember `json:"members"`
	NextCursor string        `json:"next_cursor"`
}

// GetMember 查单个群成员(含昵称)。
//
// 注意: 这两个成员接口需要在开放平台额外授权, 没授权时平台回 11253「应用无接口访问权限」
// (实测我们当前这套 appid 就是这个状态)。发信人自己的昵称不用走这里 —— 消息事件里自带。
func (g *GroupAPI) GetMember(groupID, memberOpenID string) (*GroupMember, error) {
	url := fmt.Sprintf("%v/v2/groups/%v/members/%v", g.api.ProxyAPI, groupID, memberOpenID)
	raw, err := g.api.do("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("get group member: %w", err)
	}
	var member GroupMember
	if err := json.Unmarshal(raw, &member); err != nil {
		return nil, err
	}
	return &member, nil
}

// GetMembers 拉群成员, 内部按 next_cursor 翻页。
func (g *GroupAPI) GetMembers(groupID string) ([]GroupMember, error) {
	var out []GroupMember
	cursor := ""
	for page := 0; page < 40; page++ {
		query := url.Values{}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		endpoint := fmt.Sprintf("%v/v2/groups/%v/members", g.api.ProxyAPI, groupID)
		if encoded := query.Encode(); encoded != "" {
			endpoint += "?" + encoded
		}
		raw, err := g.api.do("GET", endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("get group members: %w", err)
		}
		var resp groupMemberPage
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Members...)
		if resp.NextCursor == "" {
			return out, nil
		}
		cursor = resp.NextCursor
	}
	return out, nil
}
