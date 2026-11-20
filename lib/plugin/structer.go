package plugin

import (
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"io/fs"
)

type CommandHandleFunc func(*context.MessageContext) error
type PermissionDeniedHandleFunc func(*context.MessageContext) error
type CommandErrorHandleFunc func(*context.MessageContext, error) error

type Command struct {
	Prefix             string
	Aliases            []string
	Role               constant.RoleRequired
	DisablePrivate     bool
	NoFooter           bool
	Describe           string
	Handle             CommandHandleFunc
	PermissionDenied   PermissionDeniedHandleFunc
	HandleError        CommandErrorHandleFunc
	PluginId           string
	SubCommand         []*Command
	SubCommandFallback CommandHandleFunc
	Args               any
	Rate               Rate

	children map[string]*Command
}

type Plugin struct {
	Id             string
	Name           string
	Description    string
	Commands       []*Command
	Config         []ConfigField
	ValidateConfig func(map[string]any) error
	ApplyConfig    func(map[string]any) error
	// OnConfigApplied 在配置真正生效之后调用(启动加载与面板保存都算); init() 里读到的永远只有出厂默认值。
	OnConfigApplied func()
	// TemplateFS 插件自带 markdown 模板, 目录约定 templates/markdown/*.md。
	TemplateFS fs.FS

	// HTMLTemplateFS 插件自带 HTML 模板, 目录约定 templates/html/*.html; 与 markdown 同构, 两个目录可以放在同一个 embed.FS 里。
	HTMLTemplateFS fs.FS

	WebUI *WebUI
}

type ConfigField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Placeholder string `json:"placeholder,omitempty"`
	Required    bool   `json:"required,omitempty"`

	// Options 非空时按下拉框渲染, Type 写 select。
	Options []string `json:"options,omitempty"`

	// ShowIf 让字段只在另一字段取到指定值时才出现, 一份配置里放多套互斥参数时用。
	ShowIf *ShowIf `json:"show_if,omitempty"`

	// Default 是面板首次打开时的初始值; 布尔和文本框缺值本来就按空渲染, 只有下拉与依赖它的 ShowIf 判断需要它。
	Default any `json:"default,omitempty"`
}

type ShowIf struct {
	Key    string   `json:"key,omitempty"`
	Values []string `json:"values,omitempty"`

	// Any 是"任一成立就显示"的分支: 同一个平台被文字和图片分别选中时, 那套参数要能在两种情况下都出现。
	Any []ShowIf `json:"any,omitempty"`
}
