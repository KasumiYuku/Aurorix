package event

// 内置事件解析器: 从 PrasedData 提取纯数据事件对象。
// 解析不做严格校验, 缺字段时事件对象保持零值, 由订阅者自行判断。

import (
	"strconv"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/structers"
	"github.com/KasumiYuku/Aurorix/lib/utils"
)

// parseMessageEvent 群/私聊消息事件。群消息清洗机器人@, 私聊去首尾空白, 与指令路由口径一致。
func parseMessageEvent(origin constant.MessageOrigin) Parser {
	return func(p structers.Payload) Event {
		d := p.Data
		raw := d.Content
		content := raw
		if origin == constant.GroupMessage {
			content = utils.FilterAt(content)
		} else {
			content = strings.TrimSpace(content)
		}
		return &MessageEvent{
			eventType:   p.EventType,
			EventID:     p.ID,
			MessageID:   d.Id,
			GroupID:     d.GroupOpenID,
			UserID:      firstOf(d.Author.MemberOpenID, d.Author.UserOpenID, d.Author.UnionID),
			Origin:      origin,
			Content:     content,
			RawContent:  raw,
			Mentions:    d.Mentions,
			Attachments: d.Attachments,
			Quote:       structers.ExtractQuote(d.MsgElements),
		}
	}
}

// parseInteraction 按钮交互事件。
func parseInteraction(p structers.Payload) Event {
	d := p.Data
	return &InteractionEvent{
		EventID:   p.ID,
		ButtonID:  d.Callback.Resolved.ButtonId,
		Data:      d.Callback.Resolved.ButtonData,
		GroupID:   d.GroupOpenID,
		UserID:    firstOf(d.Author.MemberOpenID, d.Author.UserOpenID, d.Author.UnionID),
		MessageID: d.Id,
		Scene:     d.Scene,
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
func parseAudit(approved bool) Parser {
	return func(p structers.Payload) Event {
		return &AuditEvent{
			AuditID:   p.Data.AuditID,
			MessageID: p.Data.MessageId,
			Approved:  approved,
		}
	}
}

// parseMember 群成员变动事件。
func parseMember(added bool) Parser {
	return func(p structers.Payload) Event {
		d := p.Data
		t := constant.GROUP_MEMBER_REMOVE
		if added {
			t = constant.GROUP_MEMBER_ADD
		}
		return &MemberEvent{
			eventType:  t,
			GroupID:    d.GroupOpenID,
			UserID:     firstOf(d.Author.MemberOpenID, d.Author.UserOpenID, d.Author.UnionID),
			OperatorID: d.Author.UnionID,
			Timestamp:  parseTs(d.EventTs),
			Added:      added,
		}
	}
}

// parseRobot 机器人在群状态变动事件。
func parseRobot(added bool) Parser {
	return func(p structers.Payload) Event {
		d := p.Data
		t := constant.GROUP_DEL_ROBOT
		if added {
			t = constant.GROUP_ADD_ROBOT
		}
		return &RobotEvent{
			eventType:  t,
			GroupID:    d.GroupOpenID,
			OperatorID: d.Author.UnionID,
			Timestamp:  parseTs(d.EventTs),
			Added:      added,
		}
	}
}

// parseGroupReceive 群消息接收开关事件。
func parseGroupReceive(enabled bool) Parser {
	return func(p structers.Payload) Event {
		d := p.Data
		t := constant.GROUP_MSG_REJECT
		if enabled {
			t = constant.GROUP_MSG_RECEIVE
		}
		return &ReceiveEvent{
			eventType:  t,
			GroupID:    d.GroupOpenID,
			OperatorID: d.Author.UnionID,
			Timestamp:  parseTs(d.EventTs),
			Enabled:    enabled,
		}
	}
}

// parsePrivateReceive 私聊消息接收开关事件。
func parsePrivateReceive(enabled bool) Parser {
	return func(p structers.Payload) Event {
		d := p.Data
		t := constant.C2C_MSG_REJECT
		if enabled {
			t = constant.C2C_MSG_RECEIVE
		}
		return &ReceiveEvent{
			eventType:  t,
			UserID:     d.Author.UserOpenID,
			OperatorID: d.Author.UnionID,
			Timestamp:  parseTs(d.EventTs),
			Enabled:    enabled,
		}
	}
}

// firstOf 返回第一个非空值。
func firstOf(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// parseTs 事件时间戳: 秒/毫秒字符串转秒, 非法返回 0。
func parseTs(s string) int64 {
	if s == "" {
		return 0
	}
	ts, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	if ts > 1e12 { // 毫秒转秒
		ts /= 1000
	}
	return ts
}
