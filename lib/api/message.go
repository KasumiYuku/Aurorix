package api

import (
	"fmt"

	"github.com/KasumiYuku/Aurorix/lib/state"
	"github.com/KasumiYuku/Aurorix/lib/stats"
)

type MessageAPI struct {
	api *BotAPI
}

// SendGroupMessage 发送群消息。
func (m *MessageAPI) SendGroupMessage(data []byte, groupId string) error {
	_, err := m.SendGroupMessageID(data, groupId)
	return err
}

// SendGroupMessageID 发送群消息并返回平台消息 ID, 供撤回等后续操作使用。
func (m *MessageAPI) SendGroupMessageID(data []byte, groupId string) (string, error) {
	return m.sendID(fmt.Sprintf("%v/v2/groups/%v/messages", m.api.ProxyAPI, groupId), data)
}

// SendPrivateMessage 发送私聊消息。
func (m *MessageAPI) SendPrivateMessage(data []byte, userId string) error {
	_, err := m.SendPrivateMessageID(data, userId)
	return err
}

// SendPrivateMessageID 发送私聊消息并返回平台消息 ID。
func (m *MessageAPI) SendPrivateMessageID(data []byte, userId string) (string, error) {
	return m.sendID(fmt.Sprintf("%v/v2/users/%v/messages", m.api.ProxyAPI, userId), data)
}

func (m *MessageAPI) sendID(endpoint string, data []byte) (string, error) {
	result, err := m.api.sendWithRetry(endpoint, data)
	if err != nil {
		return "", err
	}
	state.IncSent()
	stats.Sent()
	if result == nil {
		return "", nil
	}
	return result.ID, nil
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
