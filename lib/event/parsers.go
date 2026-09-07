package event

// 内置事件解析器: 已知事件从 PrasedData 提取公共视图。
// 解析不做严格校验, 缺字段时事件对象保持零值, 由订阅者自行判断。

import (
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/structers"
	"github.com/KasumiYuku/Aurorix/lib/utils"
)

func init() {
	RegisterParser(constant.INTERACTION_CREATE, parseInteraction)
	RegisterParser(constant.C2C_MESSAGE_CREATE, func(p structers.Payload) Event { return parseMessage(p, constant.PrivateMessage) })
	RegisterParser(constant.GROUP_AT_MESSAGE_CREATE, func(p structers.Payload) Event { return parseMessage(p, constant.GroupMessage) })
	RegisterParser(constant.GROUP_MESSAGE_CREATE, func(p structers.Payload) Event { return parseMessage(p, constant.GroupMessage) })
	RegisterParser(constant.GROUP_JOIN_REQUEST, parseJoinRequest)
	RegisterParser(constant.MESSAGE_AUDIT_PASS, func(p structers.Payload) Event { return parseAudit(p, true) })
	RegisterParser(constant.MESSAGE_AUDIT_REJECT, func(p structers.Payload) Event { return parseAudit(p, false) })
}

// parseMessage 群/私聊消息事件。群消息清洗机器人@, 私聊去首尾空白, 与指令路由口径一致。
func parseMessage(p structers.Payload, origin constant.MessageOrigin) Event {
	d := p.Data
	raw := d.Content
	content := raw
	if origin == constant.GroupMessage {
		content = utils.FilterAt(content)
	} else {
		content = strings.TrimSpace(content)
	}
	userID := d.Author.MemberOpenID
	if userID == "" {
		userID = d.Author.UserOpenID
	}
	if userID == "" {
		userID = d.Author.UnionID
	}
	return &MessageEvent{
		eventType:   p.EventType,
		EventID:     p.ID,
		MessageID:   d.Id,
		GroupID:     d.GroupOpenID,
		UserID:      userID,
		Origin:      origin,
		Content:     content,
		RawContent:  raw,
		Mentions:    d.Mentions,
		Attachments: d.Attachments,
		Quote:       structers.ExtractQuote(d.MsgElements),
	}
}

// parseInteraction 按钮交互事件。
func parseInteraction(p structers.Payload) Event {
	d := p.Data
	userID := d.Author.MemberOpenID
	if userID == "" {
		userID = d.Author.UserOpenID
	}
	if userID == "" {
		userID = d.Author.UnionID
	}
	return &InteractionEvent{
		EventID:   p.ID,
		ButtonID:  d.Callback.Resolved.ButtonId,
		Data:      d.Callback.Resolved.ButtonData,
		GroupID:   d.GroupOpenID,
		UserID:    userID,
		MessageID: d.Id,
	}
}

// parseJoinRequest 入群申请事件, 按验证方式提取回答。
func parseJoinRequest(p structers.Payload) Event {
	d := p.Data
	var answer string
	switch d.VerifyInfo.Method {
	case "verify_message":
		answer = d.VerifyInfo.VerifyMsg
	case "admin_review_qa":
		if len(d.VerifyInfo.AnswerList) > 0 {
			answer = d.VerifyInfo.AnswerList[0].Answer
		}
	}
	return &JoinRequestEvent{
		RequestID: d.JoinRequestId,
		GroupID:   d.GroupOpenID,
		UserID:    d.Author.UserOpenID,
		Method:    d.VerifyInfo.Method,
		Answer:    answer,
		MessageID: d.MessageId,
	}
}

// parseAudit 消息审核结果事件。
func parseAudit(p structers.Payload, approved bool) Event {
	return &AuditEvent{
		AuditID:   p.Data.AuditID,
		MessageID: p.Data.MessageId,
		Approved:  approved,
	}
}
