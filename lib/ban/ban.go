// Package ban 封禁名单: 谁被封了、给他回什么话。名单整份放内存(拦截在每条用户消息的入口, 走库就是每条消息多一跳), 写时整份写回。
package ban

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/storage"
)

var log = logx.New("ban")

const (
	// DefaultMessage 没配自定义文案时发给用户的那句话。
	DefaultMessage = "您的账号因违反Bot使用协议已被封禁"

	storageKey = "roster"

	// MessageLimit 单条文案的字符上限, 与频道单条消息 content 上限(2000 字)对齐; 超了平台拒收, 所以保存时就拒绝。
	MessageLimit = 2000

	throttle = 15 * time.Second

	pruneAt = 4096
)

// Entry 一条封禁记录。
type Entry struct {
	OpenID string `json:"openid"`
	// Message 发给用户的文案, 支持 markdown; 空串表示用 DefaultMessage。
	Message string `json:"message,omitempty"`
	// Note 管理员给自己看的备注, 永远不会发给用户。
	Note string `json:"note,omitempty"`
	// At 封禁时间(RFC3339), 只用于列表排序与展示。
	At string `json:"at,omitempty"`
}

// Text 这条记录该发给用户的话。
func (e Entry) Text() string {
	if strings.TrimSpace(e.Message) == "" {
		return DefaultMessage
	}
	return e.Message
}

type store struct {
	mu      sync.RWMutex
	entries map[string]Entry
	last    map[string]time.Time
}

var s = &store{entries: map[string]Entry{}, last: map[string]time.Time{}}

// Open 从库里加载名单, 启动时调一次; 读失败只返回错误给调用方记日志, 内存保持空表 —— 名单读不出来不该让进程起不来。
func Open() error {
	var list []Entry
	found, err := storage.Global().Get(storageKey, &list)
	if err != nil {
		return fmt.Errorf("读取封禁名单: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[string]Entry, len(list))
	for _, e := range list {
		if e.OpenID == "" {
			continue
		}
		s.entries[e.OpenID] = e
	}
	if !found || len(s.entries) == 0 {
		log.Infof("封禁名单为空")
		return nil
	}
	log.Infof("封禁名单已加载: %d 条", len(s.entries))
	return nil
}

// List 当前名单, 最近封的排在前面。
func List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At != out[j].At {
			return out[i].At > out[j].At
		}
		return out[i].OpenID < out[j].OpenID
	})
	return out
}

// Put 新增或更新一条封禁; 先写库、成功了才改内存, 否则写库失败时管理台已经显示成功。
func Put(e Entry) error {
	e.OpenID = strings.TrimSpace(e.OpenID)
	if e.OpenID == "" {
		return errors.New("openid 不能为空")
	}
	if strings.ContainsAny(e.OpenID, " \t\r\n") {
		return errors.New("openid 不能包含空白字符")
	}
	if n := len([]rune(e.Message)); n > MessageLimit {
		return fmt.Errorf("文案 %d 字, 超过上限 %d 字", n, MessageLimit)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := make(map[string]Entry, len(s.entries)+1)
	for k, v := range s.entries {
		next[k] = v
	}
	if prev, ok := next[e.OpenID]; ok && prev.At != "" {
		e.At = prev.At
	}
	if e.At == "" {
		e.At = time.Now().Format(time.RFC3339)
	}
	next[e.OpenID] = e

	if err := persist(next); err != nil {
		return err
	}
	s.entries = next
	return nil
}

// Delete 解封。
func Delete(openid string) error {
	openid = strings.TrimSpace(openid)
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.entries[openid]; !ok {
		return errors.New("该用户不在封禁名单里")
	}
	next := make(map[string]Entry, len(s.entries))
	for k, v := range s.entries {
		if k == openid {
			continue
		}
		next[k] = v
	}
	if err := persist(next); err != nil {
		return err
	}
	s.entries = next
	delete(s.last, openid)
	return nil
}

// Hit 这个 openid 是否被封, 命中时返回该发给他的话; 同一人在 throttle 内只算一次命中。
func Hit(openid string) (string, bool) {
	if openid == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[openid]
	if !ok {
		return "", false
	}
	now := time.Now()
	if last, seen := s.last[openid]; seen && now.Sub(last) < throttle {
		return "", false
	}
	if len(s.last) > pruneAt {
		for k, v := range s.last {
			if now.Sub(v) >= throttle {
				delete(s.last, k)
			}
		}
	}
	s.last[openid] = now
	return e.Text(), true
}

func persist(entries map[string]Entry) error {
	list := make([]Entry, 0, len(entries))
	for _, e := range entries {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].OpenID < list[j].OpenID })
	if err := storage.Global().Set(storageKey, list); err != nil {
		return fmt.Errorf("保存封禁名单: %w", err)
	}
	return nil
}
