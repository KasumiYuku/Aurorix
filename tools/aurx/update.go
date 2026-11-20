package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

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
