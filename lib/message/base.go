package message

import (
	"Plrx/lib/api"
	"Plrx/lib/constant"
	"Plrx/lib/contract"
	"encoding/json"
	"fmt"
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
	Target           constant.MessageOrigin `json:"-"` // 发送目标(私聊/群)
	used             bool                   `json:"-"` // 是否被使用过
	MarshalInterface contract.CanMarshal    `json:"-"`
	initiativePush   bool                   `json:"-"` // 是否为主动推送
}

// MessageReference 引用回复：message_id 为被引用的原消息 ID。
type MessageReference struct {
	MessageID string `json:"message_id"`
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
	if msg.used {
		return &MessageUsed{
			MessageId: msg.MsgId,
		}
	}
	seq, err := msg.Count()
	if err != nil {
		return err
	}
	msg.MsgSeq = seq

	var data []byte
	// 发送前预处理（markdown 图片内嵌、@保护等）
	if pre, ok := msg.MarshalInterface.(contract.PreSend); ok {
		pre.Prepare()
	}
	if msg.MarshalInterface != nil {
		data, err = msg.MarshalInterface.Marshal()
	} else {
		data, err = json.Marshal(msg)
	}
	if err != nil {
		return &JSONMarshalError{
			Err: err,
		}
	}

	if msg.Qapi == nil {
		panic("QQAPI Clinet空指针异常")
	}

	switch msg.Target {
	case constant.GroupMessage:
		return msg.Qapi.SendGroupMessage(data, msg.GroupId)
	case constant.PrivateMessage:
		return msg.Qapi.SendPrivateMessage(data, msg.UserId)
	default:
		return fmt.Errorf("Unknown message target type: %v", msg.Target)
	}
}

// 设置为主动推送消息
func (msg *Message) SetInitiativeMessage() {
	msg.initiativePush = true
}
