package api

// MessageAPI 群/私聊/C2C 消息域。发送统一走 sendWithRetry, 附带审计等待与重试语义。

import (
	"fmt"

	"Plrx/lib/state"
)

type MessageAPI struct {
	api *BotAPI
}

// SendGroupMessage 发送群消息。
func (m *MessageAPI) SendGroupMessage(data []byte, groupId string) error {
	_, err := m.api.sendWithRetry(fmt.Sprintf("%v/v2/groups/%v/messages", m.api.ProxyAPI, groupId), data)
	if err == nil {
		state.IncSent()
	}
	return err
}

// SendPrivateMessage 发送私聊消息。
func (m *MessageAPI) SendPrivateMessage(data []byte, userId string) error {
	_, err := m.api.sendWithRetry(fmt.Sprintf("%v/v2/users/%v/messages", m.api.ProxyAPI, userId), data)
	if err == nil {
		state.IncSent()
	}
	return err
}

// RecallGroupMessage 撤回群消息。hideTip 为 true 时附 hidetip 参数, 群内不提示"撤回了一条消息"。
func (m *MessageAPI) RecallGroupMessage(groupOpenID, messageID string, hideTip bool) error {
	url := fmt.Sprintf("%v/v2/groups/%v/messages/%v", m.api.ProxyAPI, groupOpenID, messageID)
	if hideTip {
		url += "?hidetip=true"
	}
	_, err := m.api.do("DELETE", url, nil)
	return err
}

// RecallC2CMessage 撤回私聊消息。
func (m *MessageAPI) RecallC2CMessage(openID, messageID string, hideTip bool) error {
	url := fmt.Sprintf("%v/v2/users/%v/messages/%v", m.api.ProxyAPI, openID, messageID)
	if hideTip {
		url += "?hidetip=true"
	}
	_, err := m.api.do("DELETE", url, nil)
	return err
}
