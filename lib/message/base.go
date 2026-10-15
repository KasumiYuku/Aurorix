package message

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/contract"
	errorx "github.com/KasumiYuku/Aurorix/lib/error"
	"github.com/KasumiYuku/Aurorix/lib/logx"
)

// logger 降级投递的日志口。它记的是"用户做了什么才拿到消息"，
// 属于排查链路的一环，不该混进业务插件的日志里（故 scope 取 message）。

// logger 降级投递的日志口（scope 取 message，不混进业务插件的日志）。
var logger = logx.New("message")

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
	// activeFallback 被动回复窗口失效时，是否允许改走主动通道重发（见 SendWithID）。
	// 由 InitRef 置为默认开启；DisableActiveFallback 可关掉。
	activeFallback bool `json:"-"`
	// OriginalInput 触发这条回复的原消息文本(主动推送时为空); 底部「复读」按钮用它把原文回填到输入框。
	OriginalInput string `json:"-"`
	// NoFooter 为 true 时这条消息不追加框架级底部按钮。
	NoFooter bool `json:"-"`
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
	//降级默认开：消息的**唯一出口**就在这里，而"被动窗口过期"是长任务的常态
	//（见 SendWithID 的说明）。默认关等于每个调用方都要记得手动打开一次，
	// 而漏掉的那个必然是最慢、最需要它的那条路径。
	msg.activeFallback = true
	return msg
}

// DisableActiveFallback 关掉被动窗口失效后的主动重发。
//
// 给"宁可失败也不要主动消息"的场景留口子：主动消息在 QQ 侧有频控与去重，
// 有些会话确实不希望收到没有引用上下文的消息。
func (msg *Message) DisableActiveFallback() *Message {
	msg.activeFallback = false
	return msg
}

// logFallback 记一条"被动窗口失效、已改走主动通道"。
//
// 这条必须留：降级是**静默的体验变化**（消息不再引用原消息、也可能从群退到私聊），
// 不记的话，下次有人问"为什么这条消息没有引用"就无从查起。
func (msg *Message) logFallback(cause error) {
	logger.Warnf("被动回复窗口已失效，改走主动通道重发 target=%v group=%s user=%s err=%v",
		msg.Target, msg.GroupId, msg.UserId, cause)
}

// logFallbackFailed 三级都失败时的收尾日志。
//
// 这是**用户真的一条消息都收不到**的情况，必须能一眼看出卡在哪一级：
// 主动群聊失败往往是"机器人不在群里/被禁言"，主动私聊失败往往是"用户没加机器人好友"，
// 两者的处置完全不同（前者要拉群，后者要引导加好友）。
func (msg *Message) logFallbackFailed(groupErr, privateErr error) {
	if privateErr != nil {
		logger.Errorf("主动降级也失败：群聊与私聊都发不出去 group=%s user=%s 群聊err=%v 私聊err=%v",
			msg.GroupId, msg.UserId, groupErr, privateErr)
		return
	}
	logger.Errorf("主动降级也失败 group=%s user=%s err=%v", msg.GroupId, msg.UserId, groupErr)
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
//
// # 三级投递降级
//
// 被动回复（回复用户那条消息）在 QQ 侧有**有效期**。长任务（一次冷启动全量扫描实测
// 4m58s）等结果出来时窗口早就过了，于是"图已经渲染好、也传到了图床，就是发不出去"
// —— 2026-10-14 现场：`ERROR 消息已过期，不能回复`，用户什么都没收到。
//
// 所以这里按 被动 → 主动群聊 → 主动私聊 依次尝试：
//  1. **被动回复**：正常路径，带引用灰条，最自然；
//  2. **主动群聊**：被动窗口失效时改走它 —— 消息照样落到群里，只是不再引用原消息、也没有 event_id；
//  3. **主动私聊**：群聊也发不出去（非群成员 / 被禁言 / 无权限）时退到私聊用户本人。
//
// 为什么放在这个函数里、而不是各插件自己处理：**它是所有消息的唯一出口**
// （全框架的群/私聊分派就在下面那个 switch 里）。在这里加工一次，
// arcaea / phigros / 以后任何插件都自动获得这个能力，一行都不用改。
//
// 只有"被动窗口失效"这一类错误会触发降级（判据见 errorx.IsPassiveExpired）；
// 内容违规、参数错误这些换通道也发不出去，重发只是把一条注定失败的请求再打一遍。
func (msg *Message) SendWithID() (string, error) {
	if msg.used {
		return "", &MessageUsed{
			MessageId: msg.MsgId,
		}
	}
	id, err := msg.sendOnce(false)
	if err == nil {
		return id, nil
	}
	if !errorx.IsPassiveExpired(err) || !msg.activeFallback {
		return "", err
	}
	//走到这里说明"这条消息发得出去，只是不能再回复那条了" —— 改走主动通道。
	//主动模式复用现成的 initiativePush 分支：它会把 MsgId / EventId / MsgSeq / Reference
	// 一并清空（清 MsgSeq 尤其关键，被动序号在主动消息上没有意义，留着会被判重复）。
	msg.logFallback(err)
	if id, aerr := msg.sendOnce(true); aerr == nil {
		return id, nil
	} else if msg.Target != constant.PrivateMessage {
		//群发不出去（非群成员/被禁言/无权限）：退到私聊用户本人。
		//这是最后一级 —— 用户至少能拿到结果，而不是一无所获。
		msg.Target = constant.PrivateMessage
		msg.GroupId = ""
		if id, perr := msg.sendOnce(true); perr == nil {
			return id, nil
		} else {
			msg.logFallbackFailed(aerr, perr)
			return "", perr
		}
	} else {
		msg.logFallbackFailed(aerr, nil)
		return "", aerr
	}
}

// sendOnce 执行一次投递；initiative 为真时走主动通道。
func (msg *Message) sendOnce(initiative bool) (string, error) {
	seq, err := msg.Count()
	if err != nil {
		return "", err
	}
	msg.MsgSeq = seq
	if initiative || msg.initiativePush {
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
