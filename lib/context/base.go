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
func (context *Context) Init(messageId, eventId string, qqapi *api.BotAPI) {
	context.MessageManager = &MessageManager{
		MessageId: messageId,
		EventId:   eventId,
		Qapi:      qqapi,
	}
	context.Request = qqapi.Request
	context.GlobalStorage = storage.Global()
}

// BindStorage exposes namespaces bound to the command currently being handled.
func (context *Context) BindStorage(pluginID, commandID string) {
	context.PluginStorage = storage.Plugin(pluginID)
	context.CommandStorage = storage.Command(pluginID, commandID)
}

func (context *Context) SetGroupId(id string) {
	context.MessageManager.GroupId = id
	if id != "" {
		context.GroupStorage = storage.Group(id)
	}
}

func (context *Context) SetUserId(id string) {
	context.MessageManager.UserId = id
	if id != "" {
		context.UserStorage = storage.User(id)
	}
}

func (context *Context) SetMessageOrigin(origin constant.MessageOrigin) {
	context.MessageManager.Target = origin
}
