package event

import (
	"encoding/json"

	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// UnknownEvent 未注册或解析失败的事件, 原样透传供订阅者观察。
type UnknownEvent struct {
	RawType string          // 平台事件名, 未必在 constant 表中
	RawBody json.RawMessage // 事件载荷
}

func (e *UnknownEvent) Type() constant.EventType { return constant.EventType(e.RawType) }
