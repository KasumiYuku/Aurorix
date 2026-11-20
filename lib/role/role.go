// Package role 回答一个问题: **这个用户在本机器人里是谁**。
package role

import (
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/constant"
)

// Provider 是机器人级角色的一个来源。
type Provider interface {
	Resolve(userID, groupID string, platform constant.RoleRequired) (role constant.RoleRequired, claimed bool)
}

var (
	mu        sync.RWMutex
	providers []Provider
)

// Use 注册一个来源。约定在插件 init() 里调用。
func Use(p Provider) {
	if p == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	providers = append(providers, p)
}

// Resolve 走完整条来源链, 返回最终角色。
func Resolve(userID, groupID string, platform constant.RoleRequired) constant.RoleRequired {
	mu.RLock()
	list := providers
	mu.RUnlock()

	for _, p := range list {
		if resolved, claimed := p.Resolve(userID, groupID, platform); claimed {
			if resolved == "" {
				return constant.RoleMember
			}
			return resolved
		}
	}
	return platformRole(platform)
}

func platformRole(platform constant.RoleRequired) constant.RoleRequired {
	if platform == "" {
		return constant.RoleMember
	}
	return platform
}

// Reset 清空已注册的来源。
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	providers = nil
}
