package templates

import (
	"html"
	"io/fs"
)

// HTML 模板与 markdown 模板共用同一份注册表和填充逻辑, 注册方式、命名空间优先级、
// 占位符语法({{name}} / {{#each}})完全一致, 区别只有文件后缀(.html)与渲染口径。
//
// 唯一要留意的是转义: 引擎按原样插入值, 不会替你转义。值来自用户输入(群名、昵称、
// 邮箱等)而结果要进浏览器时, 先过 EscapeHTML, 否则对方能把标签写进页面。
// 另外 {{inline}} 是 markdown 专用标签, 在 HTML 模板里会渲染成 markdown 链接, 别用。

// RegisterHTMLFS 把 FS 内 dir 子树下的 *.html 注册到命名空间, 文件名即模板 ID。
// 插件用 //go:embed 自带模板时, 在 init 里这样注册:
//
//	//go:embed templates/html/*.html
//	var htmlFS embed.FS
//
//	templates.RegisterHTMLFS(pluginId, htmlFS, "templates/html")
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
