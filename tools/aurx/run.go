package main

import (
	"os"
	"os/exec"
)

func cmdRun(_ []string) error {
	cmd := exec.Command("go", "run", ".")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
