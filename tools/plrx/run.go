package main

import (
	"os"
	"os/exec"
)

// cmdRun 构建并运行当前实例 (等价 go run .)。
func cmdRun(_ []string) error {
	cmd := exec.Command("go", "run", ".")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
