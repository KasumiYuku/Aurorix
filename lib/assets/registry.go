package assets

import (
	"fmt"
	"maps"
	"slices"
)

type providerEntry struct {
	factory   ProviderFactory
	schema    []ConfigField
	defaultOn bool
	probeSkip bool
}

var registry = make(map[string]providerEntry)

// Register 注册图床 provider，name 全局唯一。默认语义：必须显式配置才启用，上传后要探测。
func Register(name string, factory ProviderFactory, schema []ConfigField) {
	register(name, factory, schema, providerEntry{})
}

// RegisterDefault 注册"默认启用"的 provider：assets.json 里没写它，也参与链路
func RegisterDefault(name string, factory ProviderFactory, schema []ConfigField) {
	register(name, factory, schema, providerEntry{defaultOn: true, probeSkip: true})
}

// RegisterNoProbe 注册"上传后不做 HEAD 探测"的普通 provider。
func RegisterNoProbe(name string, factory ProviderFactory, schema []ConfigField) {
	register(name, factory, schema, providerEntry{probeSkip: true})
}

func register(name string, factory ProviderFactory, schema []ConfigField, flags providerEntry) {
	if _, ok := registry[name]; ok {
		panic("assets: duplicate provider: " + name)
	}
	flags.factory = factory
	flags.schema = schema
	registry[name] = flags
}

// IsDefaultOn 该 provider 是否"未配置也启用"。
func IsDefaultOn(name string) bool {
	e, ok := registry[name]
	return ok && e.defaultOn
}

// ProbeSkippedByDefault 宿主是否默认不对该 provider 做上传后探测。
func ProbeSkippedByDefault(name string) bool {
	e, ok := registry[name]
	return ok && e.probeSkip
}

// Names 返回所有已注册 provider 名称（按名称排序，顺序稳定）。
func Names() []string {
	return slices.Sorted(maps.Keys(registry))
}

// ProviderSchema 返回 provider 的配置字段定义，供管理面板使用。
func ProviderSchema(name string) []ConfigField {
	if e, ok := registry[name]; ok {
		return e.schema
	}
	return nil
}

// Instantiate 从配置项实例化一个 provider。
func Instantiate(name string, cl *Client, config map[string]any) (ImageProvider, error) {
	e, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	return e.factory(cl, config)
}

// ConfigField 图床 provider 配置字段定义，与 plugin.ConfigField 一致。
type ConfigField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`
	Required    bool     `json:"required,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Default     any      `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
}
