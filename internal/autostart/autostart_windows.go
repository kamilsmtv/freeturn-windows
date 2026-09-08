package autostart

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// schtasks запускает планировщик без консольного окна и возвращает вывод.
func schtasks(args ...string) (string, error) {
	cmd := exec.Command("schtasks.exe", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func enabled() bool {
	_, err := schtasks("/Query", "/TN", TaskName)
	return err == nil
}

func enable() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить путь к приложению: %w", err)
	}

	// /RL HIGHEST - задача стартует с правами администратора без запроса UAC.
	// /F - перезаписать задачу, если она уже была.
	out, err := schtasks("/Create", "/TN", TaskName, "/TR", `"`+exe+`"`,
		"/SC", "ONLOGON", "/RL", "HIGHEST", "/F")
	if err != nil {
		return fmt.Errorf("не удалось включить автозапуск: %s", firstLine(out))
	}
	return nil
}

func disable() error {
	out, err := schtasks("/Delete", "/TN", TaskName, "/F")
	if err != nil {
		// Задачи и так нет - это не ошибка.
		if strings.Contains(strings.ToLower(out), "cannot find") || !enabled() {
			return nil
		}
		return errors.New("не удалось выключить автозапуск: " + firstLine(out))
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i > 0 {
		return s[:i]
	}
	if s == "" {
		return "планировщик задач не ответил"
	}
	return s
}
