package inbox

import (
	"sync"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/event"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/stats"
	"github.com/KasumiYuku/Aurorix/lib/storage"
)

var log = logx.New("inbox")

const (
	nameQueue   = 512
	nameWorkers = 2
)

type handler struct {
	store *Store
	api   *api.BotAPI
	names chan constant.Target

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

	h := &handler{
		store: store,
		api:   client,
		names: make(chan constant.Target, nameQueue),
		busy:  make(map[string]bool),
	}
	event.On(constant.GROUP_AT_MESSAGE_CREATE, h.inbound)
	event.On(constant.GROUP_MESSAGE_CREATE, h.inbound)
	event.On(constant.C2C_MESSAGE_CREATE, h.inbound)
	event.On(constant.AT_MESSAGE_CREATE, h.inbound)
	event.On(constant.MESSAGE_CREATE, h.inbound)
	event.On(constant.DIRECT_MESSAGE_CREATE, h.inbound)
	message.SetSentHook(h.outbound)
	for range nameWorkers {
		go h.nameWorker()
	}
	go h.backfillNames()
	return store
}

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

func (h *handler) inbound(_ *event.Context, ev *event.MessageEvent) {
	if ev == nil || ev.Target.User == "" || ev.Target.Empty() {
		return
	}
	h.save(Message{
		ConvID:     ev.Target.ID,
		Kind:       ev.Target.Scene,
		Direction:  Inbound,
		SenderID:   ev.Target.User,
		SenderName: ev.Username,
		Content:    ev.Content,
		PlatformID: ev.MessageID,
	})
	if ev.Target.Scene == constant.SceneC2C && ev.Username != "" {
		if err := h.store.SetName(ev.Target.ID, ev.Username); err != nil {
			log.Warnf("回填单聊会话名失败: %v", err)
		}
	}
	h.fillName(ev.Target)
}

func (h *handler) outbound(sent message.SentMessage) {
	if sent.Target.Empty() || sent.Body == "" {
		return
	}
	h.save(Message{
		ConvID:     sent.Target.ID,
		Kind:       sent.Target.Scene,
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

// SetSavedHook 注册"消息已经进库"的回调, 传 nil 注销; 存成功了才推, 页面上不会出现库里没有的消息。
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

func (h *handler) fillName(target constant.Target) {
	if target.Empty() || h.store.NameOf(target.ID) != "" || !h.markBusy(target.ID) {
		return
	}
	select {
	case h.names <- target:
	default:
		h.unmarkBusy(target.ID)
	}
}

func (h *handler) nameWorker() {
	for target := range h.names {
		h.fetchName(target)
		h.unmarkBusy(target.ID)
	}
}

func (h *handler) fetchName(target constant.Target) {
	if h.api == nil {
		return
	}
	var name string
	switch target.Scene {
	case constant.SceneGroup:
		if info, err := h.api.Group.GetInfo(target.ID); err == nil && info != nil {
			name = info.GroupName
		}
	case constant.SceneChannel:
		if info, err := h.api.Channel.Info(target.ID); err == nil && info != nil {
			name = info.Name
		}
	case constant.SceneDirect:
		if info, err := h.api.Channel.Guild(target.ID); err == nil && info != nil {
			name = info.Name
		}
	}
	if name == "" {
		return
	}
	if err := h.store.SetName(target.ID, name); err != nil {
		log.Warnf("回填会话名失败: %v", err)
	}
	stats.Name(target, name)
}

func (h *handler) markBusy(convID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.busy[convID] {
		return false
	}
	h.busy[convID] = true
	return true
}

func (h *handler) unmarkBusy(convID string) {
	h.mu.Lock()
	delete(h.busy, convID)
	h.mu.Unlock()
}
