package utils

import "strings"

// FilterAt 仅剥离首部的 @token, 保留剩余原文排版(换行/缩进不折叠)。
func FilterAt(raw string) string {
	if strings.HasPrefix(raw, "<@") {
		if i := strings.IndexByte(raw, '>'); i >= 0 {
			return strings.TrimLeft(raw[i+1:], " \t\n")
		}
	}
	if strings.HasPrefix(raw, "@") {
		if i := strings.IndexAny(raw, " \t\n"); i >= 0 {
			return strings.TrimLeft(raw[i:], " \t\n")
		}
		return ""
	}
	return raw
}
