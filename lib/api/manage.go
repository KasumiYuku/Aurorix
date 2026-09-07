package api

// ManageAPI 管理域: 按钮交互回执与入群审批。

import (
	"fmt"

	"github.com/KasumiYuku/Aurorix/lib/requests"
)

type ManageAPI struct {
	api *BotAPI
}

// InteracteCallback 回执按钮交互, 终止 QQ 端按钮 loading。
// code=0 表示正常处理完成; QQ 要求 3 秒内回执, 超时按钮会判定失败。
func (m *ManageAPI) InteracteCallback(eventId string) error {
	url := fmt.Sprintf("%v/interactions/%v", m.api.ProxyAPI, eventId)
	_, err := m.api.do("PUT", url, requests.JSON(map[string]int{"code": 0}))
	return err
}

// AcceptGroupJoinRequest 同意入群请求。
func (m *ManageAPI) AcceptGroupJoinRequest(requestId, groupId, userId string) error {
	type data struct {
		Op  string `json:"op"`
		Rid string `json:"join_request_id"`
	}
	url := fmt.Sprintf("%v/v2/groups/%v/approval_join_request/%v", m.api.ProxyAPI, groupId, userId)
	_, err := m.api.do("POST", url, requests.JSON(data{Op: "approve", Rid: requestId}))
	return err
}

// RejectGroupJoinRequest 拒绝入群请求。
func (m *ManageAPI) RejectGroupJoinRequest(requestId, groupId, userId, reason string) error {
	return m.approveGroupJoinRequest(requestId, groupId, userId, reason, false)
}

// RejectGroupJoinRequestAndAddToBlacklist 拒绝入群请求并拉黑。
func (m *ManageAPI) RejectGroupJoinRequestAndAddToBlacklist(requestId, groupId, userId, reason string) error {
	return m.approveGroupJoinRequest(requestId, groupId, userId, reason, true)
}

func (m *ManageAPI) approveGroupJoinRequest(requestId, groupId, userId, reason string, blacklist bool) error {
	type data struct {
		Op     string `json:"op"`
		Rid    string `json:"join_request_id"`
		Reason string `json:"reject_reason,omitempty"`
		A      bool   `json:"add_to_member_blacklist,omitempty"`
	}
	url := fmt.Sprintf("%v/v2/groups/%v/approval_join_request/%v", m.api.ProxyAPI, groupId, userId)
	_, err := m.api.do("POST", url, requests.JSON(data{
		Op: "reject", Rid: requestId, Reason: reason, A: blacklist,
	}))
	return err
}
