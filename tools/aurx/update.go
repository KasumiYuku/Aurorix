package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// cmdUpdate 更新依赖并重新编译检查。在实例目录或插件源码目录里都能跑。
//
//	aurx update                 更新本模块依赖的全部模块 (go get -u ./...)
//	aurx update all             同上
//	aurx update <模块>...        只更新指定模块; 模块名可以只写一段
//	aurx update -n [模块...]     只看会更新什么, 不动文件 (--dry-run)
//
// replace 到本地目录的模块不受影响 —— 那是你自己的代码, 不归上游管。
// 只做「更新 + 编译校验」, 不启动实例: 启动是 aurx run 的事。
func cmdUpdate(args []string) error {
	if _, err := os.ReadFile("go.mod"); err != nil {
		return fmt.Errorf("读取 go.mod: %w (请在实例目录或插件源码目录内执行)", err)
	}

	dryRun := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "-n" || arg == "--dry-run" {
			dryRun = true
			continue
		}
		filtered = append(filtered, arg)
	}

	targets, err := resolveUpdateTargets(filtered)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Println("将更新:", strings.Join(targets, " "))
		return nil
	}

	steps := [][]string{
		append([]string{"get", "-u"}, targets...),
		{"mod", "tidy"},
		{"build", "./..."},
	}
	for _, step := range steps {
		if err := runGo(step...); err != nil {
			return err
		}
	}
	fmt.Println("已更新并编译通过。用 `aurx run` 重新启动实例。")
	return nil
}

// resolveUpdateTargets 把参数翻成 go get 的目标: 空 / `all` 表示全部;
// 否则按模块名解析成完整路径(允许只写一段, 唯一命中即可)。
func resolveUpdateTargets(args []string) ([]string, error) {
	if len(args) == 0 {
		return []string{"./..."}, nil
	}
	modules, err := requiredModules()
	if err != nil {
		return nil, err
	}

	targets := make([]string, 0, len(args))
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		if arg == "" {
			continue
		}
		if strings.EqualFold(arg, "all") {
			return []string{"./..."}, nil
		}
		path, err := matchModule(modules, arg)
		if err != nil {
			return nil, err
		}
		targets = append(targets, path)
	}
	if len(targets) == 0 {
		return []string{"./..."}, nil
	}
	return targets, nil
}

// requiredModules 读 go.mod 里 require 的模块路径(用 go mod edit -json, 省得自己解析语法)。
func requiredModules() ([]string, error) {
	body, err := exec.Command("go", "mod", "edit", "-json").Output()
	if err != nil {
		return nil, fmt.Errorf("读取模块列表: %w", err)
	}
	var mod struct {
		Require []struct {
			Path string `json:"Path"`
		} `json:"Require"`
	}
	if err := json.Unmarshal(body, &mod); err != nil {
		return nil, fmt.Errorf("解析 go.mod: %w", err)
	}
	paths := make([]string, 0, len(mod.Require))
	for _, req := range mod.Require {
		if req.Path != "" {
			paths = append(paths, req.Path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// matchModule 按完整路径或其中一段匹配模块。
// 先看有没有「最后一段完全相同」的, 再看「包含」; 命中多个就报错让用户说清。
func matchModule(modules []string, query string) (string, error) {
	for _, path := range modules {
		if path == query {
			return path, nil
		}
	}

	var exact, loose []string
	for _, path := range modules {
		last := lastSegment(path)
		switch {
		case strings.EqualFold(last, query):
			exact = append(exact, path)
		case strings.Contains(strings.ToLower(last), strings.ToLower(query)):
			loose = append(loose, path)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		return "", ambiguous(query, exact)
	}
	if len(loose) == 1 {
		return loose[0], nil
	}
	if len(loose) > 1 {
		return "", ambiguous(query, loose)
	}
	return "", fmt.Errorf("没有匹配 %q 的模块; 当前依赖:\n  %s", query, strings.Join(modules, "\n  "))
}

func ambiguous(query string, hits []string) error {
	return fmt.Errorf("%q 匹配到多个模块, 请写全:\n  %s", query, strings.Join(hits, "\n  "))
}

func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func runGo(args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}
