package message

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/contract"
	errorx "github.com/KasumiYuku/Aurorix/lib/error"
	"github.com/KasumiYuku/Aurorix/lib/logx"
)

var logger = logx.New("message")

// Message 所有出站消息的公共字段, 也是**全框架唯一的发送出口**(见 SendWithID)。
type Message struct {
	*MsgRef
	MsgId     string               `json:"msg_id,omitempty"`
	MsgSeq    uint8                `json:"msg_seq,omitempty"`
	EventId   string               `json:"event_id,omitempty"`
	Type      constant.MessageType `json:"msg_type"`
	Reference *MessageReference    `json:"message_reference,omitempty"`
	Qapi      *api.BotAPI          `json:"-"`
	// Target 这条消息发给谁; 降级成功后会改成真实落点(群消息退到单聊时就是单聊)。
	Target           constant.Target `json:"-"`
	guildImage       *api.GuildImage
	MarshalInterface contract.CanMarshal `json:"-"`
	initiativePush   bool                `json:"-"`
	deliveredActive  bool                `json:"-"`
	activeFallback   bool                `json:"-"`
	// OriginalInput 触发这条回复的原消息文本(主动推送时为空); 底部「复读」按钮用它回填输入框。
	OriginalInput string `json:"-"`
	// NoFooter 为 true 时这条消息不追加框架级底部按钮。
	NoFooter bool `json:"-"`
	// Trace 与 Command 是这条消息的排查线索: 图床/降级等日志靠它们对齐到"哪条消息、哪个指令"。
	Trace   string `json:"-"`
	Command string `json:"-"`
}

// DeliveredActive 报告这条消息是不是走主动通道发出去的; 主动推送与降级过来的都算。
func (msg *Message) DeliveredActive() bool {
	return msg.deliveredActive || msg.initiativePush
}

// MessageReference 引用回复：message_id 为被引用的原消息 ID。
type MessageReference struct {
	MessageID             string `json:"message_id"`
	IgnoreGetMessageError bool   `json:"ignore_get_message_error"`
}

// QuoteTo 让本条消息引用原消息。
func (msg *Message) QuoteTo(messageID string) {
	msg.Reference = &MessageReference{MessageID: messageID}
}

func (msg *Message) InitRef() *Message {
	msg.MsgRef = &MsgRef{}
	msg.activeFallback = true
	return msg
}

// DisableActiveFallback 关掉被动窗口失效后的主动重发, 给"宁可失败也不要主动消息"的场景留口子。
func (msg *Message) DisableActiveFallback() *Message {
	msg.activeFallback = false
	return msg
}

func (msg *Message) SetInitiativeMessage() {
	msg.initiativePush = true
}

func (msg *Message) Send() error {
	_, err := msg.SendWithID()
	return err
}

// SendWithID 发送消息并返回平台消息 ID。被动回复在平台侧有有效期, 所以按"被动 → 主动"依次尝试, 群聊再多一级退到本人单聊; 只有窗口过期或本地预算用尽才降级。
func (msg *Message) SendWithID() (string, error) {
	steps := msg.deliverySteps()
	id, err := msg.sendOnce(steps[0])
	if err == nil {
		return id, nil
	}
	if !passiveGone(err) || !msg.activeFallback {
		return "", err
	}
	msg.logFallback(err)

	failed := make([]error, 0, len(steps)-1)
	for _, step := range steps[1:] {
		if id, ferr := msg.sendOnce(step); ferr == nil {
			return id, nil
		} else {
			failed = append(failed, fmt.Errorf("%s: %w", step.target.String(), ferr))
		}
	}
	all := errors.Join(failed...)
	msg.logFallbackFailed(all)
	return "", all
}

type deliveryStep struct {
	target constant.Target
	active bool
}

func (msg *Message) deliverySteps() []deliveryStep {
	t := msg.Target
	steps := []deliveryStep{{target: t}, {target: t, active: true}}
	if t.Scene == constant.SceneGroup && t.User != "" {
		steps = append(steps, deliveryStep{
			target: constant.Target{Scene: constant.SceneC2C, ID: t.User, User: t.User},
			active: true,
		})
	}
	return steps
}

func passiveGone(err error) bool {
	var limit *ReplyMessageReachLimit
	return errorx.IsPassiveExpired(err) || errors.As(err, &limit)
}

func (msg *Message) sendOnce(step deliveryStep) (string, error) {
	active := step.active || msg.initiativePush
	if msg.Type == constant.Media && step.target.Scene.Guild() {
		return "", &api.Unsupported{Scene: step.target.Scene, What: "语音/视频/文件消息"}
	}
	if !active {
		if msg.MsgRef != nil && msg.MsgRef.Spent() {
			return "", &ReplyMessageReachLimit{}
		}
		seq, err := msg.Count()
		if err != nil {
			return "", err
		}
		msg.MsgSeq = seq
	} else {
		msg.EventId, msg.MsgId, msg.MsgSeq, msg.Reference = "", "", 0, nil
	}

	session := msg.Qapi.Session(step.target)
	if step.target.Scene.Guild() {
		msg.EventId, msg.MsgSeq = "", 0
	}
	anchor := api.Anchor{MsgID: msg.MsgId, EventID: msg.EventId, Seq: msg.MsgSeq}
	var id string
	var err error
	if msg.guildImage != nil && step.target.Scene.Guild() {
		id, err = session.SendGuildImage(*msg.guildImage, anchor)
	} else {
		var data []byte
		if data, err = msg.marshal(); err == nil {
			id, err = session.Send(data)
		}
	}
	if err != nil {
		return "", err
	}
	msg.Target = step.target
	if active {
		msg.deliveredActive = true
	}
	observeSent(msg, id)
	return id, nil
}

func (msg *Message) marshal() ([]byte, error) {
	var data []byte
	var err error
	if pre, ok := msg.MarshalInterface.(contract.PreSend); ok {
		pre.Prepare()
	}
	if msg.MarshalInterface != nil {
		data, err = msg.MarshalInterface.Marshal()
	} else {
		data, err = json.Marshal(msg)
	}
	if err != nil {
		return nil, &JSONMarshalError{Err: err}
	}
	return data, nil
}

func (msg *Message) logFallback(cause error) {
	logger.WithTrace(msg.Trace).Warnf("被动回复窗口已失效，改走主动通道重发 target=%s user=%s cmd=%s err=%v",
		msg.Target.String(), msg.Target.User, msg.Command, cause)
}

func (msg *Message) logFallbackFailed(cause error) {
	logger.WithTrace(msg.Trace).Errorf("主动降级也失败 target=%s user=%s cmd=%s err=%v",
		msg.Target.String(), msg.Target.User, msg.Command, cause)
}

type UserMessage struct {
	Content     string
	Attachments []Attachment
}

type Attachment struct {
	ContentType string `json:"content_type"`
	Filename    string `json:"filename"`
	Height      int    `json:"height"`
	Width       int    `json:"width"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
}
