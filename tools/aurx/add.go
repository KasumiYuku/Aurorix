package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// cmdAdd 把插件模块写入实例 main.go 空导入, 并 go get 拉取依赖。
// 用法: aurx add github.com/某作者/某插件
func cmdAdd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: aurx add <插件 module 路径>")
	}
	path := args[0]
	mainFile := "main.go"
	src, err := os.ReadFile(mainFile)
	if err != nil {
		return fmt.Errorf("读取 main.go: %w (请在实例目录内执行)", err)
	}
	content := string(src)
	inject := `_ "` + path + `"`
	if strings.Contains(content, inject) {
		return fmt.Errorf("插件已接入: %s", path)
	}
	// 注入点: import 块内 (首个 import 声明后的闭括号前)
	importPos := strings.Index(content, "import (")
	if importPos < 0 {
		return fmt.Errorf("main.go 未找到 import 块")
	}
	closePos := strings.Index(content[importPos:], ")")
	if closePos < 0 {
		return fmt.Errorf("main.go 的 import 块不完整")
	}
	pos := importPos + closePos
	content = content[:pos] + "\t" + inject + "\n" + content[pos:]
	if err := os.WriteFile(mainFile, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Printf("已接入插件 %s\n", path)
	// 实例内部插件 (module 前缀) 无需拉取, 直接结束
	if strings.HasPrefix(path, moduleName()+"/") {
		return nil
	}
	return goGet(path)
}

// moduleName 读取当前 go.mod 的 module 名。
func moduleName() string {
	src, err := os.ReadFile("go.mod")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

// goGet 拉取插件模块依赖, 失败不阻断 (可稍后 go mod tidy)。
func goGet(path string) error {
	cmd := exec.Command("go", "get", path)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "aurx: go get 失败, 可稍后手动执行 go mod tidy:", err)
	}
	return nil
}
