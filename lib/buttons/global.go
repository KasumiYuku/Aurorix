package buttons

import (
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"sync"
)

var logger = logx.New("buttons")

var CallbackFuncMap map[string]CallbackButtonHandleFunc = map[string]CallbackButtonHandleFunc{}

var CallbackFuncMapLock sync.RWMutex = sync.RWMutex{}

// 根据ButtonID查找处理函数
func GetCallbackFunc(id string) (CallbackButtonHandleFunc, bool) {
	CallbackFuncMapLock.RLock()
	defer CallbackFuncMapLock.RUnlock()
	f, ok := CallbackFuncMap[id]
	return f, ok
}

func RegisterCallbackFunc(id string, handle CallbackButtonHandleFunc) {
	CallbackFuncMapLock.Lock()
	defer CallbackFuncMapLock.Unlock()
	if _, ok := CallbackFuncMap[id]; ok {
		logger.Warnf("对于ID为 %v 注册的回调函数, 覆盖了之前存在的处理函数", id)
	}
	CallbackFuncMap[id] = handle
}
