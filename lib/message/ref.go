package message

import (
	"sync"
)

// 计数器
type MsgRef struct {
	// sent 已经发出去的被动回复条数(序号从 1 开始, 与上游 msg_seq 一致)。
	sent  uint8
	spent bool
	lock  sync.Mutex
}

// Count 取下一个被动回复序号; 满 5 条之后一律报 ReplyMessageReachLimit。
//
// 上游对同一条入站消息只收前 5 条被动回复(第 6 条起回 40034128)。本地先数一步是为了不去打那个
// 注定被拒的请求, 而**数满之后仍要有个信号** —— 调用方靠它换主动通道, 而不是把消息丢掉(见 SendWithID)。
func (ref *MsgRef) Count() (uint8, error) {
	ref.lock.Lock()
	defer ref.lock.Unlock()
	if ref.spent || ref.sent >= 5 {
		ref.spent = true
		return 0, &ReplyMessageReachLimit{}
	}
	ref.sent++
	return ref.sent, nil
}

// Spent 被动预算是否已经用光。
func (ref *MsgRef) Spent() bool {
	ref.lock.Lock()
	defer ref.lock.Unlock()
	return ref.spent
}
