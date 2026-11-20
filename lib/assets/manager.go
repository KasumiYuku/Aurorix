package assets

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/utils"
)

// Manager 图床配置管理器
type Manager struct {
	path     string
	client   *Client
	mu       sync.Mutex
	cfg      HostConfig
	host     *ImageHost
	onReload func(*ImageHost)
}

// NewManager 创建管理器，默认配置路径为工作目录下的 assets.json。
func NewManager(cl *Client) *Manager {
	return &Manager{path: "assets.json", client: cl, host: NewHost(HostConfig{}, cl)}
}

// WithPath 指定配置路径，覆盖默认的 assets.json。
func (m *Manager) WithPath(path string) *Manager {
	m.path = path
	return m
}

// OnReload 注册聚合器重建回调，保存配置后触发，用于注入 api.BotAPI.SetAssets。
func (m *Manager) OnReload(fn func(*ImageHost)) {
	m.mu.Lock()
	m.onReload = fn
	m.mu.Unlock()
}

// Load 从磁盘读取配置并重建聚合器；文件缺失视为空配置，不中断启动。
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var cfg HostConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	m.Reload()
	return nil
}

// Config 返回当前配置副本。
func (m *Manager) Config() HostConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// Host 返回当前聚合器。
func (m *Manager) Host() *ImageHost {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.host
}

// Save 持久化配置到磁盘并重建聚合器，触发热更新回调。
func (m *Manager) Save(cfg HostConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := utils.WriteFile(m.path, append(data, '\n'), 0600); err != nil {
		return err
	}
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	m.Reload()
	return nil
}

// Reload 用当前配置重建聚合器并触发回调。
func (m *Manager) Reload() {
	m.mu.Lock()
	host := NewHost(m.cfg, m.client)
	m.host = host
	cb := m.onReload
	m.mu.Unlock()
	if cb != nil {
		cb(host)
	}
}
