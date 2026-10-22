package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	botcontext "github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/inbox"
)

func registerInboxRoutes(api *http.ServeMux, deps Deps) {
	api.HandleFunc("GET /admin/api/inbox/conversations", func(w http.ResponseWriter, r *http.Request) {
		list, err := deps.Inbox.Conversations()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, H{"error": "读取会话失败"})
			return
		}
		writeJSON(w, http.StatusOK, H{"conversations": list})
	})

	api.HandleFunc("GET /admin/api/inbox/messages", func(w http.ResponseWriter, r *http.Request) {
		convID := strings.TrimSpace(r.URL.Query().Get("conv"))
		if convID == "" {
			writeJSON(w, http.StatusBadRequest, H{"error": "缺少会话 id"})
			return
		}
		before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
		limit := inboxLimit(r.URL.Query().Get("limit"))
		list, err := deps.Inbox.Messages(convID, before, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, H{"error": "读取消息失败"})
			return
		}
		writeJSON(w, http.StatusOK, H{"messages": list, "has_more": len(list) == limit})
	})

	api.HandleFunc("POST /admin/api/inbox/messages", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Conv string `json:"conv"`
			Text string `json:"text"`
		}
		if err := readJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
			return
		}
		input.Conv, input.Text = strings.TrimSpace(input.Conv), strings.TrimSpace(input.Text)
		if input.Conv == "" || input.Text == "" {
			writeJSON(w, http.StatusBadRequest, H{"error": "会话和内容都不能为空"})
			return
		}
		conv, ok := deps.Inbox.Lookup(input.Conv)
		if !ok {
			writeJSON(w, http.StatusNotFound, H{"error": "会话不存在"})
			return
		}
		if deps.Client == nil {
			writeJSON(w, http.StatusServiceUnavailable, H{"error": "机器人未连接"})
			return
		}
		if err := sendInbox(deps, conv, input.Text); err != nil {
			writeJSON(w, http.StatusBadGateway, H{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, H{"ok": true})
	})
}

// sendInbox 走主动通道: 管理台发消息时没有可用的 event_id, 强行走被动只会被平台拒。
// 额度不够时把官方那句话原样抛回去, 不包装成"发送失败"这种没法排查的措辞。
func sendInbox(deps Deps, conv inbox.Conversation, text string) error {
	manager := &botcontext.MessageManager{Qapi: deps.Client, NoFooter: true}
	if conv.Kind == inbox.KindGroup {
		manager.GroupId, manager.Target = conv.ID, constant.GroupMessage
	} else {
		manager.UserId, manager.Target = conv.ID, constant.PrivateMessage
	}
	return manager.Msg().Markdown(text).Initiative().NoFooter().Send()
}

func inboxLimit(raw string) int {
	limit, _ := strconv.Atoi(raw)
	if limit <= 0 || limit > inbox.MaxPageSize {
		return inbox.DefaultPageSize
	}
	return limit
}
