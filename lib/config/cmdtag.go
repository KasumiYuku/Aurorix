package config

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	cmdTagRe  = regexp.MustCompile(`<qqbot-cmd-(input|enter)\b([^>]*)>`)
	cmdAttrRe = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z0-9_-]*)="([^"]*)"`)
)

// ValidateCmdTags 检查一段 markdown 里的指令标签写法; 没问题返回 nil。
func ValidateCmdTags(key, text string) error {
	for i, m := range cmdTagRe.FindAllStringSubmatch(text, -1) {
		if err := checkCmdTag(key, m[1], i+1, m[2]); err != nil {
			return err
		}
	}
	return nil
}

func checkCmdTag(key, kind string, index int, body string) error {
	body = strings.TrimSuffix(strings.TrimSpace(body), "/")
	seen := make(map[string]bool, 4)
	for body = strings.TrimSpace(body); body != ""; body = strings.TrimSpace(body) {
		m := cmdAttrRe.FindStringSubmatch(body)
		if m == nil {
			bad := body
			if cut := strings.IndexAny(body, "=\t \n"); cut > 0 {
				bad = body[:cut]
			}
			return fmt.Errorf(`%s 第 %d 个 <qqbot-cmd-%s> 里的 %q 不是 名字="值" 的形式`, key, index, kind, bad)
		}
		seen[m[1]] = true
		body = body[len(m[0]):]
	}
	if !seen["text"] {
		return fmt.Errorf(`%s 第 %d 个 <qqbot-cmd-%s> 缺 text 属性`, key, index, kind)
	}
	return nil
}

func validateCmdTexts(key string, lines []string) error {
	for i, line := range lines {
		if err := ValidateCmdTags(key, line); err != nil {
			return fmt.Errorf("第 %d 行: %w", i+1, err)
		}
	}
	return nil
}
