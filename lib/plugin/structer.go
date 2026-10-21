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
	NoFooter           bool // 这条指令的回复不带框架级底部按钮, 它的子指令一并豁免
	Describe           string
	Handle             CommandHandleFunc
	PermissionDenied   PermissionDeniedHandleFunc
	HandleError        CommandErrorHandleFunc
	PluginId           string
	SubCommand         []*Command
	SubCommandFallback CommandHandleFunc
	Args               any

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
	// TemplateFS 插件自带 markdown 模板, 目录约定 templates/markdown/*.md。
	TemplateFS fs.FS

	// HTMLTemplateFS 插件自带 HTML 模板, 目录约定 templates/html/*.html。与 markdown 同构:
	// 按文件名注册进插件命名空间, 用 templates.FillHTMLFor(插件Id, ...) 填充。两个目录可以
	// 放在同一个 embed.FS 里, 两处都传它即可。
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

	// ShowIf 让字段只在另一字段取到指定值时才出现: 一份配置里放多套互斥参数
	// (比如审核平台选 AI 还是腾讯云)时, 不隐藏的话用户分不清哪些该填。
	ShowIf *ShowIf `json:"show_if,omitempty"`

	// Default 是面板首次打开时的初始值; 布尔和文本框缺值本来就按空渲染, 只有下拉
	// 和依赖它的 ShowIf 判断需要, 否则没存过配置时条件字段会全露出来。
	Default any `json:"default,omitempty"`
}

type ShowIf struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}
