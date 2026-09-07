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
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/storage"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var logger = logx.New("push")

const (
	pluginID = "push"

	storeKeyPushKey = "push_key"
	storeKeyEnabled = "enabled"
)

// 暴力枚举防护：全局滑动窗口限速，防止无 key 时无限试错。
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

// pushRateLimited 判断当前是否触发限流；通过则计一次并返回 false。
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

func resolveTarget(ctx *context.MessageContext) (string, string, error) {
	switch ctx.Target {
	case constant.GroupMessage:
		if ctx.GroupId == "" {
			return "", "", errors.New("无法获取当前群 OpenID")
		}
		return "group", ctx.GroupId, nil
	case constant.PrivateMessage:
		if ctx.UserId == "" {
			return "", "", errors.New("无法获取当前用户 OpenID")
		}
		return "private", ctx.UserId, nil
	default:
		return "", "", errors.New("不支持的会话类型")
	}
}

func enabledKey(scope, openid string) string {
	return storeKeyEnabled + ":" + scope + ":" + openid
}

func enablePush(ctx *context.MessageContext) error {
	scope, openid, err := resolveTarget(ctx)
	if err != nil {
		return ctx.Text(err.Error()).Send()
	}
	if err := storage.Global().Set(enabledKey(scope, openid), true); err != nil {
		return ctx.Text("启用失败: " + err.Error()).Send()
	}
	return ctx.Text(fmt.Sprintf("已允许机器人主动向本 %s 推送", scope)).Send()
}

func disablePush(ctx *context.MessageContext) error {
	scope, openid, err := resolveTarget(ctx)
	if err != nil {
		return ctx.Text(err.Error()).Send()
	}
	if err := storage.Global().Delete(enabledKey(scope, openid)); err != nil {
		return ctx.Text("关闭失败: " + err.Error()).Send()
	}
	return ctx.Text(fmt.Sprintf("已关闭本 %s 的主动推送", scope)).Send()
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
	return ctx.Text(fmt.Sprintf("本 %s 的主动推送状态: %s", scope, state)).Send()
}

func setPushKey(ctx *context.MessageContext) error {
	args := ctx.Parsed.(*pushKeyArgs)
	key := args.Key
	if key == "" {
		return ctx.Text("密钥不能为空").Send()
	}
	var err error
	if ctx.GroupId != "" {
		err = storage.Global().Set(ctx.GroupId+storeKeyPushKey, key)
	} else {
		err = storage.Global().Set(ctx.UserId+storeKeyPushKey, key)
	}
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

func HTTPHandle(c *gin.Context) {
	scope := c.Param("scope")
	openid := c.Param("openid")
	if scope != "group" && scope != "private" {
		c.JSON(http.StatusBadRequest, pushResponse{Error: "scope must be 'group' or 'private'"})
		return
	}
	if openid == "" {
		c.JSON(http.StatusBadRequest, pushResponse{Error: "openid is required"})
		return
	}

	var req pushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, pushResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	key := c.GetHeader("X-Push-Key")
	if key == "" {
		key = req.Key
	}
	if !verifyPushKey(openid, key) {
		// 触发限速时统一 429，拒绝后续暴力尝试
		if pushRateLimited() {
			c.JSON(http.StatusTooManyRequests, pushResponse{Error: "too many failed attempts"})
			return
		}
		c.JSON(http.StatusUnauthorized, pushResponse{Error: "invalid or missing push key"})
		return
	}

	enabled, err := storage.Global().Has(enabledKey(scope, openid))
	if err != nil {
		c.JSON(http.StatusInternalServerError, pushResponse{Error: "check enabled: " + err.Error()})
		return
	}
	if !enabled {
		c.JSON(http.StatusForbidden, pushResponse{Error: "target has not enabled push"})
		return
	}

	clientMu.RLock()
	api := client
	clientMu.RUnlock()
	if api == nil {
		c.JSON(http.StatusInternalServerError, pushResponse{Error: "qq api client not ready"})
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		c.JSON(http.StatusBadRequest, pushResponse{Error: "content is required"})
		return
	}
	payload, err := buildPayload(strings.ToLower(req.Type), content)
	if err != nil {
		c.JSON(http.StatusBadRequest, pushResponse{Error: err.Error()})
		return
	}

	logger.Infof("external push to %s/%s (%d bytes)", scope, openid, len(payload))
	var sendErr error
	if scope == "group" {
		sendErr = api.SendGroupMessage(payload, openid)
	} else {
		sendErr = api.SendPrivateMessage(payload, openid)
	}
	if sendErr != nil {
		c.JSON(http.StatusBadGateway, pushResponse{Error: sendErr.Error()})
		return
	}
	c.JSON(http.StatusOK, pushResponse{OK: true})
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
	// 常量时间比较，防 key 长度/内容侧信道被逐字节枚举
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
