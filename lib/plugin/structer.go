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
}
