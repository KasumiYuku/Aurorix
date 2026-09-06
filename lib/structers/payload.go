package structers

import (
	"Plrx/lib/constant"
	"Plrx/lib/message"
	"encoding/json"
)

// 推送内容解析
type Payload struct {
	ID        string             `json:"id"`
	Op        int                `json:"op"`
	Data      PrasedData         `json:"d"`
	T         string             `json:"t"`
	EventType constant.EventType `json:"-"`
	RawEvent  json.RawMessage    `json:"-"` // 平台原始事件载荷(d), 未知事件透传用
}

type CallbackData struct {
	ButtonData string `json:"button_data"`
	ButtonId   string `json:"button_id"`
}

type PrasedData struct {
	Id           string               `json:"id"`
	Content      string               `json:"content"`
	GroupOpenID  string               `json:"group_openid"`
	MemberOpenId string               `json:"member_openid"` // 入群验证时传递的member_id
	Attachments  []message.Attachment `json:"attachments"`
	Mentions     []Mention            `json:"mentions,omitempty"`
	MsgElements  []MsgElement         `json:"msg_elements,omitempty"`
	Author       struct {
		ID           string                `json:"id"`
		UserOpenID   string                `json:"user_openid"`
		MemberOpenID string                `json:"member_openid"`
		UnionID      string                `json:"union_openid"`
		Role         constant.RoleRequired `json:"member_role"`
		Username     string                `json:"username"`
		Bot          bool                  `json:"bot"`
	} `json:"author"`
	// Op=13 网络探测数据
	PlainToken string `json:"plain_token"`
	EventTs    string `json:"event_ts"`
	Scene      string `json:"scene"`
	Callback   struct {
		Resolved CallbackData `json:"resolved"`
	} `json:"data"`
	JoinRequestId string         `json:"join_request_id"`
	VerifyInfo    VerifyInfoData `json:"verify_info"`
	AuditID       string         `json:"audit_id"`
	MessageId     string         `json:"message_id"`
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
