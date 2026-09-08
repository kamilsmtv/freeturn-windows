package routes

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// defaultGateway читает шлюз по умолчанию из таблицы маршрутов.
// Способ тот же, что у ядра (internal/routemgr/route_windows.go).
func defaultGateway() (string, error) {
	out, err := run("route", "print", "0.0.0.0")
	if err != nil {
		return "", fmt.Errorf("не удалось прочитать таблицу маршрутов: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(strings.TrimSpace(scanner.Text()))
		if len(fields) >= 3 && fields[0] == "0.0.0.0" && fields[1] == "0.0.0.0" {
			return fields[2], nil
		}
	}
	return "", fmt.Errorf("шлюз по умолчанию не найден")
}

// addRoute пиннит адрес на физический шлюз с низкой метрикой, чтобы он
// выигрывал у маршрута туннеля.
func addRoute(ip, gateway string) error {
	if _, err := run("route", "add", ip, "mask", "255.255.255.255", gateway, "metric", "1"); err != nil {
		return err
	}
	return nil
}

func delRoute(ip string) error {
	_, err := run("route", "delete", ip)
	return err
}

// run выполняет команду route без консольного окна.
func run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Output()
}
