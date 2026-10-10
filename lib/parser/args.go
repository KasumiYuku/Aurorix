package parser

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"github.com/alecthomas/kong"
)

// escapedPrefix 是给「看起来像 flag 但实际是普通参数」的 token 打的哨兵。
//
// 为什么需要它: 曲名里带 "-" 很常见(如 "-迷宮リリス-"), 而 kong 见到 - 开头就当 flag,
// 于是直接报 unknown flag 把整条指令打死。改成给这些 token 加个哨兵前缀让 kong 当位置参数收下,
// 解析完再从结果里摘掉 —— 用户看到的参数原样不变。
//
// 用 \x1f (US, 单元分隔符): 正常输入里不会出现, 也不参与 kong 的 flag/分隔符判断。
const escapedPrefix = "\x1f"

// ParseArgs 用 kong 解析一条指令的参数。
// 返回: 解析结果、出错时的用法文本、错误。
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

// usageOf 拿 kong 生成的完整用法(命令 + 位置参数 + flags 及其 help);
// 它比"前缀 + 命令路径"有用得多 —— 后者等于没告诉用户参数该怎么写。
// 面向中文群聊, 把 kong 的分节标题换成中文(命令名与参数说明本来就来自各插件的 help 标签)。
var usageLabels = strings.NewReplacer(
	"Usage:", "用法:",
	"Arguments:", "参数:",
	"Flags:", "选项:",
	"Show context-sensitive help.", "查看帮助",
	"[flags]", "[选项]",
)

func usageOf(p *kong.Kong, buf *bytes.Buffer, name string, err error) string {
	buf.Reset()
	p.FatalIfErrorf(err) // 装了 kong.Exit 空实现, 这里只把用法打进 buf, 不会真退出
	text := buf.String()
	if i := strings.Index(text, "\n"+name+": error:"); i >= 0 {
		text = text[:i] // 末尾那行是原始报错, 卡片里"原因"已经有了
	}
	return strings.TrimSpace(usageLabels.Replace(text))
}

// protectUnknownFlags 给「不是本命令声明的 flag」的 - 开头 token 打哨兵前缀。
// 声明过的 flag(如 -s/--source) 原样保留, 所以放在位置参数后面也照样生效。
func protectUnknownFlags(p *kong.Kong, tokens []string) []string {
	if len(tokens) == 0 {
		return tokens
	}
	known := declaredFlags(p)
	out := make([]string, 0, len(tokens))
	for i, token := range tokens {
		// 遇到 -- 之后 kong 本来就把剩下的一律当位置参数, 不用管。
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

// declaredFlags 收集本命令树里所有 flag 的写法(--name 与 -x)。
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
	// 短 flag 连写: -swyy 等于 -s wyy。
	if len(name) > 2 && !strings.HasPrefix(name, "--") {
		return known[name[:2]]
	}
	return false
}

// unescape 把哨兵前缀从解析结果里摘掉; 用户拿到的参数与输入完全一致。
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
