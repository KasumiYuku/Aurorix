package templates

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

const qqCardFunc = "qqcard"

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
		if strings.TrimSpace(value) == "" {
			continue
		}
		pairs = append(pairs, a.Key+"="+encodeValue(value))
	}
	return "<mqqapi://markdown/template?" + strings.Join(pairs, "&") + ">", nil
}

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

func encodeValue(value string) string {
	if value == "" {
		return value
	}
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		decoded = value
	}
	return strings.ReplaceAll(url.QueryEscape(decoded), "+", "%20")
}
