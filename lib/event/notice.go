package event

import (
	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// MemberEvent 群成员变动事件 (加入/退出)。
type MemberEvent struct {
	eventType  constant.EventType
	GroupID    string
	UserID     string
	OperatorID string
	Timestamp  int64
	Added      bool
}

func (e *MemberEvent) Type() constant.EventType { return e.eventType }

// RobotEvent 机器人在群中的状态变动 (被加入/被移出)。
type RobotEvent struct {
	eventType  constant.EventType
	GroupID    string
	OperatorID string
	Timestamp  int64
	Added      bool
}

func (e *RobotEvent) Type() constant.EventType { return e.eventType }

// ReceiveEvent 消息接收权限开关事件 (群/私聊开启或关闭)。
type ReceiveEvent struct {
	eventType  constant.EventType
	GroupID    string // 私聊场景为空
	UserID     string // 群场景为操作者
	OperatorID string
	Timestamp  int64
	Enabled    bool
}

func (e *ReceiveEvent) Type() constant.EventType { return e.eventType }
