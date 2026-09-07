// Package api QQ 开放平台业务 API 门面。
// 按域组织: Message/Manage/Stream/Files/Gateway, 共享凭证与请求基建。
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	errorx "github.com/KasumiYuku/Aurorix/lib/error"
	"github.com/KasumiYuku/Aurorix/lib/requests"
	"github.com/KasumiYuku/Aurorix/lib/token"
)

// BotAPI 平台客户端门面: 持有凭证、HTTP 基建与各域 API 实例。
type BotAPI struct {
	AppID               string
	AppSecret           string
	ProxyAPI            string
	Request             *requests.Client
	Assets              *assets.ImageHost
	GlobalMarkdown      bool
	MarkdownVerifyImage bool
	RetryWhen           []int
	UploadThreshold     int // 超过该字节数用分片上传
	TokenAPI            string

	Tokens *token.Store

	Message *MessageAPI
	Manage  *ManageAPI
	Stream  *StreamAPI
	Files   *FilesAPI
	Gateway *GatewayAPI
	Me      *MeAPI
}

// Init 构造门面; 各域实例持有同一凭证存储。
func Init(appID, appSecret, proxy string, req *requests.Client) *BotAPI {
	tokens := token.New(appID, appSecret)
	bot := &BotAPI{
		AppID: appID, AppSecret: appSecret, ProxyAPI: proxy,
		Request: req, Tokens: tokens, TokenAPI: tokens.TokenAPI,
	}
	bot.Message = &MessageAPI{api: bot}
	bot.Manage = &ManageAPI{api: bot}
	bot.Stream = &StreamAPI{api: bot}
	bot.Files = &FilesAPI{api: bot}
	bot.Gateway = &GatewayAPI{api: bot}
	bot.Me = &MeAPI{api: bot}
	return bot
}

// SetAssets 注入图床聚合器 (config 热更回调)。
func (c *BotAPI) SetAssets(h *assets.ImageHost) { c.Assets = h }

// SetMessageOptions 注入消息管道配置 (config 热更回调)。
func (c *BotAPI) SetMessageOptions(globalMarkdown, markdownVerifyImage bool, retryWhen []int, uploadThreshold int) {
	c.GlobalMarkdown = globalMarkdown
	c.MarkdownVerifyImage = markdownVerifyImage
	c.RetryWhen = retryWhen
	c.UploadThreshold = uploadThreshold
}

// AccessToken 供外部使用 (如 WebSocket 鉴权)。
func (c *BotAPI) AccessToken() (string, error) {
	return c.Tokens.AccessToken()
}

// InvalidateToken 清空缓存 token, 网关鉴权失败时强制重取。
func (c *BotAPI) InvalidateToken() { c.Tokens.Invalidate() }

// 以下转发方法保持旧调用点签名不变, 域实例为唯一实现方。

func (c *BotAPI) SendGroupMessage(data []byte, groupId string) error {
	return c.Message.SendGroupMessage(data, groupId)
}

func (c *BotAPI) SendPrivateMessage(data []byte, userId string) error {
	return c.Message.SendPrivateMessage(data, userId)
}

func (c *BotAPI) UploadImage(target constant.MessageOrigin, groupID, userID, filePath string) (string, error) {
	return c.Files.UploadImage(target, groupID, userID, filePath)
}

func (c *BotAPI) UploadMedia(target constant.MessageOrigin, groupID, userID string, up MediaUpload) (string, error) {
	return c.Files.UploadMedia(target, groupID, userID, up)
}

func (c *BotAPI) RecallGroupMessage(groupOpenID, messageID string, hideTip bool) error {
	return c.Message.RecallGroupMessage(groupOpenID, messageID, hideTip)
}

func (c *BotAPI) RecallC2CMessage(openID, messageID string, hideTip bool) error {
	return c.Message.RecallC2CMessage(openID, messageID, hideTip)
}

func (c *BotAPI) InteracteCallback(eventId string) error {
	return c.Manage.InteracteCallback(eventId)
}

func (c *BotAPI) AcceptGroupJoinRequest(requestId, groupId, userId string) error {
	return c.Manage.AcceptGroupJoinRequest(requestId, groupId, userId)
}

func (c *BotAPI) RejectGroupJoinRequest(requestId, groupId, userId, reason string) error {
	return c.Manage.RejectGroupJoinRequest(requestId, groupId, userId, reason)
}

func (c *BotAPI) RejectGroupJoinRequestAndAddToBlacklist(requestId, groupId, userId, reason string) error {
	return c.Manage.RejectGroupJoinRequestAndAddToBlacklist(requestId, groupId, userId, reason)
}

func (c *BotAPI) SendStreamMessage(userID string, msg StreamMessage) (*SendStreamResult, error) {
	return c.Stream.SendStreamMessage(userID, msg)
}

func (c *BotAPI) NewStreamSession(userID, eventID, msgID string) *StreamSession {
	return c.Stream.NewStreamSession(userID, eventID, msgID)
}

func (c *BotAPI) GatewayBot() (*GatewayBotInfo, error) {
	return c.Gateway.GatewayBot()
}

func (c *BotAPI) GatewayURL() (string, error) {
	return c.Gateway.GatewayURL()
}

// SendMessageResult 发送结果。
type SendMessageResult struct {
	ID        string `json:"id"`
	AuditID   string `json:"audit_id"`
	Timestamp string `json:"timestamp"`
}

// do 统一执行带鉴权的请求; 401 视为 token 失效, 作废旧 token 取新后重试一次。
func (c *BotAPI) do(method, url string, body requests.Body) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		header, err := c.generateHeader()
		if err != nil {
			return nil, err
		}
		raw, err := c.Request.DoBytes(method, url, body, header)
		if err != nil {
			var se *requests.StatusError
			if attempt == 0 && errors.As(err, &se) && se.Code == http.StatusUnauthorized {
				c.InvalidateToken()
				continue
			}
			return nil, err
		}
		return raw, nil
	}
	return nil, fmt.Errorf("unreachable")
}

// sendWithRetry 发送并处理业务错误码重试与审计等待。
func (c *BotAPI) sendWithRetry(endpoint string, data []byte) (*SendMessageResult, error) {
	var result SendMessageResult
	for attempt := 0; ; attempt++ {
		raw, err := c.do("POST", endpoint, requests.Bytes(data))
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			return &result, nil
		}
		if err := json.Unmarshal(raw, &result); err == nil && result.ID != "" {
			return &result, nil
		}
		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				MessageAudit *struct {
					AuditID string `json:"audit_id"`
				} `json:"message_audit"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &errResp); err == nil && errResp.Code != 0 {
			if errResp.Code == 304023 && errResp.Data.MessageAudit != nil {
				if err := waitAudit(errResp.Data.MessageAudit.AuditID); err != nil {
					return nil, err
				}
				return &result, nil
			}
			if errorx.IsRetryable(errResp.Code, c.RetryWhen) && attempt < 2 {
				time.Sleep(time.Duration(300*(attempt+1)) * time.Millisecond)
				continue
			}
			return nil, errorx.New(errResp.Code, errResp.Message)
		}
		return &result, nil
	}
}

func (c *BotAPI) generateHeader() (map[string]string, error) {
	token, err := c.Tokens.AccessToken()
	if err != nil {
		return map[string]string{}, err
	}
	return map[string]string{
		"Authorization": fmt.Sprintf("QQBot %v", token),
		"Content-Type":  "application/json",
	}, nil
}

// auditMu 审计等待注册表: audit_id -> 结果通道。
var (
	auditMu     sync.Mutex
	auditWaiter = make(map[string]chan auditResult)
)

type auditResult struct {
	approved  bool
	messageID string
}

// ResolveAudit 由 MESSAGE_AUDIT_PASS/REJECT 事件调用, resolve 等待者。
func ResolveAudit(auditID, messageID string, approved bool) {
	auditMu.Lock()
	ch, ok := auditWaiter[auditID]
	if ok {
		delete(auditWaiter, auditID)
	}
	auditMu.Unlock()
	if ok {
		ch <- auditResult{approved: approved, messageID: messageID}
	}
}
