package message

import (
	"sync"
)

type MsgRef struct {
	sent  uint8
	spent bool
	lock  sync.Mutex
}

// Count 取下一个被动回复序号; 满 5 条之后一律报 ReplyMessageReachLimit, 调用方据此换主动通道。
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
