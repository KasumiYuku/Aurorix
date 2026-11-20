// Package api QQ 开放平台业务 API 门面。
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	errorx "github.com/KasumiYuku/Aurorix/lib/error"
	"github.com/KasumiYuku/Aurorix/lib/images"
	"github.com/KasumiYuku/Aurorix/lib/requests"
	"github.com/KasumiYuku/Aurorix/lib/token"
)

// BotAPI 平台客户端门面: 持有凭证、HTTP 基建与各域 API 实例。
type BotAPI struct {
	AppID           string
	AppSecret       string
	ProxyAPI        string
	Request         *requests.Client
	Assets          *assets.ImageHost
	GlobalMarkdown  bool
	RetryWhen       []int
	UploadThreshold int
	ImageBox        images.Box

	Tokens *token.Store

	botUserID atomic.Value

	Manage  *ManageAPI
	Group   *GroupAPI
	Channel *ChannelAPI
	Stream  *StreamAPI
	Gateway *GatewayAPI
	Me      *MeAPI
	Panel   *PanelAPI
}

func Init(appID, appSecret, proxy string, req *requests.Client) *BotAPI {
	tokens := token.New(appID, appSecret)
	bot := &BotAPI{
		AppID: appID, AppSecret: appSecret, ProxyAPI: proxy,
		Request: req, Tokens: tokens,
	}
	bot.Manage = &ManageAPI{api: bot}
	bot.Group = &GroupAPI{api: bot}
	bot.Channel = &ChannelAPI{api: bot}
	bot.Stream = &StreamAPI{api: bot}
	bot.Gateway = &GatewayAPI{api: bot}
	bot.Me = &MeAPI{api: bot}
	bot.Panel = &PanelAPI{api: bot}
	return bot
}

// SetAssets 注入图床聚合器 (config 热更回调)。
func (c *BotAPI) SetAssets(h *assets.ImageHost) { c.Assets = h }

// Threshold 是分片直传的字节阈值; 未配置时给默认值。媒体层也拿它判断"值不值得先换直链"。
func (c *BotAPI) Threshold() int {
	if c.UploadThreshold > 0 {
		return c.UploadThreshold
	}
	return constant.DefaultUploadThreshold
}

// SetMessageOptions 注入消息管道配置 (config 热更回调)。
func (c *BotAPI) SetMessageOptions(globalMarkdown bool, retryWhen []int, uploadThreshold int) {
	c.GlobalMarkdown = globalMarkdown
	c.RetryWhen = retryWhen
	c.UploadThreshold = uploadThreshold
}

// SetImageBox 注入 markdown 图片显示上限 (config 热更回调)。
func (c *BotAPI) SetImageBox(box images.Box) { c.ImageBox = box }

// AccessToken 供外部使用 (如 WebSocket 鉴权)。
func (c *BotAPI) AccessToken() (string, error) {
	return c.Tokens.AccessToken()
}

// SetBotUserID 记下机器人自己的用户 ID(频道里艾特/身份比对要用)。
func (c *BotAPI) SetBotUserID(id string) {
	if id != "" {
		c.botUserID.Store(id)
	}
}

// BotUserID 机器人自己的用户 ID; 还没取到档案时是空串。
func (c *BotAPI) BotUserID() string {
	id, _ := c.botUserID.Load().(string)
	return id
}

// InvalidateToken 清空缓存 token, 网关鉴权失败时强制重取。
func (c *BotAPI) InvalidateToken() { c.Tokens.Invalidate() }

// UploadOfficialImage 官方图床上传：把图片传进平台自己的富媒体存储，返回公网直链。
func (c *BotAPI) UploadOfficialImage(in assets.ProviderInput) (assets.ResolvedImage, error) {
	if in.Target.Scene.Guild() {
		return assets.ResolvedImage{}, &Unsupported{Scene: in.Target.Scene, What: "官方图床"}
	}
	if in.Target.Empty() {
		return assets.ResolvedImage{}, fmt.Errorf("官方图床上传需要会话目标")
	}
	if len(in.Buffer) == 0 {
		return assets.ResolvedImage{}, fmt.Errorf("官方图床需要图片字节（只给公网 URL 拿不到 raw_url）")
	}
	mimeType := in.MimeType
	if mimeType == "" {
		mimeType = images.ProbeMime(in.Buffer)
	}
	name := in.Filename
	if name == "" {
		if ext := images.ExtFromMime(mimeType); ext != "" {
			name = "image." + ext
		} else {
			name = "image"
		}
	}
	res, err := c.Session(in.Target).UploadEx(MediaUpload{
		FileType: constant.MediaImage, Data: in.Buffer, Filename: name, MimeType: mimeType, ForceChunked: true,
	})
	if err != nil {
		return assets.ResolvedImage{}, err
	}
	if res.RawURL == "" {
		return assets.ResolvedImage{}, fmt.Errorf("官方上传未返回 raw_url")
	}
	return assets.ResolvedImage{URL: res.RawURL}, nil
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

// SendMessageResult 发送结果。
type SendMessageResult struct {
	ID        string `json:"id"`
	AuditID   string `json:"audit_id"`
	Timestamp string `json:"timestamp"`
}

func (c *BotAPI) do(method, url string, body requests.Body) ([]byte, error) {
	return c.doWithin(method, url, body, 0)
}

func (c *BotAPI) doWithin(method, url string, body requests.Body, timeout time.Duration) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		header, err := c.generateHeader()
		if err != nil {
			return nil, err
		}
		var raw []byte
		var err2 error
		if timeout > 0 {
			raw, err2 = c.Request.DoBytesTimeout(method, url, body, header, timeout)
		} else {
			raw, err2 = c.Request.DoBytes(method, url, body, header)
		}
		if err2 != nil {
			var se *requests.StatusError
			if attempt == 0 && errors.As(err2, &se) && se.Code == http.StatusUnauthorized {
				c.InvalidateToken()
				continue
			}
			return nil, err2
		}
		return raw, nil
	}
	return nil, fmt.Errorf("unreachable")
}

const (
	codeEventIDNotReady = 40034025
	eventIDRetryDelay   = 200 * time.Millisecond
	eventIDRetryLimit   = 3
)

func (c *BotAPI) sendWithRetry(endpoint string, data []byte) (*SendMessageResult, error) {
	var result SendMessageResult
	for attempt := 0; ; attempt++ {
		raw, err := c.do("POST", endpoint, requests.Bytes(data))
		if err != nil {
			qe := asQQError(err)
			if qe == nil {
				return nil, err
			}
			if qe.Code == codeEventIDNotReady && attempt < eventIDRetryLimit {
				time.Sleep(eventIDRetryDelay)
				continue
			}
			if errorx.IsRetryable(qe.Code, c.RetryWhen) && attempt < 2 {
				time.Sleep(time.Duration(300*(attempt+1)) * time.Millisecond)
				continue
			}
			return nil, qe
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
