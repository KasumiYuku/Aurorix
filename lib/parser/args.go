package parser

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/alecthomas/kong"
)

// ParseArgs 用声明式结构解析剩余词元。
func ParseArgs(name string, args any, tokens []string) (any, string, error) {
	t := reflect.TypeOf(args)
	if t.Kind() != reflect.Ptr || t.Elem().Kind() != reflect.Struct {
		return nil, "", fmt.Errorf("args 必须是结构体指针")
	}
	inst := reflect.New(t.Elem()).Interface()

	var buf bytes.Buffer
	p, err := kong.New(inst,
		kong.Name(name),
		kong.Exit(func(int) {}),
		kong.Writers(&buf, &buf),
		kong.UsageOnError(),
	)
	if err != nil {
		return nil, "", err
	}
	ctx, err := p.Parse(tokens)
	if err != nil {
		usage := buf.String()
		if usage == "" && ctx != nil {
			ctx.PrintUsage(false)
			usage = buf.String()
		}
		return nil, usage, err
	}
	_ = ctx
	return inst, "", nil
}
