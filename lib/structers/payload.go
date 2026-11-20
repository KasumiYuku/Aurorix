package structers

import (
	"bytes"
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"strings"
)

type Payload struct {
	ID        string             `json:"id"`
	Op        int                `json:"op"`
	Data      PrasedData         `json:"d"`
	T         string             `json:"t"`
	EventType constant.EventType `json:"-"`
	RawEvent  json.RawMessage    `json:"-"`
}

// Type 事件类型; 未注入 EventType 时回退平台事件名。
func (p Payload) Type() constant.EventType {
	if p.EventType != "" {
		return p.EventType
	}
	return constant.EventType(p.T)
}

// ActorID 事件里的操作者/发送者 openid。
func (p Payload) ActorID() string {
	d := p.Data
	switch p.Type() {
	case constant.GROUP_AT_MESSAGE_CREATE, constant.GROUP_MESSAGE_CREATE:
		return firstOf(d.Author.MemberOpenID, d.Author.ID, d.Author.UnionID)
	case constant.C2C_MESSAGE_CREATE:
		return firstOf(d.Author.UserOpenID, d.Author.ID, d.Author.UnionID)
	case constant.INTERACTION_CREATE:
		return firstOf(d.GroupMemberOpenID, d.UserOpenID, d.UserID, d.Callback.Resolved.UserId, d.Author.ID)
	case constant.GROUP_JOIN_REQUEST, constant.GROUP_MEMBER_ADD, constant.GROUP_MEMBER_REMOVE:
		return firstOf(d.MemberOpenId, d.Author.UnionID)
	case constant.GROUP_ADD_ROBOT, constant.GROUP_DEL_ROBOT, constant.GROUP_MSG_RECEIVE, constant.GROUP_MSG_REJECT:
		return d.OpMemberOpenID
	case constant.C2C_MSG_RECEIVE, constant.C2C_MSG_REJECT:
		return firstOf(d.OpenID, d.Author.UserOpenID)
	case constant.AT_MESSAGE_CREATE, constant.MESSAGE_CREATE, constant.DIRECT_MESSAGE_CREATE:
		return firstOf(d.Author.UnionID, d.Author.ID)
	case constant.GUILD_MEMBER_ADD, constant.GUILD_MEMBER_REMOVE:
		return firstOf(d.User.ID, d.Author.ID)
	case constant.GUILD_CREATE, constant.GUILD_UPDATE, constant.GUILD_DELETE,
		constant.CHANNEL_CREATE, constant.CHANNEL_UPDATE, constant.CHANNEL_DELETE:
		return firstOf(d.OpUserID, d.Author.ID)
	}
	return ""
}

// Target 这条载荷属于哪个会话; 场景不同会话 ID 取的字段也不同: 群/单聊 openid, 子频道 channel_id, 频道私信 guild_id。
func (p Payload) Target(scene constant.Scene) constant.Target {
	d := p.Data
	user := p.ActorID()
	switch scene {
	case constant.SceneGroup:
		return constant.Target{Scene: scene, ID: d.GroupOpenID, User: user}
	case constant.SceneChannel:
		return constant.Target{Scene: scene, ID: d.ChannelID, User: user, MentionID: d.Author.ID}
	case constant.SceneDirect:
		return constant.Target{Scene: scene, ID: d.GuildID, User: user, MentionID: d.Author.ID}
	case constant.SceneC2C:
		return constant.Target{Scene: scene, ID: user, User: user}
	}
	return constant.Target{}
}

// InteractionTarget 交互事件落在哪个会话。
func (p Payload) InteractionTarget() constant.Target {
	return p.Target(InteractionScene(p.Data.ChatType, string(p.Data.Scene)))
}

// InteractionScene 判定交互事件落在哪个场景; chat_type 与 scene 是平台对同一语义的两种下发方式, 谁有值取谁。
func InteractionScene(chatType constant.ChatType, scene string) constant.Scene {
	switch chatType {
	case constant.ChatTypeGroup:
		return constant.SceneGroup
	case constant.ChatTypeDirect:
		return constant.SceneC2C
	case constant.ChatTypeChannel:
		return constant.SceneChannel
	}
	switch scene {
	case string(constant.SceneC2C):
		return constant.SceneC2C
	case string(constant.SceneGroup):
		return constant.SceneGroup
	case "guild":
		return constant.SceneChannel
	}
	return ""
}

// CallbackData 按钮交互事件里 data.resolved 的内容。
type CallbackData struct {
	ButtonData string `json:"button_data"`
	ButtonId   string `json:"button_id"`
	UserId     string `json:"user_id"`
}

// PrasedData 事件载荷(d)。
type PrasedData struct {
	Id          string `json:"id"`
	Content     string `json:"content"`
	GroupOpenID string `json:"group_openid"`
	GroupID     string `json:"group_id"`
	ChannelID   string `json:"channel_id"`
	UserID      string `json:"user_id"`
	GuildID     string `json:"guild_id"`
	OpUserID    string `json:"op_user_id"`
	Name        string `json:"name"`
	Nick        string `json:"nick"`
	User        struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Bot      bool   `json:"bot"`
	} `json:"user"`
	MessageScene struct {
		Source string   `json:"source"`
		Ext    []string `json:"ext"`
	} `json:"message_scene"`
	MessageTypeRaw int                  `json:"message_type"`
	MsgTypeRaw     int                  `json:"msg_type"`
	Attachments    []message.Attachment `json:"attachments"`
	Mentions       []Mention            `json:"mentions,omitempty"`
	MsgElements    []MsgElement         `json:"msg_elements,omitempty"`
	ArkData        *ArkData             `json:"ark_data,omitempty"`

	Author struct {
		ID           string                `json:"id"`
		MemberOpenID string                `json:"member_openid"`
		UserOpenID   string                `json:"user_openid"`
		UnionID      string                `json:"union_openid"`
		Role         constant.RoleRequired `json:"member_role"`
		Username     string                `json:"username"`
		Bot          bool                  `json:"bot"`
	} `json:"author"`

	UserOpenID        string            `json:"user_openid"`
	GroupMemberOpenID string            `json:"group_member_openid"`
	ChatType          constant.ChatType `json:"chat_type"`
	Scene             SceneField        `json:"scene"`
	Callback          struct {
		Resolved CallbackData `json:"resolved"`
	} `json:"data"`

	OpenID string `json:"openid"`

	MemberOpenId  string         `json:"member_openid"`
	JoinRequestId string         `json:"join_request_id"`
	VerifyInfo    VerifyInfoData `json:"verify_info"`

	OpMemberOpenID string `json:"op_member_openid"`

	AuditID    string      `json:"audit_id"`
	MessageId  string      `json:"message_id"`
	Timestamp  TimeValue   `json:"timestamp"`
	PlainToken string      `json:"plain_token"`
	EventTs    json.Number `json:"event_ts"`
}

// ArkData 结构化卡片里平台已经解析好的那部分, 框架原样透传; 卡片没有统一形状, 这里只钉住 prompt 与原文本体。
type ArkData struct {
	Prompt  string         `json:"prompt"`
	ArkType string         `json:"ark_type"`
	ArkName string         `json:"ark_name"`
	Fields  map[string]any `json:"fields"`
	// Raw 是卡片的原始 JSON: 形状不认识时, 链接仍能从它里面扫出来。
	Raw json.RawMessage `json:"-"`
}

// URLs 按平台下发顺序返回卡片里的 http(s) 链接, 已去重; 扫整份原文而不只是 fields, 因为小程序卡片把链接放在 meta.detail_1.qqdocurl。
func (a *ArkData) URLs() []string {
	if a == nil {
		return nil
	}
	source := a.Raw
	if len(source) == 0 {
		source = a.fieldsRaw()
	}
	if len(source) == 0 {
		return nil
	}

	out := make([]string, 0, 4)
	seen := make(map[string]bool, 4)
	dec := json.NewDecoder(bytes.NewReader(source))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		s, ok := tok.(string)
		if !ok || !isHTTPURL(s) || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func (a *ArkData) fieldsRaw() []byte {
	if len(a.Fields) == 0 {
		return nil
	}
	raw, err := json.Marshal(a.Fields)
	if err != nil {
		return nil
	}
	return raw
}

// UnmarshalJSON 宽松解码: 卡片结构不认识也留原文、不报错, 否则整条消息的事件解析会一起失败。
func (a *ArkData) UnmarshalJSON(data []byte) error {
	a.Raw = append(a.Raw[:0], data...)

	var probe struct {
		Prompt  string          `json:"prompt"`
		ArkType string          `json:"ark_type"`
		ArkName string          `json:"ark_name"`
		Fields  json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil
	}
	a.Prompt, a.ArkType, a.ArkName = probe.Prompt, probe.ArkType, probe.ArkName
	fields := map[string]any{}
	if err := json.Unmarshal(probe.Fields, &fields); err == nil {
		a.Fields = fields
	}
	return nil
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// MsgType 统一消息类型: 群事件给 message_type, C2C 事件给 msg_type, 谁有值取谁。
func (d PrasedData) MsgType() int {
	if d.MsgTypeRaw != 0 {
		return d.MsgTypeRaw
	}
	return d.MessageTypeRaw
}

// TimeValue 时间字段原文。
type TimeValue string

func (t *TimeValue) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*t = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*t = TimeValue(s)
		return nil
	}
	*t = TimeValue(b)
	return nil
}

// SceneField 交互事件的 scene 标记。平台给的类型随事件而变: INTERACTION_CREATE 给
type SceneField string

func (s *SceneField) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*s = ""
		return nil
	}
	if b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = SceneField(v)
		return nil
	}
	*s = SceneField(b)
	return nil
}

type VerifyInfoData struct {
	Method     string         `json:"method"`
	VerifyMsg  string         `json:"verify_message"`
	AnswerList []QuestionData `json:"review_qa_list"`
}

type QuestionData struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

func firstOf(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
