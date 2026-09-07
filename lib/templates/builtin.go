package templates

import "embed"

// 框架内置模板, 注册到全局命名空间供所有插件复用。
//
//go:embed builtin/markdown/*.md
var builtinFS embed.FS

func init() {
	if err := RegisterFS("", builtinFS, "builtin/markdown"); err != nil {
		panic(err)
	}
}
