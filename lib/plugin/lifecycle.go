package plugin

import (
	"github.com/KasumiYuku/Aurorix/lib/context"
	"sync"
)

// 处理入群请求
type joinGroupHandle func(*context.ApplyJoinGroupContext) error

var joinGroupHandleFunc joinGroupHandle
var joinGroupHandleLock sync.Locker

func SetGlobalJoinGroupHandle(handle joinGroupHandle) {
	joinGroupHandleLock.Lock()
	defer joinGroupHandleLock.Unlock()
	joinGroupHandleFunc = handle
}

func CallGlobalJoinGroupHandle(ctx *context.ApplyJoinGroupContext) error {
	joinGroupHandleLock.Lock()
	defer joinGroupHandleLock.Unlock()
	return joinGroupHandleFunc(ctx)
}
