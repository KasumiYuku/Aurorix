package logx

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// Entry 一条日志记录, ID 全局递增供前端去重; Fields 是除 trace 以外的结构化属性。
type Entry struct {
	ID     int64             `json:"id"`
	Time   int64             `json:"time"`
	Level  string            `json:"level"`
	Scope  string            `json:"scope"`
	Msg    string            `json:"msg"`
	Group  string            `json:"group,omitempty"`
	User   string            `json:"user,omitempty"`
	Trace  string            `json:"trace,omitempty"`
	Fields map[string]string `json:"fields,omitempty"`
}

// Filter 快照/接口侧过滤条件, 空字段表示不过滤。
type Filter struct {
	MinLevel string
	Scope    string
	Text     string
	Group    string
	User     string
	Trace    string
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
	if f.Group != "" && f.Group != e.Group {
		return false
	}
	if f.User != "" && f.User != e.User {
		return false
	}
	if f.Trace != "" && f.Trace != e.Trace {
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

// 缓冲同时受条数与字节数约束: 高频长正文不该因为"只数条数"就把内存顶上去。
const (
	DefaultRingBytes = 8 << 20
	maxEntryBytes    = 8 << 10
)

type ring struct {
	mu       sync.Mutex
	buf      []Entry
	cap      int
	bytesCap int
	bytes    int
}

func newRing(cap int) *ring {
	if cap <= 0 {
		cap = DefaultRing
	}
	return &ring{cap: cap, bytesCap: DefaultRingBytes}
}

func (r *ring) resized(cap int) *ring {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := newRing(cap)
	if cap <= 0 {
		return next
	}
	start := len(r.buf) - cap
	if start < 0 {
		start = 0
	}
	for _, e := range r.buf[start:] {
		next.push(e)
	}
	return next
}

func (r *ring) push(e Entry) {
	e = fit(e)
	size := entrySize(e)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, e)
	r.bytes += size
	for len(r.buf) > 1 && (len(r.buf) > r.cap || r.bytes > r.bytesCap) {
		r.bytes -= entrySize(r.buf[0])
		r.buf[0] = Entry{}
		r.buf = r.buf[1:]
	}
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
	for i := n - 1; i >= 0 && len(out) < limit; i-- {
		if e := r.buf[i]; f.matches(e) {
			out = append(out, e)
		}
	}
	return out
}

func entrySize(e Entry) int {
	return len(e.Msg) + len(e.Scope) + len(e.Level) + len(e.Group) + len(e.User) + fieldsSize(e.Fields) + 64
}

func fieldsSize(fields map[string]string) int {
	n := 0
	for k, v := range fields {
		n += len(k) + len(v)
	}
	return n
}

const truncMark = "…(截断)"

func clip(s string, max int) string {
	if max < 0 {
		max = 0
	}
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + truncMark
}

func fit(e Entry) Entry {
	if len(e.Msg) > maxEntryBytes {
		e.Msg = clip(e.Msg, maxEntryBytes)
	}
	if len(e.Fields) > 0 {
		e = fitFields(e)
	}
	return e
}

func fitFields(e Entry) Entry {
	left := maxEntryBytes - (entrySize(e) - fieldsSize(e.Fields))
	if left < 0 {
		left = 0
	}
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	kept := make(map[string]string, len(e.Fields))
	for _, k := range keys {
		room := left - len(k)
		if room <= 0 {
			break
		}
		v := e.Fields[k]
		if len(v) > room {
			kept[k] = clip(v, room)
			left = 0
			break
		}
		kept[k] = v
		left -= len(k) + len(v)
	}
	if len(kept) == 0 {
		e.Fields = nil
		return e
	}
	e.Fields = kept
	return e
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

var contextFieldRe = regexp.MustCompile(`\b(group|user)=([^\s,;]+)`)

func extractContext(msg string) (group, user string) {
	for _, m := range contextFieldRe.FindAllStringSubmatch(msg, -1) {
		switch m[1] {
		case "group":
			if group == "" {
				group = m[2]
			}
		case "user":
			if user == "" {
				user = m[2]
			}
		}
	}
	return group, user
}

// Summary 按级别与来源数一遍缓冲, 给筛选条上的数字用。
type Summary struct {
	Levels map[string]int `json:"levels"`
	Scopes map[string]int `json:"scopes"`
}

func Summarise() Summary { return std.ring.summarise() }

func (r *ring) summarise() Summary {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := Summary{Levels: map[string]int{}, Scopes: map[string]int{}}
	for _, e := range r.buf {
		out.Levels[strings.ToLower(e.Level)]++
		out.Scopes[e.Scope]++
	}
	return out
}
