package message

import (
	"encoding/json"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/contract"
	"sync"
)

type Message struct {
	*MsgRef
	MsgId            string                 `json:"msg_id,omitempty"`
	MsgSeq           uint8                  `json:"msg_seq,omitempty"`
	EventId          string                 `json:"event_id,omitempty"`
	Type             constant.MessageType   `json:"msg_type"`
	Reference        *MessageReference      `json:"message_reference,omitempty"`
	Qapi             *api.BotAPI            `json:"-"`
	GroupId          string                 `json:"-"`
	UserId           string                 `json:"-"`
	Target           constant.MessageOrigin `json:"-"`
	used             bool                   `json:"-"`
	MarshalInterface contract.CanMarshal    `json:"-"`
	initiativePush   bool                   `json:"-"`
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

// 初始化回复计数器
func (msg *Message) InitRef() *Message {
	msg.MsgRef = &MsgRef{
		msgSeq: 1,
		lock:   sync.Mutex{},
	}
	return msg
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

// Send 发送消息。
func (msg *Message) Send() error {
	_, err := msg.SendWithID()
	return err
}

// SendWithID 发送消息并返回平台消息 ID, 供撤回等后续操作使用。
func (msg *Message) SendWithID() (string, error) {
	if msg.used {
		return "", &MessageUsed{
			MessageId: msg.MsgId,
		}
	}
	seq, err := msg.Count()
	if err != nil {
		return "", err
	}
	msg.MsgSeq = seq
	if msg.initiativePush {
		msg.EventId = ""
		msg.MsgId = ""
		msg.MsgSeq = 0
		msg.Reference = nil
	}

	var data []byte
	if pre, ok := msg.MarshalInterface.(contract.PreSend); ok {
		pre.Prepare()
	}
	if msg.MarshalInterface != nil {
		data, err = msg.MarshalInterface.Marshal()
	} else {
		data, err = json.Marshal(msg)
	}
	if err != nil {
		return "", &JSONMarshalError{
			Err: err,
		}
	}

	if msg.Qapi == nil {
		panic("QQAPI Clinet空指针异常")
	}

	switch msg.Target {
	case constant.GroupMessage:
		return msg.Qapi.SendGroupMessageID(data, msg.GroupId)
	case constant.PrivateMessage:
		return msg.Qapi.SendPrivateMessageID(data, msg.UserId)
	default:
		return "", fmt.Errorf("Unknown message target type: %v", msg.Target)
	}
}

// 设置为主动推送消息
func (msg *Message) SetInitiativeMessage() {
	msg.initiativePush = true
}
