package templates

import (
	"html"
	"io/fs"
)

// RegisterHTMLFS 把 FS 内 dir 子树下的 *.html 注册到命名空间, 文件名即模板 ID; 插件在 init 里传自己的 embed.FS 即可。
func RegisterHTMLFS(ns string, fsys fs.FS, dir string) error {
	return registerFS(kindHTML, ns, fsys, dir)
}

// NewHTMLTemplate 注册单个 HTML 模板到全局命名空间。
func NewHTMLTemplate(Id string, Template string) {
	register(kindHTML, "", Id, Template)
}

// IsHTMLTemplateExist 任意命名空间是否存在该 HTML 模板。
func IsHTMLTemplateExist(Id string) bool {
	return exists(kindHTML, Id)
}

// FillHTMLTemplate 全局命名空间填充 HTML 模板并校验是否仍有未填充项。
func FillHTMLTemplate(Id string, arg Args) (string, error) {
	return fillFor(kindHTML, "", Id, arg)
}

// FillHTMLFor 插件命名空间优先填充 HTML 模板, 未命中回落全局。
func FillHTMLFor(ns, Id string, arg Args) (string, error) {
	return fillFor(kindHTML, ns, Id, arg)
}

// GetHTMLTemplateCount 全部命名空间的 HTML 模板总数。
func GetHTMLTemplateCount() uint {
	return count(kindHTML)
}

// EscapeHTML 转义 & < > " ', 供 HTML 模板的不可信值使用。
func EscapeHTML(s string) string {
	return html.EscapeString(s)
}
