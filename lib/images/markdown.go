package images

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	mdImageRe = regexp.MustCompile(`!\[([^\]\n]*)\]\(([^)\n]*)\)`)
	mdDimRe   = regexp.MustCompile(`#\s*([1-9]\d{0,8})\s*px\s*#\s*([1-9]\d{0,8})\s*px`)
)

// ClampMarkdown 按 b 改写正文里每张 markdown 图片的 #宽px #高px; 没标注尺寸的图原样留着。
func ClampMarkdown(content string, b Box) string {
	if !b.On() || !strings.Contains(content, "px") {
		return content
	}
	locs := mdImageRe.FindAllStringSubmatchIndex(content, -1)
	if len(locs) == 0 {
		return content
	}
	var out strings.Builder
	last := 0
	changed := false
	for _, loc := range locs {
		alt, ok := clampAlt(content[loc[2]:loc[3]], b)
		if !ok {
			continue
		}
		if !changed {
			changed = true
			out.Grow(len(content))
		}
		out.WriteString(content[last:loc[2]])
		out.WriteString(alt)
		last = loc[3]
	}
	if !changed {
		return content
	}
	out.WriteString(content[last:])
	return out.String()
}

func clampAlt(alt string, b Box) (string, bool) {
	dims := mdDimRe.FindAllStringSubmatchIndex(alt, -1)
	if len(dims) == 0 {
		return alt, false
	}
	var out strings.Builder
	last := 0
	changed := false
	for _, d := range dims {
		w, errW := strconv.Atoi(alt[d[2]:d[3]])
		h, errH := strconv.Atoi(alt[d[4]:d[5]])
		if errW != nil || errH != nil {
			continue
		}
		fw, fh := b.Limit(w, h)
		if fw == w && fh == h {
			continue
		}
		if !changed {
			changed = true
			out.Grow(len(alt) + 8)
		}
		out.WriteString(alt[last:d[0]])
		out.WriteString("#")
		out.WriteString(strconv.Itoa(fw))
		out.WriteString("px #")
		out.WriteString(strconv.Itoa(fh))
		out.WriteString("px")
		last = d[1]
	}
	if !changed {
		return alt, false
	}
	out.WriteString(alt[last:])
	return out.String(), true
}
