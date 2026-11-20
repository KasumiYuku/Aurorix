package parser

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"github.com/alecthomas/kong"
)

const escapedPrefix = "\x1f"

// ParseArgs 用 kong 解析一条指令的参数; 返回解析结果、出错时的用法文本与错误。
func ParseArgs(name string, args any, tokens []string) (any, string, error) {
	t := reflect.TypeOf(args)
	if t.Kind() != reflect.Ptr || t.Elem().Kind() != reflect.Struct {
		return nil, "", fmt.Errorf("args 必须是结构体指针")
	}
	inst := reflect.New(t.Elem()).Interface()

	var buf bytes.Buffer
	p, err := kong.New(inst,
		kong.Name(name),
		kong.Exit(func(int) {}),
		kong.Writers(&buf, &buf),
		kong.UsageOnError(),
	)
	if err != nil {
		return nil, "", err
	}

	ctx, err := p.Parse(protectUnknownFlags(p, tokens))
	if err != nil {
		return nil, usageOf(p, &buf, name, err), err
	}
	_ = ctx
	unescape(reflect.ValueOf(inst))
	return inst, "", nil
}

var usageLabels = strings.NewReplacer(
	"Usage:", "用法:",
	"Arguments:", "参数:",
	"Flags:", "选项:",
	"Show context-sensitive help.", "查看帮助",
	"[flags]", "[选项]",
)

func usageOf(p *kong.Kong, buf *bytes.Buffer, name string, err error) string {
	buf.Reset()
	p.FatalIfErrorf(err)
	text := buf.String()
	if i := strings.Index(text, "\n"+name+": error:"); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(usageLabels.Replace(text))
}

func protectUnknownFlags(p *kong.Kong, tokens []string) []string {
	if len(tokens) == 0 {
		return tokens
	}
	known := declaredFlags(p)
	out := make([]string, 0, len(tokens))
	for i, token := range tokens {
		if i > 0 && tokens[i-1] == "--" {
			out = append(out, token)
			continue
		}
		if token == "" || token == "-" || token == "--" || !strings.HasPrefix(token, "-") || isKnownFlag(known, token) {
			out = append(out, token)
			continue
		}
		out = append(out, escapedPrefix+token)
	}
	return out
}

func declaredFlags(p *kong.Kong) map[string]bool {
	known := map[string]bool{"-h": true, "--help": true}
	var walk func(node *kong.Node)
	walk = func(node *kong.Node) {
		for _, flag := range node.Flags {
			if flag.Name != "" {
				known["--"+flag.Name] = true
			}
			if flag.Short != 0 {
				known["-"+string(flag.Short)] = true
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(p.Model.Node)
	return known
}

func isKnownFlag(known map[string]bool, token string) bool {
	name := token
	if i := strings.IndexByte(token, '='); i >= 0 {
		name = token[:i]
	}
	if known[name] {
		return true
	}
	if len(name) > 2 && !strings.HasPrefix(name, "--") {
		return known[name[:2]]
	}
	return false
}

func unescape(value reflect.Value) {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			unescape(value.Elem())
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).PkgPath == "" {
				unescape(value.Field(i))
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			unescape(value.Index(i))
		}
	case reflect.String:
		if value.CanSet() && strings.HasPrefix(value.String(), escapedPrefix) {
			value.SetString(strings.TrimPrefix(value.String(), escapedPrefix))
		}
	}
}
