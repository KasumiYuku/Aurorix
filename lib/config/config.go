package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/utils"
	"os"
	"slices"
	"strings"
	"sync"
)

var configLock sync.Mutex

type Plugin struct {
	Id     string   `json:"id"`
	Prefix string   `json:"prefix"`
	Group  []string `json:"group"`
}

type AppConfig struct {
	Port                uint16                    `json:"port"`
	AppId               string                    `json:"appid"`
	AppSecret           string                    `json:"secret"`
	Plugins             []Plugin                  `json:"plugins"`
	ProxyAPI            string                    `json:"proxy"`
	Database            string                    `json:"database"`
	AdminPassword       string                    `json:"admin_password"`
	Protocol            string                    `json:"protocol,omitempty"` // webhook | websocket
	Intents             []string                  `json:"intents,omitempty"`  // websocket 订阅事件
	GlobalMarkdown      bool                      `json:"global_markdown,omitempty"`
	MarkdownVerifyImage bool                      `json:"markdown_verify_image,omitempty"`
	RetryWhen           []int                     `json:"retry_when,omitempty"`
	UploadThreshold     int                       `json:"upload_threshold,omitempty"` // 分片上传阈值(字节)
	LogLevel            string                    `json:"log_level,omitempty"`        // 控制台最低级别
	Prefixes            []string                  `json:"prefixes,omitempty"`         // 指令前缀符号, "" 为无前缀
	PluginSettings      map[string]map[string]any `json:"plugin_settings"`
	PluginAccess        map[string]AccessConfig   `json:"plugin_access"`
}

type AccessRule struct {
	Mode   string   `json:"mode"`
	Users  []string `json:"users,omitempty"`
	Groups []string `json:"groups,omitempty"`
}

type AccessConfig struct {
	Default  AccessRule            `json:"default"`
	Commands map[string]AccessRule `json:"commands,omitempty"`
	Disabled bool                  `json:"disabled,omitempty"` // 停用整个插件
}

// current 运行期配置的内存态, 所有写盘路径都会刷新它。
var current AppConfig

func InitConfig() AppConfig {
	file, err := os.ReadFile("./config.json")
	if err != nil {
		fmt.Println("请正确配置config.json")
		os.Exit(1)
	}

	var appConfig AppConfig
	err = json.Unmarshal(file, &appConfig)
	if err != nil {
		fmt.Println("请正确配置config.json")
		os.Exit(1)
	}
	normalize(&appConfig)
	current = appConfig
	return appConfig
}

// Current 返回当前生效配置的拷贝, 读锁保护。
func Current() AppConfig {
	configLock.Lock()
	defer configLock.Unlock()
	return current
}

func normalize(cfg *AppConfig) {
	if cfg.Database == "" {
		cfg.Database = "bot.db"
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "webhook"
	}
	if cfg.UploadThreshold == 0 {
		cfg.UploadThreshold = 3 << 20 // 3MB
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.Intents == nil {
		cfg.Intents = []string{
			"GROUP_AT_MESSAGE_CREATE", "GROUP_MESSAGE_CREATE", "C2C_MESSAGE_CREATE",
			"INTERACTION_CREATE", "GROUP_JOIN_REQUEST", "GROUP_MEMBER_ADD", "GROUP_MEMBER_REMOVE",
			"MESSAGE_AUDIT_PASS", "MESSAGE_AUDIT_REJECT", "GROUP_ADD_ROBOT", "GROUP_DEL_ROBOT",
		}
	}
	if cfg.Prefixes == nil {
		cfg.Prefixes = constant.DefaultPrefixChars()
	}
}

// persist 读盘 -> 执行变更 -> 写盘 -> 刷新内存态, 全程持锁。
// keys 为变更后需要落盘的顶层键; 对应值为空时删除该键。
func persist(mutate func(cfg *AppConfig) error, keys ...string) error {
	configLock.Lock()
	defer configLock.Unlock()

	rawBytes, err := os.ReadFile("./config.json")
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rawBytes, &raw); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	cfg := current
	if enc, ok := raw["plugin_settings"]; ok {
		json.Unmarshal(enc, &cfg.PluginSettings)
	}
	if enc, ok := raw["plugin_access"]; ok {
		json.Unmarshal(enc, &cfg.PluginAccess)
	}
	if err := mutate(&cfg); err != nil {
		return err
	}

	// 变更键回写: 结构体里的值序列化后覆盖原始键, 其余键保持文件原样
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	var encMap map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &encMap); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	for _, key := range keys {
		if value, ok := encMap[key]; ok {
			raw[key] = value
		} else {
			delete(raw, key)
		}
	}
	updated, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	updated = append(updated, '\n')
	if err := utils.WriteFile("./config.json", updated, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	normalize(&cfg)
	current = cfg
	return nil
}

func SavePluginSettings(pluginID string, settings map[string]any) error {
	return persist(func(cfg *AppConfig) error {
		if cfg.PluginSettings == nil {
			cfg.PluginSettings = make(map[string]map[string]any)
		}
		cfg.PluginSettings[pluginID] = settings
		return nil
	}, "plugin_settings")
}

func SavePluginAccess(pluginID string, access AccessConfig) error {
	return persist(func(cfg *AppConfig) error {
		if cfg.PluginAccess == nil {
			cfg.PluginAccess = make(map[string]AccessConfig)
		}
		cfg.PluginAccess[pluginID] = access
		return nil
	}, "plugin_access")
}

// UpdateCore 原子更新核心配置键。overrides 值需为已按 JSON 编码的合法负载。
// 仅接受白名单键; 键值编码为 null/空时按类型语义落盘。
func UpdateCore(overrides map[string]json.RawMessage) error {
	return persist(func(cfg *AppConfig) error {
		for key, rawValue := range overrides {
			if err := applyCoreKey(cfg, key, rawValue); err != nil {
				return err
			}
		}
		return nil
	}, mapsKeysSorted(overrides)...)
}

func mapsKeysSorted(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

var errCoreUnknownKey = errors.New("未知配置项")

func applyCoreKey(cfg *AppConfig, key string, rawValue json.RawMessage) error {
	// 注意: json.RawMessage(nil) 或 "null" 在 json.Unmarshal 中按零值处理
	switch key {
	case "port":
		var v uint16
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("port: %w", err)
		}
		if v == 0 {
			return errors.New("port 不能为 0")
		}
		cfg.Port = v
	case "appid":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("appid: %w", err)
		}
		cfg.AppId = v
	case "secret":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("secret: %w", err)
		}
		cfg.AppSecret = v
	case "proxy":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("proxy: %w", err)
		}
		cfg.ProxyAPI = v
	case "database":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("database: %w", err)
		}
		cfg.Database = v
	case "admin_password":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("admin_password: %w", err)
		}
		cfg.AdminPassword = v
	case "protocol":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("protocol: %w", err)
		}
		if v != "webhook" && v != "websocket" {
			return fmt.Errorf("protocol 只能为 webhook 或 websocket")
		}
		cfg.Protocol = v
	case "intents":
		var v []string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("intents: %w", err)
		}
		cfg.Intents = cleanStrings(v)
	case "global_markdown":
		var v bool
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("global_markdown: %w", err)
		}
		cfg.GlobalMarkdown = v
	case "markdown_verify_image":
		var v bool
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("markdown_verify_image: %w", err)
		}
		cfg.MarkdownVerifyImage = v
	case "retry_when":
		var v []int
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("retry_when: %w", err)
		}
		cfg.RetryWhen = v
	case "upload_threshold":
		var v int
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("upload_threshold: %w", err)
		}
		if v <= 0 {
			return errors.New("upload_threshold 必须大于 0")
		}
		cfg.UploadThreshold = v
	case "log_level":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("log_level: %w", err)
		}
		if !slices.Contains([]string{"debug", "info", "warn", "error"}, v) {
			return fmt.Errorf("log_level 只能为 debug/info/warn/error")
		}
		cfg.LogLevel = v
	default:
		return fmt.Errorf("%w: %s", errCoreUnknownKey, key)
	}
	return nil
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
