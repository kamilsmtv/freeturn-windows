package core

import (
	"os/exec"
	"syscall"
)

// hideWindow не даёт ядру мигнуть консольным окном.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		// Своя группа процессов нужна, чтобы Ctrl-события GUI не убивали
		// ядро раньше, чем мы снимем маршруты.
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
