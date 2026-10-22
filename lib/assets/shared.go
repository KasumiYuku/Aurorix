package assets

import "sync/atomic"

// shared 是当前生效的图床聚合器, 供插件侧直接转存媒体。
var shared atomic.Pointer[ImageHost]

// Share 由框架在装配与配置重载后调用, 把生效的聚合器挂到包级入口。
func Share(h *ImageHost) { shared.Store(h) }

// Shared 返回当前生效的图床; 还没装配时为 nil。
func Shared() *ImageHost { return shared.Load() }
