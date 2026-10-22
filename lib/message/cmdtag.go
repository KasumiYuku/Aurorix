package message

import (
	"net/url"
	"regexp"
	"strings"
)

// 平台要求 <qqbot-cmd-input> / <qqbot-cmd-enter> 的 text、show 传 urlencode 后的值, 但它不替开发者编:
// 把原文写进去, 整条 markdown 会被判「指令参数解析失败」, 一个字都发不出来。
// 所以正文出站前统一转一遍 —— 配置里写人话, 框架负责编码。
var (
	cmdTagRe        = regexp.MustCompile(`<qqbot-cmd-(?:input|enter)\b[^>]*>`)
	cmdAttrRe       = regexp.MustCompile(`\b(text|show)="([^"]*)"`)
	percentEscapeRe = regexp.MustCompile(`%[0-9A-Fa-f]{2}`)
)

func encodeCmdTagAttrs(content string) string {
	if !strings.Contains(content, "qqbot-cmd-") {
		return content
	}
	return cmdTagRe.ReplaceAllStringFunc(content, func(tag string) string {
		return cmdAttrRe.ReplaceAllStringFunc(tag, func(attr string) string {
			m := cmdAttrRe.FindStringSubmatch(attr)
			return m[1] + `="` + cmdEscape(m[2]) + `"`
		})
	})
}

// cmdEscape 是 urlencode, 但空格用 %20 而不是 '+' —— 这是 URI 属性值, 不是表单字段。
// 值里已经有 %XX 就认为编过了并原样返回: Prepare 每次发送都会走, 重试不能再编一遍(%2F → %252F)。
func cmdEscape(value string) string {
	if percentEscapeRe.MatchString(value) {
		return value
	}
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}
