package message

import (
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// SentMessage 是一条真正发出去的消息的摘要。存档、审计这类旁路只需要这些。
type SentMessage struct {
	MsgID   string
	GroupID string
	UserID  string
	Target  constant.MessageOrigin
	Body    string
}

var (
	sentHookMu sync.RWMutex
	sentHook   func(SentMessage)
)

// SetSentHook 注册"发送成功后"的回调; 传 nil 注销。
// 回调在发送方 goroutine 上同步执行, 必须自己做超时/失败隔离 —— 它不该拖慢或拖垮一次发送。
func SetSentHook(fn func(SentMessage)) {
	sentHookMu.Lock()
	sentHook = fn
	sentHookMu.Unlock()
}

func observeSent(msg *Message, id string) {
	sentHookMu.RLock()
	fn := sentHook
	sentHookMu.RUnlock()
	if fn == nil || msg == nil {
		return
	}
	fn(SentMessage{
		MsgID:   id,
		GroupID: msg.GroupId,
		UserID:  msg.UserId,
		Target:  msg.Target,
		Body:    bodyOf(msg),
	})
}

// bodyOf 取可读正文: 只有 markdown 与纯文本有, 媒体消息返回空串。
func bodyOf(msg *Message) string {
	switch m := msg.MarshalInterface.(type) {
	case *MarkdownMessage:
		return m.Markdown.Content
	case *TextMessage:
		return m.TextContent
	}
	return ""
}
