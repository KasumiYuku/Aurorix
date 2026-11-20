package api

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/requests"
	"github.com/KasumiYuku/Aurorix/lib/state"
	"github.com/KasumiYuku/Aurorix/lib/stats"
)

// Unsupported 这个场景没有这个能力: 频道域没有 v2 富媒体, 群/单聊没有图片直传。
type Unsupported struct {
	Scene constant.Scene
	What  string
}

func (e *Unsupported) Error() string { return e.Scene.Label() + "不支持" + e.What }

// Session 一个会话的出口: 收发地址、撤回方式、可发能力全按场景在这里定, 上层只认 constant.Target。
type Session struct {
	api    *BotAPI
	Target constant.Target
}

func (c *BotAPI) Session(target constant.Target) Session {
	return Session{api: c, Target: target}
}

// Label 给日志用, 形如 "channel/12345"。
func (s Session) Label() string { return s.Target.String() }

func (s Session) messages() (string, error) {
	switch s.Target.Scene {
	case constant.SceneGroup:
		return "/v2/groups/" + s.Target.ID + "/messages", nil
	case constant.SceneC2C:
		return "/v2/users/" + s.Target.ID + "/messages", nil
	case constant.SceneChannel:
		return "/channels/" + s.Target.ID + "/messages", nil
	case constant.SceneDirect:
		return "/dms/" + s.Target.ID + "/messages", nil
	}
	return "", fmt.Errorf("未知会话场景 %q", s.Target.Scene)
}

func (s Session) ready() error {
	if s.api == nil {
		return fmt.Errorf("API 客户端为空")
	}
	if s.Target.Empty() {
		return fmt.Errorf("会话目标为空")
	}
	return nil
}

// Send 发一条已序列化好的消息, 返回平台消息 ID。
func (s Session) Send(data []byte) (string, error) {
	if err := s.ready(); err != nil {
		return "", err
	}
	path, err := s.messages()
	if err != nil {
		return "", err
	}
	result, err := s.api.sendWithRetry(s.api.ProxyAPI+path, data)
	if err != nil {
		return "", err
	}
	state.IncSent()
	stats.Sent()
	if result == nil {
		return "", nil
	}
	return result.ID, nil
}

// Recall 撤回一条消息; hideTip 为 true 时不再提示"撤回了一条消息"。
func (s Session) Recall(messageID string, hideTip bool) error {
	if err := s.ready(); err != nil {
		return err
	}
	path, err := s.messages()
	if err != nil {
		return err
	}
	url := s.api.ProxyAPI + path + "/" + messageID
	if hideTip {
		url += "?hidetip=true"
	}
	_, err = s.api.do("DELETE", url, nil)
	return err
}

// Anchor 被动回复锚点。频道域发图也要带上它, 否则那条就成了一条主动消息(额度另算)。
type Anchor struct {
	MsgID   string
	EventID string
	Seq     uint8
}

func (a Anchor) jsonFields() map[string]any {
	out := map[string]any{}
	if a.MsgID != "" {
		out["msg_id"] = a.MsgID
	}
	if a.EventID != "" {
		out["event_id"] = a.EventID
	}
	if a.Seq > 0 {
		out["msg_seq"] = a.Seq
	}
	return out
}

func (a Anchor) formFields() [][2]string {
	out := make([][2]string, 0, 3)
	if a.MsgID != "" {
		out = append(out, [2]string{"msg_id", a.MsgID})
	}
	if a.EventID != "" {
		out = append(out, [2]string{"event_id", a.EventID})
	}
	if a.Seq > 0 {
		out = append(out, [2]string{"msg_seq", strconv.Itoa(int(a.Seq))})
	}
	return out
}

// GuildImage 频道域的一张图: 给 URL 让平台转存, 给字节走多部件直传。
type GuildImage struct {
	Content  string
	URL      string
	Data     []byte
	Filename string
	MimeType string
}

// SendGuildImage 频道域发图: 报文是 content + image, 与文本/markdown 形状不同, 所以单独一条路。
func (s Session) SendGuildImage(img GuildImage, anchor Anchor) (string, error) {
	if err := s.ready(); err != nil {
		return "", err
	}
	if !s.Target.Scene.Guild() {
		return "", &Unsupported{Scene: s.Target.Scene, What: "图片消息"}
	}
	path, err := s.messages()
	if err != nil {
		return "", err
	}
	if img.URL != "" {
		return s.sendImageJSON(path, img, anchor)
	}
	if len(img.Data) == 0 {
		return "", fmt.Errorf("图片既没有 URL 也没有字节")
	}
	return s.sendImageBytes(path, img, anchor)
}

func (s Session) sendImageJSON(path string, img GuildImage, anchor Anchor) (string, error) {
	body := map[string]any{"image": img.URL}
	if img.Content != "" {
		body["content"] = img.Content
	}
	for k, v := range anchor.jsonFields() {
		body[k] = v
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	result, err := s.api.sendWithRetry(s.api.ProxyAPI+path, data)
	if err != nil {
		return "", err
	}
	state.IncSent()
	stats.Sent()
	if result == nil {
		return "", nil
	}
	return result.ID, nil
}

func (s Session) sendImageBytes(path string, img GuildImage, anchor Anchor) (string, error) {
	mp := requests.NewMultipart()
	mp.AddField("content", img.Content)
	for _, field := range anchor.formFields() {
		mp.AddField(field[0], field[1])
	}
	mp.AddFile("file_image", img.Filename, img.MimeType, img.Data)
	mp.Close()

	header, err := s.api.generateHeader()
	if err != nil {
		return "", err
	}
	delete(header, "Content-Type")
	raw, err := s.api.Request.DoBytesTimeout("POST", s.api.ProxyAPI+path, mp, header, uploadBudget(len(img.Data)))
	if err != nil {
		if qe := asQQError(err); qe != nil {
			return "", qe
		}
		return "", err
	}
	var result SendMessageResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("解析发图回执失败: %w", err)
	}
	state.IncSent()
	stats.Sent()
	return result.ID, nil
}
