package message

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	mdImageRe      = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)
	mdCmdTagRe     = regexp.MustCompile(`<qqbot-cmd-(?:input|enter)\b[^>]*>`)
	mdFenceRe      = regexp.MustCompile("(?m)^[ \t]*```.*$")
	mdCodeRe       = regexp.MustCompile("`([^`\\n]*)`")
	mdHeadingRe    = regexp.MustCompile(`(?m)^[ \t]*#{1,6}[ \t]+`)
	mdQuoteRe      = regexp.MustCompile(`(?m)^[ \t]*>[ \t]?`)
	mdBoldRe       = regexp.MustCompile(`\*\*|__`)
	mdLatexColorRe = regexp.MustCompile(`\\color\s*\{[^}]*\}`)
	mdLatexCmdRe   = regexp.MustCompile(`\\[a-zA-Z]+\s*\{`)
	mdLatexSpaceRe = regexp.MustCompile(`\\[;,:!>]|\\q?quad`)
	mdLatexBareRe  = regexp.MustCompile(`\\[a-zA-Z]+`)
	mdLatexRe      = regexp.MustCompile(`\${1,2}[^$]+\${1,2}`)
	mdLatexBraceRe = regexp.MustCompile(`[${}]`)
	mdLinkRe       = regexp.MustCompile(`\[([^\]]*)\]\([^)\s]+\)`)
	mdBareURLRe    = regexp.MustCompile(`https?://\S+`)
	mdHRRe         = regexp.MustCompile(`(?m)^[ \t]*(?:-{3,}|\*{3,}|_{3,})[ \t]*$`)
)

const guildTextLimit = 2000

// SplitImages 把正文里的 markdown 图片摘出来, 返回图片地址与剩下的正文; 频道一条消息只能带一张图, 所以正文里的图必须变成独立部件。地址不筛形状, 交给 ImageMessage 归一化。
func SplitImages(content string) (urls []string, rest string) {
	if !strings.Contains(content, "![") {
		return nil, content
	}
	urls = make([]string, 0, 2)
	rest = mdImageRe.ReplaceAllStringFunc(content, func(tag string) string {
		m := mdImageRe.FindStringSubmatch(tag)
		if len(m) < 2 {
			return ""
		}
		urls = append(urls, m[1])
		return ""
	})
	return urls, rest
}

// GuildText 把 markdown 正文压成频道能读的纯文本: 只去装饰不删字, 最后 tidyGuildLines 收敛剥壳留下的行首尾空格与空行。
func GuildText(content string) string {
	if content == "" {
		return ""
	}
	content = renderCmdTags(content)
	content = strings.ReplaceAll(content, "<qqbot-at-everyone />", "@全体成员")
	content = mdFenceRe.ReplaceAllString(content, "")
	content = mdCodeRe.ReplaceAllString(content, "$1")
	content = mdHeadingRe.ReplaceAllString(content, "")
	content = mdQuoteRe.ReplaceAllString(content, "")
	content = mdBoldRe.ReplaceAllString(content, "")
	content = stripLatex(content)
	content = mdImageRe.ReplaceAllString(content, "")
	content = mdLinkRe.ReplaceAllString(content, "$1")
	content = mdBareURLRe.ReplaceAllString(content, "")
	content = mdHRRe.ReplaceAllString(content, "")
	return truncateRunes(tidyGuildLines(content), guildTextLimit)
}

func tidyGuildLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || isDecorative(line) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func isDecorative(line string) bool {
	for _, r := range line {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func renderCmdTags(s string) string {
	if !strings.Contains(s, "<qqbot-cmd-") {
		return s
	}
	return mdCmdTagRe.ReplaceAllStringFunc(s, func(tag string) string {
		text := cmdAttr(tag, "text")
		show := cmdAttr(tag, "show")
		if strings.HasPrefix(show, "/") {
			return show
		}
		if text != "" {
			return text
		}
		return show
	})
}

func cmdAttr(tag, name string) string {
	key := name + "=\""
	i := strings.Index(tag, key)
	if i < 0 {
		return ""
	}
	rest := tag[i+len(key):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		return ""
	}
	if v, err := url.QueryUnescape(rest[:j]); err == nil {
		return v
	}
	return rest[:j]
}

func stripLatex(s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	return mdLatexRe.ReplaceAllStringFunc(s, func(frag string) string {
		frag = mdLatexColorRe.ReplaceAllString(frag, "")
		for {
			next := mdLatexCmdRe.ReplaceAllString(frag, "{")
			if next == frag {
				break
			}
			frag = next
		}
		frag = mdLatexSpaceRe.ReplaceAllString(frag, " ")
		frag = mdLatexBareRe.ReplaceAllString(frag, "")
		frag = mdLatexBraceRe.ReplaceAllString(frag, "")
		return strings.Join(strings.Fields(frag), " ")
	})
}

func truncateRunes(s string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
