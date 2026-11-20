package buttons

import (
	"strings"
	"sync/atomic"
)

// KeyboardStyle 是 keyboard.content.style, 作用于整块按钮板(字号/尺寸), 与单个按钮的 ButtonStyle 不是一回事。平台对未知字段静默丢弃, 字段名或位置写错不会有任何提示。
type KeyboardStyle struct {
	// FontSize 字号。留空表示不传, 由客户端用默认大小。
	FontSize string `json:"font_size,omitempty"`
}

const (
	// FontSizeSmall 小号按钮。目前唯一实测生效的取值。
	FontSizeSmall = "small"
)

var defaultFontSize atomic.Pointer[func() string]

// SetDefaultFontSize 注入全局缺省字号来源, 传 nil 关闭; 只填没自己设过字号的键盘。
func SetDefaultFontSize(source func() string) {
	if source == nil {
		defaultFontSize.Store(nil)
		return
	}
	defaultFontSize.Store(&source)
}

func currentDefaultFontSize() string {
	p := defaultFontSize.Load()
	if p == nil {
		return ""
	}
	return normalizeFontSize((*p)())
}

func normalizeFontSize(size string) string {
	return strings.ToLower(strings.TrimSpace(size))
}

func (k *Keyboard) applyDefaultFontSize() {
	if k.fontSizeSet {
		return
	}
	size := currentDefaultFontSize()
	if size == "" {
		return
	}
	if k.Style == nil {
		k.Style = &KeyboardStyle{}
	}
	k.Style.FontSize = size
}

// SetFontSize 给整块键盘设字号; 传空串表示显式要客户端默认大小, 不吃全局缺省。
func (k *Keyboard) SetFontSize(size string) *Keyboard {
	k.fontSizeSet = true
	size = normalizeFontSize(size)
	if size == "" {
		k.Style = nil
		return k
	}
	if k.Style == nil {
		k.Style = &KeyboardStyle{}
	}
	k.Style.FontSize = size
	return k
}
