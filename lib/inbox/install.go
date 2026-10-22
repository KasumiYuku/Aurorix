package inbox

import (
	"sync"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/event"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/storage"
)

var log = logx.New("inbox")

type handler struct {
	store *Store
	api   *api.BotAPI

	mu   sync.Mutex
	busy map[string]bool
}

// Install 接上收发两侧的存档。数据库不可用时只告警: 存档是旁路, 不该拖垮机器人本身。
func Install(client *api.BotAPI) *Store {
	db, err := storage.DB()
	if err != nil {
		log.Warnf("消息存档未启用: %v", err)
		return nil
	}
	store := New(db)
	if err := store.Ensure(); err != nil {
		log.Warnf("消息存档建表失败: %v", err)
		return nil
	}
	if err := store.Seed(); err != nil {
		log.Warnf("导入历史会话失败: %v", err)
	}
	go func() {
		if err := store.Trim(); err != nil {
			log.Warnf("清理旧消息失败: %v", err)
		}
	}()

	h := &handler{store: store, api: client, busy: make(map[string]bool)}
	event.On(constant.GROUP_AT_MESSAGE_CREATE, h.inbound)
	event.On(constant.C2C_MESSAGE_CREATE, h.inbound)
	message.SetSentHook(h.outbound)
	go h.backfillNames()
	return store
}

// backfillNames 把种子会话的群名补齐。
//
// 种子只拿到 openid, 群名得一个个查; 串行 + 间隔是因为六十多个群不该在启动瞬间打六十多个请求。
// 失败的跳过, 下次启动还会再来一遍。
func (h *handler) backfillNames() {
	if h.api == nil || h.api.Group == nil {
		return
	}
	convs, err := h.store.Conversations()
	if err != nil {
		return
	}
	filled := 0
	for _, conv := range convs {
		if conv.Kind != KindGroup || conv.Name != "" {
			continue
		}
		if info, err := h.api.Group.GetInfo(conv.ID); err == nil && info != nil && info.GroupName != "" {
			if h.store.SetName(conv.ID, info.GroupName) == nil {
				filled++
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	if filled > 0 {
		log.Infof("回填群名 %d 个", filled)
	}
}

// inbound 只收得到"和机器人有关"的消息: 群里是 @ 到机器人的, 单聊是直接发给机器人的。
func (h *handler) inbound(_ *event.Context, ev *event.MessageEvent) {
	if ev == nil || ev.UserID == "" {
		return
	}
	kind, convID := KindC2C, ev.UserID
	if ev.GroupID != "" {
		kind, convID = KindGroup, ev.GroupID
	}
	h.save(Message{
		ConvID:     convID,
		Kind:       kind,
		Direction:  Inbound,
		SenderID:   ev.UserID,
		SenderName: ev.Username,
		Content:    ev.Content,
		PlatformID: ev.MessageID,
	})
	if kind == KindGroup {
		h.fillGroupName(convID)
	}
}

func (h *handler) outbound(sent message.SentMessage) {
	kind, convID := KindC2C, sent.UserID
	if sent.Target == constant.GroupMessage {
		kind, convID = KindGroup, sent.GroupID
	}
	if convID == "" || sent.Body == "" {
		return
	}
	h.save(Message{
		ConvID:     convID,
		Kind:       kind,
		Direction:  Outbound,
		Content:    sent.Body,
		PlatformID: sent.MsgID,
	})
}

func (h *handler) save(m Message) {
	if err := h.store.Save(m); err != nil {
		log.Errorf("消息入库失败: %v", err)
		return
	}
	notifySaved(m)
}

var (
	savedHookMu sync.RWMutex
	savedHook   func(Message)
)

// SetSavedHook 注册"消息已经进库"的回调; 传 nil 注销。
// 管理台靠它把新消息推成实时事件 —— 存成功了才推, 页面上不会出现库里没有的消息。
func SetSavedHook(fn func(Message)) {
	savedHookMu.Lock()
	savedHook = fn
	savedHookMu.Unlock()
}

func notifySaved(m Message) {
	savedHookMu.RLock()
	fn := savedHook
	savedHookMu.RUnlock()
	if fn != nil {
		fn(m)
	}
}

// fillGroupName 异步补群名, 同一个群只飞一次: 列表里 32 位 openid 对谁都没意义。
func (h *handler) fillGroupName(convID string) {
	if h.store.NameOf(convID) != "" {
		return
	}
	h.mu.Lock()
	if h.busy[convID] {
		h.mu.Unlock()
		return
	}
	h.busy[convID] = true
	h.mu.Unlock()

	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.busy, convID)
			h.mu.Unlock()
		}()
		if h.api == nil || h.api.Group == nil {
			return
		}
		info, err := h.api.Group.GetInfo(convID)
		if err != nil || info == nil || info.GroupName == "" {
			return
		}
		if err := h.store.SetName(convID, info.GroupName); err != nil {
			log.Warnf("回填群名失败: %v", err)
		}
	}()
}
