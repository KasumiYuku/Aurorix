package config

import (
	"fmt"
	"regexp"
	"strings"
)

// 平台对 <qqbot-cmd-input> / <qqbot-cmd-enter> 是当 XML 属性解析的:
// 属性必须写成 名字="值", 少一个等号或值整条消息会被拒, 回一句「指令参数解析失败」,
// 而那时整条欢迎语已经发不出去了。所以在保存时就拦下来, 让报错落在写配置的人眼前。
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

// validateCmdTexts 检查一组文案行里的指令标签, 报错带上是第几行。
func validateCmdTexts(key string, lines []string) error {
	for i, line := range lines {
		if err := ValidateCmdTags(key, line); err != nil {
			return fmt.Errorf("第 %d 行: %w", i+1, err)
		}
	}
	return nil
}
