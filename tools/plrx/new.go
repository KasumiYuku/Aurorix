package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cmdNew 生成实例骨架: go.mod + main.go + config.example.json。
func cmdNew(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: plrx new <目录> [--framework 框架路径]")
	}
	dir := args[0]
	framework := "../Polarix"
	for i := 1; i < len(args); i++ {
		if args[i] == "--framework" && i+1 < len(args) {
			framework = args[i+1]
			i++
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := filepath.Base(dir)
	module := sanitizeModule(name)
	files := map[string]string{
		"go.mod": fmt.Sprintf(`module %s

go 1.26

require Plrx v0.0.0

replace Plrx => %s
`, module, framework),
		"main.go": `package main

import (
	"Plrx/lib/bot"
)

func main() {
	bot.Run()
}
`,
		".gitignore": "config.json\nassets.json\ndata/\n*.db\n*.db-shm\n*.db-wal\n" + module + "\npolarix\n",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
			return err
		}
	}
	if err := initConfig(dir, framework); err != nil {
		return fmt.Errorf("生成 config 失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0o755); err != nil {
		return err
	}
	if err := copyTemplates(dir, framework); err != nil {
		return err
	}
	if err := runTidy(dir); err != nil {
		return fmt.Errorf("go mod tidy 失败: %w (可稍后在实例目录手动执行)", err)
	}
	fmt.Printf("实例已创建: %s\n", dir)
	fmt.Printf("下一步: cd %s && 编辑 config.json 填入凭证 && plrx add <插件module> && plrx run\n", dir)
	return nil
}

// initConfig 从框架 config.example.json 克隆全字段配置; 数据库入 data/, 生成 config.example.json + config.json。
func initConfig(dir, framework string) error {
	content, err := os.ReadFile(filepath.Join(framework, "config.example.json"))
	if err != nil {
		return err
	}
	cfg := strings.Replace(string(content), `"database": "bot.db"`, `"database": "data/bot.db"`, 1)
	for _, name := range []string{"config.example.json", "config.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(cfg), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// copyTemplates 把框架官方模板复制进实例 templates/markdown, 实例独有一份。
func copyTemplates(dir, framework string) error {
	src := filepath.Join(framework, "templates", "markdown")
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil // 框架无模板目录时跳过
	}
	dst := filepath.Join(dir, "templates", "markdown")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// runTidy 在指定目录执行 go mod tidy, 拉齐框架第三方依赖。
func runTidy(dir string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// sanitizeModule 目录名转合法 module 名 (仅小写字母数字与 -_. )。
func sanitizeModule(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "polarix-bot"
	}
	return b.String()
}
