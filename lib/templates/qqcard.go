package templates

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// 模板里写 QQ 卡片的那一个函数。
//
// 用函数而不是让模板手写 <mqqapi://…> 的原因: 十来个参数要么带要么不带,
// 手写出来就是一大坨, 还容易漏掉分隔符。模板里只该写"用哪张卡、放什么内容":
//
//	{{qqcard 2 leftImageUrl=cover scheme=link text1=title text2=artist}}
//
// 参数写成 名字=值, 而不是按位置排: 卡片 2 有十一个参数, 靠顺序对应的话读模板的人
// 根本看不出哪个是哪个, 想跳过中间某个还得数空位。
// 值不带引号时从模板参数里取, 带引号就是写死的字面量 —— 与 {{inline}} 同一套写法。
// 值要 URL 编码: 值里的 & 会被当成参数分隔符, 空格和中文也得编, 不编参数就断了。
const qqCardFunc = "qqcard"

// qqCardParams 是每张卡片认得的参数名; 名字与 QQ 的字段一致, 便于对照文档。
// 这里既用来校验(写错名字会被指出来, 而不是变成一张怪卡片), 也用来拼 URL。
// 加一张新卡 = 在这里加一行。
var qqCardParams = map[string][]string{
	"1": {"leftImageUrl", "leftImageWidth", "leftImageHeight", "scheme", "text1", "text2", "text3", "icon3"},
	"2": {"leftImageUrl", "scheme", "text1", "text2", "text3", "icon1", "icon2", "icon3", "iconDye1", "iconDye2", "iconDye3"},
	"3": {"title", "desc", "tag1", "tag2", "tag3"},
	"5": {"text1", "text2", "plot_id"},
}

func isQQCardCall(tag string) bool {
	_, ok := qqCardCallBody(tag)
	return ok
}

func qqCardCallBody(tag string) (string, bool) {
	if tag == qqCardFunc {
		return "", true
	}
	if !strings.HasPrefix(tag, qqCardFunc) {
		return "", false
	}
	rest := tag[len(qqCardFunc):]
	if !isSpaceByte(rest[0]) {
		return "", false
	}
	return rest, true
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// qqCardArgNames 收出调用里引用的模板参数名, 供填充阶段连同其它占位符一起解析。
func qqCardArgNames(tag string) []string {
	body, ok := qqCardCallBody(tag)
	if !ok {
		return nil
	}
	_, rest, err := splitLeadingToken(body)
	if err != nil {
		return nil
	}
	args, err := parseNamedArgs(rest)
	if err != nil {
		return nil
	}
	var names []string
	for _, a := range args {
		if !a.literal {
			names = append(names, a.value)
		}
	}
	return names
}

// renderQQCard 把 {{qqcard <款式> 名字=值…}} 拼成 <mqqapi://markdown/template…>。
func renderQQCard(tag string, flat map[string]string) (string, error) {
	body, ok := qqCardCallBody(tag)
	if !ok {
		return "", fmt.Errorf("not a qqcard call: %s", tag)
	}
	style, rest, err := splitLeadingToken(body)
	if err != nil {
		return "", fmt.Errorf("qqcard 要写款式, 例如: {{qqcard 2 text1=标题}}")
	}
	names, ok := qqCardParams[style]
	if !ok {
		return "", fmt.Errorf("qqcard 不认识第 %q 款卡片, 可用的是 %s", style, strings.Join(cardStyles(), " "))
	}
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}

	args, err := parseNamedArgs(rest)
	if err != nil {
		return "", fmt.Errorf("qqcard: %w", err)
	}

	pairs := []string{"id=" + style}
	for _, a := range args {
		if !allowed[a.Key] {
			return "", fmt.Errorf("qqcard %s 没有 %q 这个参数, 可用的是 %s", style, a.Key, strings.Join(names, " "))
		}
		value := a.value
		if !a.literal {
			got, ok := flat[a.value]
			if !ok {
				return "", fmt.Errorf("qqcard 的 %s=%s: 找不到 %q 这个参数", a.Key, a.value, a.value)
			}
			value = got
		}
		// 空值等于没写: 空参数和缺参数在客户端是两种结果, 必须整个省掉。
		if strings.TrimSpace(value) == "" {
			continue
		}
		pairs = append(pairs, a.Key+"="+encodeValue(value))
	}
	return "<mqqapi://markdown/template?" + strings.Join(pairs, "&") + ">", nil
}

// splitLeadingToken 取开头那个位置参数, 返回它和剩下的参数串。
func splitLeadingToken(body string) (string, string, error) {
	body = strings.TrimLeftFunc(body, unicode.IsSpace)
	if body == "" {
		return "", "", fmt.Errorf("缺少开头的关键字")
	}
	at := strings.IndexFunc(body, unicode.IsSpace)
	if at < 0 {
		return body, "", nil
	}
	return body[:at], body[at:], nil
}

type namedArg struct {
	Key     string
	value   string
	literal bool
}

// parseNamedArgs 解析 名字=值 序列。值不带引号时读到一个空白为止, 带引号可以含空格。
func parseNamedArgs(input string) ([]namedArg, error) {
	out := make([]namedArg, 0, 4)
	for i := 0; ; {
		for i < len(input) && isSpaceByte(input[i]) {
			i++
		}
		if i >= len(input) {
			return out, nil
		}
		start := i
		for i < len(input) && input[i] != '=' && !isSpaceByte(input[i]) {
			i++
		}
		if i >= len(input) || input[i] != '=' {
			return nil, fmt.Errorf("参数要写成 名字=值, 这里读不懂: %q", input[start:])
		}
		key := input[start:i]
		if key == "" {
			return nil, fmt.Errorf("有个参数没写名字")
		}
		value, next, literal, err := readNamedValue(input, i+1)
		if err != nil {
			return nil, err
		}
		out = append(out, namedArg{Key: key, value: value, literal: literal})
		i = next
	}
}

func readNamedValue(input string, i int) (value string, next int, literal bool, err error) {
	if i < len(input) && (input[i] == '"' || input[i] == '\'') {
		quote := input[i]
		var b strings.Builder
		for i++; i < len(input) && input[i] != quote; i++ {
			b.WriteByte(input[i])
		}
		if i >= len(input) {
			return "", 0, false, fmt.Errorf("有个值的引号没闭合")
		}
		return b.String(), i + 1, true, nil
	}
	start := i
	for i < len(input) && !isSpaceByte(input[i]) {
		i++
	}
	return input[start:i], i, false, nil
}

// cardStyles 列出已知款式, 供报错时提示。
func cardStyles() []string {
	out := make([]string, 0, len(qqCardParams))
	for style := range qqCardParams {
		out = append(out, style)
	}
	sortStrings(out)
	return out
}

func sortStrings(list []string) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j] < list[j-1]; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// encodeValue 幂等地做一次 URL 编码: 已经编码过的值再编一次结果不变。
//
// 幂等很重要 —— 模板可能被填两遍(插件模板回落全局、或调用方重复填),
// 不幂等就会把 %20 变成 %2520。
func encodeValue(value string) string {
	if value == "" {
		return value
	}
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		decoded = value
	}
	// 空格编成 %20 而不是 +: QQ 的参数解析按 %20 处理更稳。
	return strings.ReplaceAll(url.QueryEscape(decoded), "+", "%20")
}
