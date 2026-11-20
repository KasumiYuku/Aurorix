package templates

import "embed"

//go:embed builtin/markdown/*.md
var builtinFS embed.FS

func init() {
	if err := RegisterFS("", builtinFS, "builtin/markdown"); err != nil {
		panic(err)
	}
}
