//go:build !windows

package core

import "os/exec"

// hideWindow вне Windows не нужен: сборка под другие ОС существует ради тестов.
func hideWindow(*exec.Cmd) {}
