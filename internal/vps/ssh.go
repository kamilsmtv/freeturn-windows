package vps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"golang.org/x/crypto/ssh"
)

// ErrUnknownHost - отпечаток ключа сервера ещё не подтверждён.
//
// Первое подключение всегда требует подтверждения: молча доверять любому
// ключу - значит согласиться на подмену сервера по дороге. Отпечаток
// показывается пользователю и после согласия сохраняется в профиле.
type ErrUnknownHost struct {
	Fingerprint string
	Host        string
}

func (e *ErrUnknownHost) Error() string {
	return fmt.Sprintf("сервер %s предъявил неизвестный ключ %s - подтвердите отпечаток", e.Host, e.Fingerprint)
}

// ErrHostKeyChanged - ключ сервера отличается от сохранённого.
type ErrHostKeyChanged struct {
	Expected string
	Actual   string
}

func (e *ErrHostKeyChanged) Error() string {
	return fmt.Sprintf("ключ сервера изменился: ожидался %s, получен %s. "+
		"Это либо переустановка сервера, либо подмена соединения", e.Expected, e.Actual)
}

// dial подключается по SSH с проверкой отпечатка.
func dial(ctx context.Context, cfg profile.SSH) (*ssh.Client, error) {
	auth, err := authMethods(cfg)
	if err != nil {
		return nil, err
	}

	host := strings.TrimSpace(cfg.IP)
	if host == "" {
		return nil, errors.New("не задан адрес сервера")
	}
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	clientCfg := &ssh.ClientConfig{
		User:            orDefault(cfg.Username, "root"),
		Auth:            auth,
		HostKeyCallback: hostKeyCallback(cfg.HostFingerprint, addr),
		Timeout:         15 * time.Second,
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("не удалось подключиться к %s: %w", addr, err)
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, clientCfg)
	if err != nil {
		_ = conn.Close()
		// Ошибки проверки ключа пробрасываем как есть: UI покажет отпечаток.
		var unknown *ErrUnknownHost
		var changed *ErrHostKeyChanged
		if errors.As(err, &unknown) {
			return nil, unknown
		}
		if errors.As(err, &changed) {
			return nil, changed
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, errors.New("сервер отверг учётные данные: проверьте пользователя, пароль или ключ")
		}
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

func hostKeyCallback(expected, addr string) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		actual := ssh.FingerprintSHA256(key)
		switch {
		case expected == "":
			return &ErrUnknownHost{Fingerprint: actual, Host: addr}
		case expected != actual:
			return &ErrHostKeyChanged{Expected: expected, Actual: actual}
		default:
			return nil
		}
	}
}

func authMethods(cfg profile.SSH) ([]ssh.AuthMethod, error) {
	if cfg.AuthType == profile.AuthSSHKey {
		key := strings.TrimSpace(cfg.SSHKey)
		if key == "" {
			return nil, errors.New("не задан приватный ключ SSH")
		}
		signer, err := ssh.ParsePrivateKey([]byte(key))
		if err != nil {
			return nil, fmt.Errorf("не удалось прочитать приватный ключ: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}

	if cfg.Password == "" {
		return nil, errors.New("не задан пароль SSH")
	}
	// keyboard-interactive нужен серверам, у которых выключен PasswordAuth.
	answer := func(_, _ string, questions []string, _ []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range answers {
			answers[i] = cfg.Password
		}
		return answers, nil
	}
	return []ssh.AuthMethod{
		ssh.Password(cfg.Password),
		ssh.KeyboardInteractive(answer),
	}, nil
}

// runScript передаёт install.sh на stdin bash и возвращает его вывод.
//
// Способ эскалации до root берётся из профиля (см. runAsRoot).
func runScript(ctx context.Context, client *ssh.Client, cfg profile.SSH, script string, args []string) (stdout, stderr string, err error) {
	password := sudoPassword(cfg)
	out, errText, err := runAsRoot(ctx, client, cfg, "bash -s -- "+quoteArgs(args), script)
	out, err = asOutput(out, errText, err)
	// Подстраховка: пароль не должен попасть в журнал ни при каком раскладе.
	return redact(out, password), redact(errText, password), err
}

// runAsRoot выполняет команду с повышением прав по правилам профиля.
//
// Тонкость с паролем: кеш учётных данных sudo привязан к сессии, а каждая
// команда по SSH - это своя сессия, поэтому подтвердить пароль заранее и
// запустить команду через sudo -n нельзя. Значит пароль идёт вместе со
// скриптом, в одной сессии. Но сперва проверяем, нужен ли он вообще: если
// sudo пускает без пароля, не отправляем его - лишняя строка досталась бы
// bash и ушла в stderr.
func runAsRoot(ctx context.Context, client *ssh.Client, cfg profile.SSH, command, input string) (stdout, stderr string, err error) {
	switch cfg.RootMode {
	case profile.RootSudoNoPass:
		command = "sudo -n " + command

	case profile.RootSudoPass:
		password := sudoPassword(cfg)
		if password == "" {
			return "", "", errors.New("для sudo нужен пароль: заполните его в доступе к серверу")
		}
		if sudoWithoutPassword(ctx, client) {
			command = "sudo -n " + command
		} else {
			// -S читает пароль со stdin, -p '' убирает приглашение из вывода.
			command = "sudo -S -p '' " + command
			input = password + "\n" + input
		}
	}
	return runCommand(ctx, client, command, input)
}

// sudoWithoutPassword проверяет, пускает ли sudo без пароля.
//
// Ответ определяется кодом возврата: «sudo: a password is required» - это
// ненулевой код, а не пустой вывод.
func sudoWithoutPassword(ctx context.Context, client *ssh.Client) bool {
	_, _, err := runCommand(ctx, client, "sudo -n true", "")
	return err == nil
}

// runCommand выполняет команду, подавая input на stdin.
func runCommand(ctx context.Context, client *ssh.Client, command, input string) (stdout, stderr string, err error) {
	session, err := client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer func() { _ = session.Close() }()

	var outBuf, errBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf
	session.Stdin = strings.NewReader(input)

	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return outBuf.String(), errBuf.String(), ctx.Err()
	case err := <-done:
		// Код возврата команды отдаём как есть: по нему проверяется,
		// например, пускает ли sudo без пароля.
		return outBuf.String(), errBuf.String(), err
	}
}

// asOutput приводит результат команды к виду, который ждёт разбор ответа:
// не дошедший JSON заменяется строкой ERROR с тем, что сказал sudo или bash.
func asOutput(stdout, stderr string, err error) (string, error) {
	if err == nil {
		return stdout, nil
	}
	// Ошибка запуска самой сессии - не отказ команды, её пробрасываем.
	var exitErr *ssh.ExitError
	var missingErr *ssh.ExitMissingError
	if !errors.As(err, &exitErr) && !errors.As(err, &missingErr) {
		return stdout, err
	}
	if strings.TrimSpace(stdout) != "" {
		return stdout, nil
	}
	return "ERROR: " + strings.TrimSpace(stderr), nil
}

// sudoPassword выбирает пароль для sudo: отдельный, если задан, иначе пароль SSH.
func sudoPassword(cfg profile.SSH) string {
	if cfg.SudoPassword != "" {
		return cfg.SudoPassword
	}
	return cfg.Password
}

// redact убирает пароль из текста, который может попасть в журнал.
func redact(text, password string) string {
	if password == "" {
		return text
	}
	return strings.ReplaceAll(text, password, "<скрыт>")
}

// quoteArgs заворачивает аргументы в одинарные кавычки для bash.
func quoteArgs(args []string) string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
	}
	return strings.Join(out, " ")
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// runShell выполняет произвольный bash-скрипт на сервере с тем же
// повышением прав, что и install.sh, и сохраняет код возврата: по нему
// вызывающая сторона отличает «нечего делать» от настоящего отказа.
func runShell(ctx context.Context, client *ssh.Client, cfg profile.SSH, script string) (stdout, stderr string, err error) {
	out, errText, err := runAsRoot(ctx, client, cfg, "bash -s", script)
	password := sudoPassword(cfg)
	return redact(out, password), redact(errText, password), err
}
