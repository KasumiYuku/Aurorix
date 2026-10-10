package event

import (
	"encoding/json"

	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// UnknownEvent 未注册或解析失败的事件, 原样透传供订阅者观察。
type UnknownEvent struct {
	RawType   string
	EventID   string
	MessageID string
	GroupID   string
	UserID    string
	RawBody   json.RawMessage
}

func (e *UnknownEvent) Type() constant.EventType { return constant.EventType(e.RawType) }
