package api

import (
	"encoding/json"
	"fmt"
)

// ChannelAPI 频道域资料: 频道、子频道与成员。
type ChannelAPI struct {
	api *BotAPI
}

// GuildInfo 频道资料。
type GuildInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	OwnerID     string `json:"owner_id"`
	MemberCount int    `json:"member_count"`
	Description string `json:"description"`
}

// ChannelInfo 子频道资料。
type ChannelInfo struct {
	ID       string `json:"id"`
	GuildID  string `json:"guild_id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	Position int    `json:"position"`
	ParentID string `json:"parent_id"`
	OwnerID  string `json:"owner_id"`
}

// GuildMember 频道成员。频道里 user.id 就是平台用户标识(不是 openid)。
type GuildMember struct {
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Avatar   string `json:"avatar"`
		Bot      bool   `json:"bot"`
	} `json:"user"`
	Nick     string   `json:"nick"`
	Roles    []string `json:"roles"`
	JoinedAt string   `json:"joined_at"`
}

func (c *ChannelAPI) get(url string, out any) error {
	raw, err := c.api.do("GET", c.api.ProxyAPI+url, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("解析频道接口响应失败: %w", err)
	}
	return nil
}

// Info 查子频道资料(含子频道名)。
func (c *ChannelAPI) Info(channelID string) (*ChannelInfo, error) {
	var info ChannelInfo
	if err := c.get("/channels/"+channelID, &info); err != nil {
		return nil, fmt.Errorf("get channel info: %w", err)
	}
	return &info, nil
}

// Guild 查频道资料(含频道名)。
func (c *ChannelAPI) Guild(guildID string) (*GuildInfo, error) {
	var info GuildInfo
	if err := c.get("/guilds/"+guildID, &info); err != nil {
		return nil, fmt.Errorf("get guild info: %w", err)
	}
	return &info, nil
}

// Guilds 机器人所在的频道列表。
func (c *ChannelAPI) Guilds() ([]GuildInfo, error) {
	var list []GuildInfo
	if err := c.get("/users/@me/guilds", &list); err != nil {
		return nil, fmt.Errorf("list guilds: %w", err)
	}
	return list, nil
}

// Channels 某个频道下的子频道列表。
func (c *ChannelAPI) Channels(guildID string) ([]ChannelInfo, error) {
	var list []ChannelInfo
	if err := c.get("/guilds/"+guildID+"/channels", &list); err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	return list, nil
}

// Member 频道成员资料。
func (c *ChannelAPI) Member(guildID, userID string) (*GuildMember, error) {
	var member GuildMember
	if err := c.get("/guilds/"+guildID+"/members/"+userID, &member); err != nil {
		return nil, fmt.Errorf("get guild member: %w", err)
	}
	return &member, nil
}
