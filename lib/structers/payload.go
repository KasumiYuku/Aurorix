package structers

import (
	"bytes"
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"strings"
)

// 推送内容解析
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
		return firstOf(d.GroupMemberOpenID, d.UserOpenID, d.Callback.Resolved.UserId)
	case constant.GROUP_JOIN_REQUEST, constant.GROUP_MEMBER_ADD, constant.GROUP_MEMBER_REMOVE:
		return firstOf(d.MemberOpenId, d.Author.UnionID)
	case constant.GROUP_ADD_ROBOT, constant.GROUP_DEL_ROBOT, constant.GROUP_MSG_RECEIVE, constant.GROUP_MSG_REJECT:
		return d.OpMemberOpenID
	case constant.C2C_MSG_RECEIVE, constant.C2C_MSG_REJECT:
		return firstOf(d.OpenID, d.Author.UserOpenID)
	}
	return ""
}

// InteractionIsPrivate 判定交互事件的会话类型。chat_type 与 scene 是平台对同一语义的
func InteractionIsPrivate(chatType constant.ChatType, scene string) bool {
	if chatType != 0 {
		return chatType == constant.ChatTypeDirect
	}
	return scene == constant.SceneC2C
}

// CallbackData 按钮交互事件里 data.resolved 的内容。
type CallbackData struct {
	ButtonData string `json:"button_data"`
	ButtonId   string `json:"button_id"`
	UserId     string `json:"user_id"`
}

// PrasedData 事件载荷(d)。
type PrasedData struct {
	Id           string `json:"id"`
	Content      string `json:"content"`
	GroupOpenID  string `json:"group_openid"`
	GroupID      string `json:"group_id"`
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

// ArkData 结构化卡片数据 —— App 分享到 QQ 时平台已经解析好的那部分, 框架原样透传。
type ArkData struct {
	Prompt    string         `json:"prompt"`
	ArkType   string         `json:"ark_type"`
	ArkName   string         `json:"ark_name"`
	Fields    map[string]any `json:"fields"`
	fieldsRaw []byte
}

// URLs 按平台下发顺序返回卡片里的 http(s) 链接, 已去重。
func (a *ArkData) URLs() []string {
	if a == nil || len(a.fieldsRaw) == 0 {
		return nil
	}
	out := make([]string, 0, 4)
	seen := make(map[string]bool, 4)
	dec := json.NewDecoder(bytes.NewReader(a.fieldsRaw))
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

// UnmarshalJSON 宽松解码: 卡片结构和预期对不上时留空而不是报错。
func (a *ArkData) UnmarshalJSON(data []byte) error {
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
	a.fieldsRaw = probe.Fields
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
