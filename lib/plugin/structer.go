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
	Prefix             string                     // 规范指令名
	Aliases            []string                   // 别名, 匹配与按钮规范化共用
	Role               constant.RoleRequired      // 最低权限
	DisablePrivate     bool                       // 是否禁止在私聊中使用
	Describe           string                     // 指令描述
	Handle             CommandHandleFunc          // 处理函数
	PermissionDenied   PermissionDeniedHandleFunc // 权限不足时调用
	HandleError        CommandErrorHandleFunc     // Handle 返回错误后调用
	PluginId           string                     // 属于的插件ID
	SubCommand         []*Command                 // 子指令
	SubCommandFallback CommandHandleFunc          // 子指令未找到时回退的函数
	Args               any                        // 参数结构体(kong tag 声明), nil 时解析为剩余文本

	children map[string]*Command // 名称+别名索引, 注册时构建
}

type Plugin struct {
	Id             string
	Name           string
	Description    string
	Commands       []*Command
	Config         []ConfigField
	ValidateConfig func(map[string]any) error
	ApplyConfig    func(map[string]any) error
	// TemplateFS 插件自带 markdown 模板, 注册时自动装载到插件命名空间。
	TemplateFS fs.FS
}

type ConfigField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Placeholder string `json:"placeholder,omitempty"`
	Required    bool   `json:"required,omitempty"`
}
