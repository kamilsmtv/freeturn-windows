package socks

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// ruleName - имя правила в брандмауэре. По нему же оно и удаляется,
// поэтому имя обязано быть стабильным между запусками: иначе после
// аварийного завершения в системе копились бы осиротевшие правила.
const ruleName = "FreeTurn SOCKS5"

// OpenPort разрешает входящие подключения на порт.
//
// Без этого Windows молча отбрасывает пакеты соседей, и со стороны это
// выглядит как неработающее приложение, а не как закрытый брандмауэр.
// Профиль public исключён намеренно: в кафе и отелях сеть считается
// публичной, и открывать там порт по умолчанию мы не станем.
func OpenPort(port int) error {
	ClosePort()
	_, err := netsh("advfirewall", "firewall", "add", "rule",
		"name="+ruleName,
		"dir=in", "action=allow", "protocol=TCP",
		"localport="+strconv.Itoa(port),
		"profile=private,domain",
	)
	if err != nil {
		return fmt.Errorf("не удалось открыть порт %d в брандмауэре: %w", port, err)
	}
	return nil
}

// ClosePort убирает правило. Отсутствие правила ошибкой не считается.
func ClosePort() {
	_, _ = netsh("advfirewall", "firewall", "delete", "rule", "name="+ruleName)
}

// netsh выполняет команду без консольного окна и с внятной ошибкой:
// сам netsh пишет причину в stdout, а не в stderr.
func netsh(args ...string) ([]byte, error) {
	cmd := exec.Command("netsh", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return out, err
		}
		return out, fmt.Errorf("%s", msg)
	}
	return out, nil
}
