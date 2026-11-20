package gateway

import (
	"net/http"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/api"
)

// Endpoint 一次连接所需的地址与分片。
type Endpoint struct {
	URL    string
	Shards int
}

// Source 网关来源: 官方 /gateway/bot, 或配置写死的固定地址。
type Source interface {
	Endpoint() Endpoint
	Header() http.Header
	ReauthDelay() time.Duration
}

// SourceFor 选来源: fixedURL 为空走官方, 否则连它。
func SourceFor(api *api.BotAPI, fixedURL string) (Source, error) {
	if fixedURL != "" {
		return fixedSource{url: fixedURL, appID: api.AppID}, nil
	}
	info, err := api.GatewayBot()
	if err != nil {
		return nil, err
	}
	return officialSource{api: api, url: info.URL, shards: info.Shards}, nil
}

type officialSource struct {
	api    *api.BotAPI
	url    string
	shards int
}

func (s officialSource) Endpoint() Endpoint { return Endpoint{URL: s.url, Shards: s.shards} }

func (s officialSource) Header() http.Header { return nil }

// ReauthDelay 按会话启动配额决定: 有余额等一轮心跳, 配额耗尽等窗口重置。
func (s officialSource) ReauthDelay() time.Duration {
	info, err := s.api.GatewayBot()
	if err != nil {
		logger.Warnf("查询会话启动限额失败, 按默认退避重连: %v", err)
		return jitter(5 * time.Second)
	}
	if info.Limit.Remaining > 0 || info.Limit.ResetAfter <= 0 {
		logger.Warnf("会话失效, 剩余启动配额 %d, 稍后重新鉴权", info.Limit.Remaining)
		return jitter(3 * time.Second)
	}
	wait := time.Duration(info.Limit.ResetAfter) * time.Millisecond
	logger.Warnf("会话启动配额耗尽 (%d/%d), %.0fm 后重新鉴权",
		info.Limit.Remaining, info.Limit.Total, wait.Minutes())
	return wait
}

type fixedSource struct {
	url   string
	appID string
}

func (s fixedSource) Endpoint() Endpoint { return Endpoint{URL: s.url, Shards: 1} }

func (s fixedSource) Header() http.Header {
	return http.Header{"X-Bot-Appid": []string{s.appID}}
}

func (s fixedSource) ReauthDelay() time.Duration { return jitter(3 * time.Second) }
