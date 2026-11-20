package notice

import (
	"errors"
	"sync"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/api"
)

const gateTTL = 30 * time.Minute

type gateEntry struct {
	allow   bool
	fetched time.Time
}

type gate struct {
	mu    sync.Mutex
	cache map[string]gateEntry
	ttl   time.Duration
	now   func() time.Time
	fetch func(*api.BotAPI, string) (bool, error)
}

func newGate() *gate {
	return &gate{cache: map[string]gateEntry{}, ttl: gateTTL, now: time.Now, fetch: fetchBotState}
}

func fetchBotState(qapi *api.BotAPI, groupID string) (bool, error) {
	if qapi == nil || qapi.Group == nil {
		return false, errors.New("缺少 API 客户端")
	}
	state, err := qapi.Group.GetBotState(groupID)
	if err != nil {
		return false, err
	}
	return state.AllowProactiveMsg, nil
}

func (g *gate) allow(qapi *api.BotAPI, groupID string) (bool, bool) {
	if g == nil || groupID == "" {
		return false, false
	}
	g.mu.Lock()
	entry, ok := g.cache[groupID]
	g.mu.Unlock()
	if ok && g.now().Sub(entry.fetched) < g.ttl {
		return entry.allow, true
	}
	if g.fetch == nil {
		return false, false
	}
	allow, err := g.fetch(qapi, groupID)
	if err != nil {
		return false, false
	}
	g.set(groupID, allow)
	return allow, true
}

func (g *gate) set(groupID string, allow bool) {
	if g == nil || groupID == "" {
		return
	}
	g.mu.Lock()
	g.cache[groupID] = gateEntry{allow: allow, fetched: g.now()}
	g.mu.Unlock()
}
