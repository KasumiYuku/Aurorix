package message

import (
	"net/url"
	"regexp"
	"strings"
)

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

func cmdEscape(value string) string {
	if percentEscapeRe.MatchString(value) {
		return value
	}
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}
