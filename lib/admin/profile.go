package admin

import (
	"net/http"
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/api"
)

// profileStore 机器人档案缓存: 启动拉取一次, 管理台可手动刷新。
type profileStore struct {
	mu   sync.RWMutex
	api  *api.BotAPI
	prof *api.BotProfile
}

// NewProfileStore 构造机器人档案缓存。
func NewProfileStore(client *api.BotAPI) *profileStore {
	return &profileStore{api: client}
}

// Refresh 拉取最新档案; 失败保留旧值, 返回错误供调用方提示。
func (p *profileStore) Refresh() error {
	prof, err := p.api.Me.GetMe()
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.prof = prof
	p.mu.Unlock()
	return nil
}

// Get 返回当前档案, 未拉取到时为 nil。
func (p *profileStore) Get() *api.BotProfile {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.prof
}

// handleProfileGet 读取档案缓存。
func handleProfileGet(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Profile == nil {
			writeJSON(w, http.StatusOK, H{"profile": nil})
			return
		}
		writeJSON(w, http.StatusOK, H{"profile": deps.Profile.Get()})
	}
}

// handleProfileRefresh 强制刷新档案; 失败返回 502 并带原因。
func handleProfileRefresh(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Profile == nil {
			writeJSON(w, http.StatusServiceUnavailable, H{"error": "档案服务未启用"})
			return
		}
		if err := deps.Profile.Refresh(); err != nil {
			writeJSON(w, http.StatusBadGateway, H{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, H{"profile": deps.Profile.Get()})
	}
}
