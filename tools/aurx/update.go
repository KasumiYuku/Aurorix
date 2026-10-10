package main

import (
	"fmt"
	"os"
	"os/exec"
)

// cmdUpdate 更新实例依赖的插件, 并重新编译检查。
//
//	aurx update              更新实例依赖的全部模块 (go get -u ./...)
//	aurx update <module>...  只更新指定的模块
//
// replace 到本地目录的模块不会被升级 —— 那是你自己的代码, 不受上游影响。
// 只做「更新 + 编译校验」, 不启动实例: 启动是 aurx run 的事。
func cmdUpdate(args []string) error {
	if _, err := os.ReadFile("go.mod"); err != nil {
		return fmt.Errorf("读取 go.mod: %w (请在实例目录内执行)", err)
	}
	targets := args
	if len(targets) == 0 {
		targets = []string{"./..."}
	}
	steps := [][]string{
		append([]string{"get", "-u"}, targets...),
		{"mod", "tidy"},
		{"build", "./..."},
	}
	for _, args := range steps {
		if err := runGo(args...); err != nil {
			return err
		}
	}
	fmt.Println("已更新并编译通过。用 `aurx run` 重新启动实例。")
	return nil
}

func runGo(args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}
