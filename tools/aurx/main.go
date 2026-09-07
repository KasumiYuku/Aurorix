// Command aurx 实例脚手架: new 建工程, add 接入插件, new-plugin 建插件, run 构建运行。
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "new":
		err = cmdNew(os.Args[2:])
	case "add":
		err = cmdAdd(os.Args[2:])
	case "new-plugin":
		err = cmdNewPlugin(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "aurx:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`aurx - Aurorix 实例脚手架

用法:
  aurx new <目录> [--framework 框架路径]   创建实例工程 (默认 replace ../Aurorix)
  aurx add <module路径>                    接入插件 (官方/第三方/实例内部)
  aurx new-plugin <插件名>                 在实例生成插件骨架
  aurx run                                 构建并运行当前实例
`)
	os.Exit(0)
}
