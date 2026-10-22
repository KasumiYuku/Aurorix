package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/images"
	"github.com/KasumiYuku/Aurorix/lib/templates"
	"github.com/KasumiYuku/Aurorix/lib/utils"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

var configLock sync.Mutex

type AppConfig struct {
	Port                 uint16                    `json:"port"`
	ListenAddr           string                    `json:"listen_addr,omitempty"`
	AppId                string                    `json:"appid"`
	AppSecret            string                    `json:"secret"`
	ProxyAPI             string                    `json:"proxy"`
	Database             string                    `json:"database"`
	AdminPassword        string                    `json:"admin_password"`
	Protocol             string                    `json:"protocol,omitempty"`
	Intents              []string                  `json:"intents,omitempty"`
	GatewayURL           string                    `json:"gateway_url,omitempty"`
	GlobalMarkdown       bool                      `json:"global_markdown,omitempty"`
	FooterButtons        []string                  `json:"footer_buttons,omitempty"`
	FooterButtonsEnabled *bool                     `json:"footer_buttons_enabled,omitempty"`
	WelcomeEnabled       *bool                     `json:"welcome_enabled,omitempty"`
	WelcomeMessages      []string                  `json:"welcome_messages,omitempty"`
	FarewellEnabled      *bool                     `json:"farewell_enabled,omitempty"`
	FarewellMessages     []string                  `json:"farewell_messages,omitempty"`
	WelcomeExtra         string                    `json:"welcome_extra,omitempty"`
	WelcomeButtons       []string                  `json:"welcome_buttons,omitempty"`
	FarewellExtra        string                    `json:"farewell_extra,omitempty"`
	FarewellButtons      []string                  `json:"farewell_buttons,omitempty"`
	RetryWhen            []int                     `json:"retry_when,omitempty"`
	UploadThreshold      int                       `json:"upload_threshold,omitempty"`
	ImageLimitsEnabled   *bool                     `json:"image_limits_enabled,omitempty"`
	ImageMaxWidth        int                       `json:"image_max_width,omitempty"`
	ImageMaxHeight       int                       `json:"image_max_height,omitempty"`
	LogLevel             string                    `json:"log_level,omitempty"`
	Prefixes             []string                  `json:"prefixes,omitempty"`
	MemoryLimitMB        int                       `json:"memory_limit_mb,omitempty"`
	GCPercent            int                       `json:"gc_percent,omitempty"`
	PProfAddr            string                    `json:"pprof_addr,omitempty"`
	PluginSettings       map[string]map[string]any `json:"plugin_settings"`
	PluginAccess         map[string]AccessConfig   `json:"plugin_access"`
	Templates            TemplatesConfig           `json:"templates,omitempty"`
}

type AccessRule struct {
	Mode   string   `json:"mode"`
	Users  []string `json:"users,omitempty"`
	Groups []string `json:"groups,omitempty"`
}

type AccessConfig struct {
	Default  AccessRule            `json:"default"`
	Commands map[string]AccessRule `json:"commands,omitempty"`
	Disabled bool                  `json:"disabled,omitempty"`
}

type TemplatesConfig struct {
	OverrideDir   string `json:"override_dir,omitempty"`
	ReloadSeconds int    `json:"reload_seconds,omitempty"`
}

func (t TemplatesConfig) Interval() time.Duration {
	return time.Duration(t.ReloadSeconds) * time.Second
}

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
	// 监听地址。没显式配置时按协议定默认值: websocket 模式平台不回调进来, 这个 HTTP 端口只剩
	// 管理台, 所以默认只绑回环 —— 公网机器上不该被外面扫到; webhook 模式平台必须能回调进来,
	// 只能绑全网卡。这段必须写在 protocol 默认值之后, 否则会按"还没填"的空串走错分支。
	if cfg.ListenAddr == "" {
		if cfg.Protocol == "websocket" {
			cfg.ListenAddr = "127.0.0.1"
		} else {
			cfg.ListenAddr = "0.0.0.0"
		}
	}
	cfg.GatewayURL = strings.TrimSpace(cfg.GatewayURL)
	if cfg.UploadThreshold == 0 {
		cfg.UploadThreshold = constant.DefaultUploadThreshold
	}
	if cfg.ImageMaxWidth <= 0 || cfg.ImageMaxWidth > maxImageSide {
		cfg.ImageMaxWidth = constant.DefaultImageMaxWidth
	}
	if cfg.ImageMaxHeight <= 0 || cfg.ImageMaxHeight > maxImageSide {
		cfg.ImageMaxHeight = constant.DefaultImageMaxHeight
	}
	if cfg.ProxyAPI == "" {
		cfg.ProxyAPI = "https://api.bot.qq.com"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.Intents == nil {
		cfg.Intents = []string{
			"GROUP_AT_MESSAGE_CREATE", "GROUP_MESSAGE_CREATE", "C2C_MESSAGE_CREATE",
			"INTERACTION_CREATE", "GROUP_JOIN_REQUEST", "GROUP_MEMBER_ADD", "GROUP_MEMBER_REMOVE",
			"MESSAGE_AUDIT_PASS", "MESSAGE_AUDIT_REJECT", "GROUP_ADD_ROBOT", "GROUP_DEL_ROBOT",
			"GROUP_MSG_RECEIVE", "GROUP_MSG_REJECT",
		}
	}
	if cfg.Prefixes == nil {
		cfg.Prefixes = constant.DefaultPrefixChars()
	}
	if strings.HasPrefix(cfg.PProfAddr, ":") {
		cfg.PProfAddr = "127.0.0.1" + cfg.PProfAddr
	}
	if cfg.Templates.OverrideDir == "" {
		cfg.Templates.OverrideDir = templates.DefaultOverrideDir
	}
	if cfg.Templates.ReloadSeconds <= 0 {
		cfg.Templates.ReloadSeconds = templates.DefaultReloadSeconds
	}
}

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
	case "listen_addr":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("listen_addr: %w", err)
		}
		v = strings.TrimSpace(v)
		// 只收裸 IP: 写成 "0.0.0.0:4514" 这种带端口的会被 JoinHostPort 拼成 "0.0.0.0:4514:4514",
		// 报错比让人对着一个起不来的端口猜要好。
		if v != "" && net.ParseIP(v) == nil {
			return fmt.Errorf("listen_addr 必须是 IP 地址(不带端口): %s", v)
		}
		cfg.ListenAddr = v
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
	case "gateway_url":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("gateway_url: %w", err)
		}
		v = strings.TrimSpace(v)
		if v != "" && !strings.HasPrefix(v, "ws://") && !strings.HasPrefix(v, "wss://") {
			return errors.New("gateway_url 需以 ws:// 或 wss:// 开头")
		}
		cfg.GatewayURL = v
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
	case "image_limits_enabled":
		var v bool
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("image_limits_enabled: %w", err)
		}
		cfg.ImageLimitsEnabled = &v
	case "image_max_width":
		v, err := imageSide(rawValue)
		if err != nil {
			return fmt.Errorf("image_max_width: %w", err)
		}
		cfg.ImageMaxWidth = v
	case "image_max_height":
		v, err := imageSide(rawValue)
		if err != nil {
			return fmt.Errorf("image_max_height: %w", err)
		}
		cfg.ImageMaxHeight = v
	case "log_level":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("log_level: %w", err)
		}
		if !slices.Contains([]string{"debug", "info", "warn", "error"}, v) {
			return fmt.Errorf("log_level 只能为 debug/info/warn/error")
		}
		cfg.LogLevel = v
	case "footer_buttons":
		var v []string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("footer_buttons: %w", err)
		}
		cfg.FooterButtons = cleanStrings(v)
	case "footer_buttons_enabled":
		var v bool
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("footer_buttons_enabled: %w", err)
		}
		cfg.FooterButtonsEnabled = &v
	case "welcome_enabled":
		var v bool
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("welcome_enabled: %w", err)
		}
		cfg.WelcomeEnabled = &v
	case "welcome_messages":
		var v []string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("welcome_messages: %w", err)
		}
		lines := cleanStrings(v)
		if err := validateCmdTexts("welcome_messages", lines); err != nil {
			return err
		}
		cfg.WelcomeMessages = lines
	case "farewell_enabled":
		var v bool
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("farewell_enabled: %w", err)
		}
		cfg.FarewellEnabled = &v
	case "farewell_messages":
		var v []string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("farewell_messages: %w", err)
		}
		lines := cleanStrings(v)
		if err := validateCmdTexts("farewell_messages", lines); err != nil {
			return err
		}
		cfg.FarewellMessages = lines
	case "welcome_extra":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("welcome_extra: %w", err)
		}
		if err := ValidateCmdTags("welcome_extra", v); err != nil {
			return err
		}
		cfg.WelcomeExtra = strings.TrimSpace(v)
	case "welcome_buttons":
		var v []string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("welcome_buttons: %w", err)
		}
		cfg.WelcomeButtons = cleanStrings(v)
	case "farewell_extra":
		var v string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("farewell_extra: %w", err)
		}
		if err := ValidateCmdTags("farewell_extra", v); err != nil {
			return err
		}
		cfg.FarewellExtra = strings.TrimSpace(v)
	case "farewell_buttons":
		var v []string
		if err := json.Unmarshal(rawValue, &v); err != nil {
			return fmt.Errorf("farewell_buttons: %w", err)
		}
		cfg.FarewellButtons = cleanStrings(v)
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

// FooterEnabled 报告底部按钮是否启用。该开关省略时视为启用 —— 只填了按钮列表就能生效,
// 显式写 false 才是「静音但保留配置」。
func (cfg AppConfig) FooterEnabled() bool {
	return cfg.FooterButtonsEnabled == nil || *cfg.FooterButtonsEnabled
}

// WelcomeOn 报告入群欢迎是否启用; 开关省略时视为启用, 但文案为空照样不发。
func (cfg AppConfig) WelcomeOn() bool {
	return cfg.WelcomeEnabled == nil || *cfg.WelcomeEnabled
}

// FarewellOn 报告退群提示是否启用。
func (cfg AppConfig) FarewellOn() bool {
	return cfg.FarewellEnabled == nil || *cfg.FarewellEnabled
}

const maxImageSide = 4096

// imageSide 解析图片边长: 只收 16..maxImageSide 之间的整数。
func imageSide(rawValue json.RawMessage) (int, error) {
	var v int
	if err := json.Unmarshal(rawValue, &v); err != nil {
		return 0, err
	}
	if v < 16 || v > maxImageSide {
		return 0, fmt.Errorf("需在 16..%d 之间", maxImageSide)
	}
	return v, nil
}

// ImageBox 报告 markdown 图片的显示上限; 开关省略时视为开启, 与底部按钮同一个约定。
func (cfg AppConfig) ImageBox() images.Box {
	if cfg.ImageLimitsEnabled != nil && !*cfg.ImageLimitsEnabled {
		return images.Box{}
	}
	return images.Box{Width: cfg.ImageMaxWidth, Height: cfg.ImageMaxHeight}
}
