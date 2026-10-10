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
	TemplateFS     fs.FS
	WebUI          *WebUI
}

type ConfigField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Placeholder string `json:"placeholder,omitempty"`
	Required    bool   `json:"required,omitempty"`
}
