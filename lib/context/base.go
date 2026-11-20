package context

import (
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/requests"
	"github.com/KasumiYuku/Aurorix/lib/storage"
)

type Context struct {
	*MessageManager
	Request        *requests.Client
	GlobalStorage  *storage.Store
	PluginStorage  *storage.Store
	CommandStorage *storage.Store
	UserStorage    *storage.Store
	// TargetStorage 本次会话的存储命名空间: 群/单聊/子频道/频道私信各算各的, 不共桶。
	TargetStorage *storage.Store
}

// Init 初始化 Context 与 MessageManager。
func (c *Context) Init(messageId, eventId string, qqapi *api.BotAPI) {
	c.MessageManager = &MessageManager{
		MessageId: messageId,
		EventId:   eventId,
		Qapi:      qqapi,
	}
	c.Request = qqapi.Request
	c.GlobalStorage = storage.Global()
}

// BindStorage 暴露当前正在处理的指令所绑定的命名空间。
func (c *Context) BindStorage(pluginID, commandID string) {
	c.PluginStorage = storage.Plugin(pluginID)
	c.CommandStorage = storage.Command(pluginID, commandID)
}

// SetTarget 绑定这次回复的会话: 场景、会话 ID 与触发者。
func (c *Context) SetTarget(target constant.Target) {
	c.MessageManager.Target = target
	if target.ID != "" {
		c.TargetStorage = storage.Scene(target.Scene, target.ID)
	}
	if target.User != "" {
		c.UserStorage = storage.User(target.User)
	}
}
