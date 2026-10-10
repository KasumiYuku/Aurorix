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
	GroupStorage   *storage.Store
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

// BindStorage exposes namespaces bound to the command currently being handled.
func (c *Context) BindStorage(pluginID, commandID string) {
	c.PluginStorage = storage.Plugin(pluginID)
	c.CommandStorage = storage.Command(pluginID, commandID)
}

func (c *Context) SetGroupId(id string) {
	c.MessageManager.GroupId = id
	if id != "" {
		c.GroupStorage = storage.Group(id)
	}
}

func (c *Context) SetUserId(id string) {
	c.MessageManager.UserId = id
	if id != "" {
		c.UserStorage = storage.User(id)
	}
}

func (c *Context) SetMessageOrigin(origin constant.MessageOrigin) {
	c.MessageManager.Target = origin
}
