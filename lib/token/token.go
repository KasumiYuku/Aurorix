// Package token 机器人凭证管理: 双检缓存刷新 access token, 独立请求通道避免被业务流量拖累。
package token

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"Plrx/lib/requests"
)

const defaultTokenAPI = "https://bots.qq.com/app/getAppAccessToken"

// Store 凭证与 token 缓存。
type Store struct {
	AppID     string
	AppSecret string
	TokenAPI  string

	req *requests.Client

	accessToken string
	expireAt    time.Time
	lock        sync.RWMutex
}

// New 构造凭证存储, TokenAPI 留空用官方默认端点。
func New(appID, appSecret string) *Store {
	return &Store{
		AppID:     appID,
		AppSecret: appSecret,
		TokenAPI:  defaultTokenAPI,
		req:       requests.Init(20),
	}
}

// AccessToken 返回当前有效 token; 缺失或临期时刷新, 留 50 秒余量。
func (s *Store) AccessToken() (string, error) {
	s.lock.RLock()
	if s.accessToken != "" && time.Now().Before(s.expireAt) {
		token := s.accessToken
		s.lock.RUnlock()
		return token, nil
	}
	s.lock.RUnlock()

	s.lock.Lock()
	defer s.lock.Unlock()
	if s.accessToken != "" && time.Now().Before(s.expireAt) {
		return s.accessToken, nil
	}

	body := fmt.Appendf(nil, `{"appId":"%s", "clientSecret":"%s"}`, s.AppID, s.AppSecret)
	type tokenData struct {
		AccessToken string `json:"access_token"`
		ExpireTime  string `json:"expires_in"`
	}
	var data tokenData
	if err := s.req.Post(s.TokenAPI, body, &data, nil); err != nil {
		return "", err
	}
	if data.AccessToken == "" {
		return "", fmt.Errorf("access token 响应为空")
	}
	expiresIn, err := strconv.Atoi(data.ExpireTime)
	if err != nil {
		return "", fmt.Errorf("expires_in 格式错误: %v", data.ExpireTime)
	}
	s.accessToken = data.AccessToken
	s.expireAt = time.Now().Add(time.Duration(expiresIn)*time.Second - 50*time.Second)
	return data.AccessToken, nil
}

// Invalidate 清空缓存, 下次调用强制刷新。网关鉴权失败时使用。
func (s *Store) Invalidate() {
	s.lock.Lock()
	s.accessToken = ""
	s.expireAt = time.Time{}
	s.lock.Unlock()
}

// Validate 校验凭证非空。
func (s *Store) Validate() error {
	if s.AppID == "" {
		return fmt.Errorf("AppID 为空")
	}
	if s.AppSecret == "" {
		return fmt.Errorf("AppSecret 为空")
	}
	return nil
}

// SafeDisplay 脱敏展示, 供日志与状态页使用。
func (s *Store) SafeDisplay() string {
	secret := s.AppSecret
	if len(secret) > 8 {
		secret = secret[:4] + "****" + secret[len(secret)-4:]
	} else if secret != "" {
		secret = "****"
	}
	return fmt.Sprintf("Token{app_id: %s, secret: %s}", s.AppID, secret)
}
