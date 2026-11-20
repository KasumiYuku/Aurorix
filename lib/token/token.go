// Package token 机器人凭证管理: 单飞刷新 access token，网络调用不持锁，避免全进程排队等它。
package token

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/requests"
)

const (
	defaultTokenAPI = "https://bots.qq.com/app/getAppAccessToken"
	failCooldown    = 3 * time.Second
)

// Store 凭证与 token 缓存。
type Store struct {
	AppID     string
	AppSecret string
	TokenAPI  string

	req *requests.Client

	lock        sync.RWMutex
	accessToken string
	expireAt    time.Time
	refreshing  chan struct{}
	lastErr     error
	failedAt    time.Time
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

// AccessToken 返回当前有效 token; 缺失或临期时由第一个调用者去刷, 其余等它, 不各自压一个锁。
func (s *Store) AccessToken() (string, error) {
	for {
		s.lock.RLock()
		if s.accessToken != "" && time.Now().Before(s.expireAt) {
			token := s.accessToken
			s.lock.RUnlock()
			return token, nil
		}
		waiting, lastErr, failedAt := s.refreshing, s.lastErr, s.failedAt
		s.lock.RUnlock()

		if waiting != nil {
			<-waiting
			continue
		}
		if lastErr != nil && time.Since(failedAt) < failCooldown {
			return "", lastErr
		}

		s.lock.Lock()
		if s.refreshing != nil {
			s.lock.Unlock()
			continue
		}
		waiting = make(chan struct{})
		s.refreshing = waiting
		s.lock.Unlock()

		token, ttl, err := s.fetch()

		s.lock.Lock()
		if err == nil {
			s.accessToken = token
			s.expireAt = time.Now().Add(ttl)
			s.lastErr, s.failedAt = nil, time.Time{}
		} else {
			s.lastErr, s.failedAt = err, time.Now()
		}
		s.refreshing = nil
		close(waiting)
		s.lock.Unlock()

		if err != nil {
			return "", err
		}
		return token, nil
	}
}

func (s *Store) fetch() (string, time.Duration, error) {
	body := fmt.Appendf(nil, `{"appId":"%s", "clientSecret":"%s"}`, s.AppID, s.AppSecret)
	var data struct {
		AccessToken string `json:"access_token"`
		ExpireTime  string `json:"expires_in"`
	}
	if err := s.req.Post(s.TokenAPI, body, &data, nil); err != nil {
		return "", 0, err
	}
	if data.AccessToken == "" {
		return "", 0, fmt.Errorf("access token 响应为空")
	}
	expiresIn, err := strconv.Atoi(data.ExpireTime)
	if err != nil {
		return "", 0, fmt.Errorf("expires_in 格式错误: %v", data.ExpireTime)
	}
	return data.AccessToken, time.Duration(expiresIn)*time.Second - 50*time.Second, nil
}

// Invalidate 清空缓存, 下次调用强制刷新。网关鉴权失败时使用。
func (s *Store) Invalidate() {
	s.lock.Lock()
	s.accessToken = ""
	s.expireAt = time.Time{}
	s.lastErr, s.failedAt = nil, time.Time{}
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
