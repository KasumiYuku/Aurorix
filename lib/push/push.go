// Package push 框架内置主动推送服务: 管理推送开关/密钥的指令 + 外部 HTTP 推送端点。
package push

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/images"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/storage"
	"net/http"
	"strings"
	"sync"
	"time"
)

var logger = logx.New("push")

const (
	pluginID = "push"

	storeKeyPushKey = "push_key"
	storeKeyEnabled = "enabled"
)

const (
	rateLimitWindow = time.Minute
	rateLimitMax    = 20
)

var (
	clientMu sync.RWMutex
	client   *api.BotAPI

	rateMu    sync.Mutex
	rateCount int
	rateStart time.Time
)

func pushRateLimited() bool {
	rateMu.Lock()
	defer rateMu.Unlock()
	now := time.Now()
	if rateStart.IsZero() || now.Sub(rateStart) > rateLimitWindow {
		rateStart = now
		rateCount = 0
	}
	rateCount++
	return rateCount > rateLimitMax
}

func SetClient(c *api.BotAPI) {
	clientMu.Lock()
	defer clientMu.Unlock()
	client = c
}

type pushKeyArgs struct {
	Key string `kong:"arg,name='key',help='推送密钥'"`
}

// Init 注入 BotAPI 并注册推送管理指令, 启动时调用一次。
func Init(bot *api.BotAPI) {
	SetClient(bot)
	plugin.Register(&plugin.Plugin{
		Id: pluginID,
		Commands: []*plugin.Command{
			{
				Prefix:   "enablepush",
				Role:     constant.RoleOwner,
				Describe: "允许机器人向当前会话主动推送",
				Handle:   enablePush,
			},
			{
				Prefix:   "disablepush",
				Role:     constant.RoleOwner,
				Describe: "关闭机器人对当前会话的主动推送",
				Handle:   disablePush,
			},
			{
				Prefix:   "pushkey",
				Role:     constant.RoleOwner,
				Describe: "设置主动推送的密钥",
				Args:     &pushKeyArgs{},
				Handle:   setPushKey,
			},
			{
				Prefix:   "pushstatus",
				Role:     constant.RoleOwner,
				Describe: "查看当前会话的主动推送状态",
				Handle:   pushStatus,
			},
		},
	})
}

func resolveTarget(ctx *context.MessageContext) (constant.Scene, string, error) {
	t := ctx.Target
	if t.Empty() {
		return "", "", errors.New("当前会话没有可推送的目标")
	}
	return t.Scene, t.ID, nil
}

func enabledKey(scope constant.Scene, openid string) string {
	return storeKeyEnabled + ":" + string(scope) + ":" + openid
}

func enablePush(ctx *context.MessageContext) error {
	scope, openid, err := resolveTarget(ctx)
	if err != nil {
		return ctx.Text(err.Error()).Send()
	}
	if err := storage.Global().Set(enabledKey(scope, openid), true); err != nil {
		return ctx.Text("启用失败: " + err.Error()).Send()
	}
	return ctx.Text(fmt.Sprintf("已允许机器人主动向本%s推送", scope.Label())).Send()
}

func disablePush(ctx *context.MessageContext) error {
	scope, openid, err := resolveTarget(ctx)
	if err != nil {
		return ctx.Text(err.Error()).Send()
	}
	if err := storage.Global().Delete(enabledKey(scope, openid)); err != nil {
		return ctx.Text("关闭失败: " + err.Error()).Send()
	}
	return ctx.Text(fmt.Sprintf("已关闭本%s的主动推送", scope.Label())).Send()
}

func pushStatus(ctx *context.MessageContext) error {
	scope, openid, err := resolveTarget(ctx)
	if err != nil {
		return ctx.Text(err.Error()).Send()
	}
	enabled, err := storage.Global().Has(enabledKey(scope, openid))
	if err != nil {
		return ctx.Text("查询失败: " + err.Error()).Send()
	}
	state := "未启用"
	if enabled {
		state = "已启用"
	}
	return ctx.Text(fmt.Sprintf("本%s的主动推送状态: %s", scope.Label(), state)).Send()
}

func setPushKey(ctx *context.MessageContext) error {
	args := ctx.Parsed.(*pushKeyArgs)
	key := args.Key
	if key == "" {
		return ctx.Text("密钥不能为空").Send()
	}
	err := storage.Global().Set(ctx.Target.ID+storeKeyPushKey, key)
	if err != nil {
		return ctx.Text("设置失败: " + err.Error()).Send()
	}
	return ctx.Text("已更新主动推送密钥").Send()
}

type pushRequest struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type pushResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func HTTPHandle(w http.ResponseWriter, r *http.Request) {
	scene := constant.Scene(strings.ToLower(r.PathValue("scope")))
	openid := r.PathValue("openid")
	if !scene.Valid() {
		writeJSON(w, http.StatusBadRequest, pushResponse{Error: "scope 必须是 group / c2c / channel / dm"})
		return
	}
	if openid == "" {
		writeJSON(w, http.StatusBadRequest, pushResponse{Error: "openid is required"})
		return
	}

	var req pushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, pushResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	key := r.Header.Get("X-Push-Key")
	if key == "" {
		key = req.Key
	}
	if !verifyPushKey(openid, key) {
		if pushRateLimited() {
			writeJSON(w, http.StatusTooManyRequests, pushResponse{Error: "too many failed attempts"})
			return
		}
		writeJSON(w, http.StatusUnauthorized, pushResponse{Error: "invalid or missing push key"})
		return
	}

	enabled, err := storage.Global().Has(enabledKey(scene, openid))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, pushResponse{Error: "check enabled: " + err.Error()})
		return
	}
	if !enabled {
		writeJSON(w, http.StatusForbidden, pushResponse{Error: "target has not enabled push"})
		return
	}

	clientMu.RLock()
	api := client
	clientMu.RUnlock()
	if api == nil {
		writeJSON(w, http.StatusInternalServerError, pushResponse{Error: "qq api client not ready"})
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeJSON(w, http.StatusBadRequest, pushResponse{Error: "content is required"})
		return
	}
	content = images.ClampMarkdown(content, api.ImageBox)
	payload, err := buildPayload(strings.ToLower(req.Type), content)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, pushResponse{Error: err.Error()})
		return
	}

	logger.Infof("external push to %s/%s (%d bytes)", scene, openid, len(payload))
	if _, sendErr := api.Session(constant.Target{Scene: scene, ID: openid}).Send(payload); sendErr != nil {
		writeJSON(w, http.StatusBadGateway, pushResponse{Error: sendErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, pushResponse{OK: true})
}

func verifyPushKey(openid string, provided string) bool {
	if provided == "" {
		return false
	}
	var stored string
	found, err := storage.Global().Get(openid+storeKeyPushKey, &stored)
	if err != nil || !found {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(provided)) == 1
}

func buildPayload(kind, content string) ([]byte, error) {
	switch kind {
	case "", "text":
		return json.Marshal(map[string]any{
			"msg_type": 0,
			"content":  content,
		})
	case "markdown", "md":
		return json.Marshal(map[string]any{
			"msg_type": 2,
			"markdown": map[string]any{
				"content": content,
			},
		})
	default:
		return nil, fmt.Errorf("unsupported message type: %v", kind)
	}
}
