package main

import (
	"os/exec"
	"syscall"
)

// openInExplorer открывает каталог в проводнике. explorer.exe возвращает
// ненулевой код даже при успехе, поэтому его статус игнорируем.
func openInExplorer(dir string) error {
	cmd := exec.Command("explorer.exe", dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
	return nil
}
