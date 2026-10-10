package logx

import (
	"strings"
	"sync"
)

// Entry 一条日志记录, ID 全局递增, 前端用于去重。
type Entry struct {
	ID    int64  `json:"id"`
	Time  int64  `json:"time"`
	Level string `json:"level"`
	Scope string `json:"scope"`
	Msg   string `json:"msg"`
}

// Filter 快照/接口侧过滤条件, 空字段表示不过滤。
type Filter struct {
	MinLevel string
	Scope    string
	Text     string
}

func (f Filter) matches(e Entry) bool {
	if f.MinLevel != "" {
		if min, err := parseLevel(f.MinLevel); err == nil {
			if lv, err := parseLevel(e.Level); err == nil && lv < min {
				return false
			}
		}
	}
	if f.Scope != "" && f.Scope != e.Scope {
		return false
	}
	if f.Text != "" && !strings.Contains(e.Msg, f.Text) {
		return false
	}
	return true
}

// Snapshot 返回最新 n 条(新->旧), 按 filter 过滤。
func Snapshot(n int, f Filter) []Entry {
	return std.ring.snapshot(n, f)
}

// Total 累计日志条数。
func Total() uint64 { return std.total.Load() }

// Errors 累计错误条数。
func Errors() uint64 { return std.errors.Load() }

// Scopes 当前已注册的来源列表。
func Scopes() []string {
	scopeMu.RLock()
	defer scopeMu.RUnlock()
	out := make([]string, 0, len(scopeLogs))
	for s := range scopeLogs {
		out = append(out, s)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Subscribe 订阅实时日志; 返回通道与注销函数。
func Subscribe() (<-chan Entry, func()) {
	return std.hub.subscribe()
}

type ring struct {
	mu  sync.Mutex
	buf []Entry
	cap int
	pos int
}

func newRing(cap int) *ring {
	if cap <= 0 {
		cap = DefaultRing
	}
	return &ring{cap: cap}
}

func (r *ring) resized(cap int) *ring {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := newRing(cap)
	if cap <= 0 {
		return next
	}
	n := len(r.buf)
	if n > cap {
		n = cap
	}
	start := (r.pos - n + r.cap) % r.cap
	for i := range n {
		next.push(r.buf[(start+i)%r.cap])
	}
	return next
}

func (r *ring) push(e Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) < r.cap {
		r.buf = append(r.buf, e)
	} else {
		r.buf[r.pos] = e
	}
	r.pos = (r.pos + 1) % r.cap
}

func (r *ring) snapshot(limit int, f Filter) []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(r.buf)
	if n == 0 {
		return []Entry{}
	}
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]Entry, 0, limit)
	idx := (r.pos - 1 + r.cap) % r.cap
	for i := 0; i < n && len(out) < limit; i++ {
		if e := r.buf[idx]; f.matches(e) {
			out = append(out, e)
		}
		idx = (idx - 1 + r.cap) % r.cap
	}
	return out
}

type hub struct {
	mu   sync.Mutex
	subs map[*sub]struct{}
}

type sub struct {
	ch chan Entry
}

func newHub() *hub {
	return &hub{subs: make(map[*sub]struct{})}
}

func (h *hub) subscribe() (<-chan Entry, func()) {
	s := &sub{ch: make(chan Entry, 128)}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	return s.ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[s]; ok {
			delete(h.subs, s)
			close(s.ch)
		}
		h.mu.Unlock()
	}
}

func (h *hub) broadcast(e Entry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		select {
		case s.ch <- e:
		default:
		}
	}
}
