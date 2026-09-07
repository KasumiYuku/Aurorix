package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// cmdNewPlugin 在当前实例生成插件骨架: plugins/<name>/<name>.go。
// 用法: aurx new-plugin <插件名> [--dir 目录]
func cmdNewPlugin(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: aurx new-plugin <插件名> [--dir plugins]")
	}
	name := sanitizeModule(args[0])
	dir := "plugins"
	for i := 1; i < len(args); i++ {
		if args[i] == "--dir" && i+1 < len(args) {
			dir = args[i+1]
			i++
		}
	}
	dir = filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Join(dir, "templates/markdown"), 0o755); err != nil {
		return err
	}
	mod := moduleName()
	if mod == "" {
		return fmt.Errorf("读取 go.mod 失败, 请在实例目录内执行")
	}
	importPath := mod + "/" + filepath.ToSlash(dir)
	content := fmt.Sprintf(`package %s

import (
	"embed"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
)

//go:embed templates/markdown/*.md
var templateFS embed.FS

func init() {
	plugin.Register(&plugin.Plugin{
		Id:         %q,
		Name:       %q,
		TemplateFS: templateFS,
		Commands: []*plugin.Command{
			{
				Prefix:   %q,
				Role:     constant.RoleMember,
				Describe: %q,
				Handle:   cmd,
			},
		},
	})
}

func cmd(ctx *context.MessageContext) error {
	return ctx.Text("插件运行中").Send()
}
`, name, name, name, name, name)
	file := filepath.Join(dir, name+".go")
	if _, err := os.Stat(file); err == nil {
		return fmt.Errorf("已存在: %s", file)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(dir, "templates/markdown", "Hello.md"),
		[]byte("# {{title}}\n{{body}}\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("插件骨架已生成: %s（模板目录 templates/markdown, 已随插件 embed）\n", file)
	fmt.Printf("接入: aurx add %s\n", importPath)
	fmt.Println("编写指令后 aurx run 即可运行。完整 API 见框架 docs/。")
	return nil
}
