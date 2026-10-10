package event

import (
	"strconv"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/structers"
	"github.com/KasumiYuku/Aurorix/lib/utils"
)

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
			UserID:      p.ActorID(),
			Origin:      origin,
			Content:     content,
			RawContent:  raw,
			Mentions:    d.Mentions,
			Attachments: d.Attachments,
			Quote:       structers.ExtractQuote(d.MsgElements),
			MsgType:     d.MsgType(),
			Ark:         d.ArkData,
			Elements:    d.MsgElements,
			Raw:         p.RawEvent,
		}
	}
}

func parseInteraction(p structers.Payload) Event {
	d := p.Data
	return &InteractionEvent{
		EventID:   p.ID,
		ButtonID:  d.Callback.Resolved.ButtonId,
		Data:      d.Callback.Resolved.ButtonData,
		GroupID:   d.GroupOpenID,
		UserID:    p.ActorID(),
		MessageID: d.Id,
		Scene:     string(d.Scene),
		ChatType:  d.ChatType,
	}
}

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
		UserID:    p.ActorID(),
		Method:    d.VerifyInfo.Method,
		Answer:    answer,
		MessageID: d.MessageId,
	}
}

func parseAudit(approved bool) Parser {
	return func(p structers.Payload) Event {
		return &AuditEvent{
			AuditID:   p.Data.AuditID,
			MessageID: p.Data.MessageId,
			Approved:  approved,
		}
	}
}

func parseMember(added bool) Parser {
	return func(p structers.Payload) Event {
		d := p.Data
		t := constant.GROUP_MEMBER_REMOVE
		if added {
			t = constant.GROUP_MEMBER_ADD
		}
		return &MemberEvent{
			eventType:  t,
			EventID:    p.ID,
			MessageID:  d.MessageId,
			GroupID:    d.GroupOpenID,
			UserID:     p.ActorID(),
			OperatorID: "",
			Timestamp:  parseTs(d.Timestamp),
			Added:      added,
		}
	}
}

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
			OperatorID: p.ActorID(),
			Timestamp:  parseTs(d.Timestamp),
			Added:      added,
		}
	}
}

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
			OperatorID: p.ActorID(),
			Timestamp:  parseTs(d.Timestamp),
			Enabled:    enabled,
		}
	}
}

func parsePrivateReceive(enabled bool) Parser {
	return func(p structers.Payload) Event {
		d := p.Data
		t := constant.C2C_MSG_REJECT
		if enabled {
			t = constant.C2C_MSG_RECEIVE
		}
		return &ReceiveEvent{
			eventType:  t,
			UserID:     p.ActorID(),
			OperatorID: p.ActorID(),
			Timestamp:  parseTs(d.Timestamp),
			Enabled:    enabled,
		}
	}
}

func parseFriend(added bool) Parser {
	return func(p structers.Payload) Event {
		d := p.Data
		t := constant.FRIEND_DEL
		if added {
			t = constant.FRIEND_ADD
		}
		return &FriendEvent{
			eventType: t,
			UserID:    d.OpenID,
			Timestamp: parseTs(d.Timestamp),
			Added:     added,
		}
	}
}

func parseTs(v structers.TimeValue) int64 {
	if v == "" {
		return 0
	}
	ts, err := strconv.ParseFloat(string(v), 64)
	if err != nil {
		return 0
	}
	sec := int64(ts)
	if sec > 1e12 {
		sec /= 1000
	}
	return sec
}
