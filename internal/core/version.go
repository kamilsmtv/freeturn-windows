package core

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// versionRe ловит первую строку лога ядра:
// "[INFO] Free Turn Proxy client version=1.2.3" (cmd/client/main.go).
var versionRe = regexp.MustCompile(`Free Turn Proxy client version=(\S+)`)

// ParseVersionLine достаёт версию из строки лога ядра; "" - строка не про версию.
func ParseVersionLine(line string) string {
	m := versionRe.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// probeArgs - минимальный набор, который проходит валидацию конфига ядра и
// доводит запуск до строки с версией. Отдельного флага -version у клиента нет
// (см. docs/analysis.md), поэтому версию читаем из лога и сразу гасим процесс.
// Адрес заведомо нерабочий: наружу ядро при этом не ходит.
var probeArgs = []string{"-peer", "127.0.0.1:1", "-links", "https://vk.ru/call/join/probe"}

// ErrNoVersion - ядро запустилось, но строку с версией не напечатало.
var ErrNoVersion = errors.New("ядро не сообщило свою версию")

// ProbeVersion запускает бинарь ядра, читает версию из первой строки лога и
// завершает процесс, не дожидаясь установления соединения.
func ProbeVersion(ctx context.Context, exePath string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, exePath, probeArgs...)
	hideWindow(cmd)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	// stdout не нужен, но без него ядро может заблокироваться на записи.
	cmd.Stdout = io.Discard

	if err := cmd.Start(); err != nil {
		return "", err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		if v := ParseVersionLine(scanner.Text()); v != "" {
			return v, nil
		}
	}
	return "", ErrNoVersion
}
