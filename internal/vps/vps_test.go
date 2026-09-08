package vps

import (
	"errors"
	"strings"
	"testing"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"golang.org/x/crypto/ssh"
)

func TestParseOutputOK(t *testing.T) {
	out := `{"proto":2,"result":"ok","stage":"probe","data":{"installed":true,"version":"v1.4.2","running":true,"wg":{"present":true,"port":51820}},"logs":["готово"]}`

	r := ParseOutput(out)
	if !r.OK() || r.Stage != "probe" {
		t.Fatalf("ответ разобран неверно: %+v", r)
	}
	var d ProbeData
	if err := r.Decode(&d); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !d.Installed || d.Version != "v1.4.2" || !d.WG.Present || d.WG.Port != 51820 {
		t.Errorf("данные probe разобраны неверно: %+v", d)
	}
	if len(r.Logs) != 1 {
		t.Errorf("журнал команды потерян: %+v", r.Logs)
	}
}

func TestParseOutputSkipsBanner(t *testing.T) {
	// Перед JSON часто идут баннер входа и вывод .bashrc.
	out := "Welcome to Ubuntu 24.04\nLast login: today\n" +
		`{"proto":2,"result":"err","code":"needs_root","msg":"root required"}` + "\n"

	r := ParseOutput(out)
	if r.OK() || r.Code != "needs_root" {
		t.Fatalf("ошибка разобрана неверно: %+v", r)
	}
	if !strings.Contains(Explain(r), "root") {
		t.Errorf("пояснение не про права: %q", Explain(r))
	}
}

func TestParseOutputTransportErrors(t *testing.T) {
	tests := map[string]string{
		"ERROR: sudo: a password is required": "sudo_auth_failed",
		"ERROR: sudo: a terminal is required": "sudo_requiretty",
		"ERROR: bash: command not found":      "transport",
		"вообще не json":                      "transport",
		"":                                    "internal",
	}
	for out, want := range tests {
		if got := ParseOutput(out); got.Code != want {
			t.Errorf("ParseOutput(%q).Code = %q, want %q", out, got.Code, want)
		}
	}
}

func TestParseOutputBrokenJSON(t *testing.T) {
	r := ParseOutput(`{"proto":2,"result":`)
	if r.OK() || r.Code != "internal" {
		t.Errorf("обрезанный JSON должен давать внутреннюю ошибку: %+v", r)
	}
}

func TestDecodeBase64(t *testing.T) {
	if got := DecodeBase64("W0ludGVyZmFjZV0="); got != "[Interface]" {
		t.Errorf("DecodeBase64 = %q", got)
	}
	if got := DecodeBase64("не base64"); got != "" {
		t.Errorf("битое значение должно давать пустую строку, получено %q", got)
	}
}

func TestStartArgs(t *testing.T) {
	o := InstallOptions{
		Listen: "0.0.0.0:56000", Connect: "127.0.0.1:51820", Mode: profile.ModeUDP,
		ObfProfile: profile.ObfRtpOpus3, ObfKey: strings.Repeat("ab", 32), ObfTimingMs: 20,
		ClientID: strings.Repeat("0", 32),
	}
	got := strings.Join(StartArgs(o), " ")

	for _, want := range []string{
		"start", "--listen=0.0.0.0:56000", "--connect=127.0.0.1:51820",
		"--mode=udp", "--obf-profile=rtpopus3", "--obf-timing=20ms", "--client-id=",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("в %q нет %q", got, want)
		}
	}
	if strings.Contains(got, "--kcp-") {
		t.Error("в режиме udp профиль ARQ передавать нельзя")
	}
}

func TestStartArgsKCPOnlyForTCP(t *testing.T) {
	o := InstallOptions{
		Listen: "0.0.0.0:56000", Connect: "127.0.0.1:443", Mode: profile.ModeTCP,
		KCP: profile.MobileKCP(), KCPCustom: true,
	}
	got := strings.Join(StartArgs(o), " ")
	for _, want := range []string{"--mode=tcp", "--kcp-interval=40", "--kcp-acknodelay=false"} {
		if !strings.Contains(got, want) {
			t.Errorf("в %q нет %q", got, want)
		}
	}

	o.Mode = profile.ModeUDP
	if strings.Contains(strings.Join(StartArgs(o), " "), "--kcp-") {
		t.Error("профиль ARQ уехал в udp-режим")
	}
}

func TestStartArgsWithoutObf(t *testing.T) {
	got := strings.Join(StartArgs(InstallOptions{
		Listen: "0.0.0.0:56000", Connect: "127.0.0.1:51820", ObfProfile: profile.ObfNone,
	}), " ")
	if strings.Contains(got, "--obf-") {
		t.Errorf("выключенная обфускация не должна давать аргументов: %q", got)
	}
}

func TestQuoteArgs(t *testing.T) {
	got := quoteArgs([]string{"start", "--name=про'бел"})
	if got != `'start' '--name=про'\''бел'` {
		t.Errorf("экранирование кавычек сломано: %s", got)
	}
}

func TestExplainUnknownCode(t *testing.T) {
	r := Response{Result: "err", Code: "что-то новое", Msg: "подробности"}
	if got := Explain(r); !strings.Contains(got, "подробности") {
		t.Errorf("для незнакомого кода ожидалось сообщение сервера, получено %q", got)
	}
}

// Подробности приходят из msg, журнала и stderr и часто дублируют друг
// друга: читать одно и то же трижды подряд бессмысленно.
func TestExplainDropsDuplicateDetails(t *testing.T) {
	r := Response{
		Result: "err", Code: "sudo_auth_failed",
		Msg:    "sudo: a password is required",
		Logs:   []string{"sudo: a password is required"},
		Stderr: "sudo: a password is required",
	}
	got := Explain(r)
	if strings.Count(got, "a password is required") != 1 {
		t.Errorf("подробность должна встречаться один раз: %q", got)
	}
	if !strings.Contains(got, "sudo не принял пароль") {
		t.Errorf("причина потеряна: %q", got)
	}
}

// Ключи и адрес клиента выдаёт сервер, поэтому peer-add обязан получить
// имя и endpoint: без них install.sh отвечает bad_arg.
func TestPeerAddArgs(t *testing.T) {
	got := strings.Join(PeerAddArgs(PeerOptions{
		NameB64:  "d2luZG93cw==",
		Endpoint: "127.0.0.1:9000",
		ClientID: strings.Repeat("a", 32),
		DNS:      "1.1.1.1",
	}), " ")

	for _, want := range []string{
		"peer-add", "--name-b64=d2luZG93cw==", "--endpoint=127.0.0.1:9000",
		"--client-id=", "--dns=1.1.1.1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("в %q нет %q", got, want)
		}
	}
}

func TestPeerAddArgsDefaultsEndpoint(t *testing.T) {
	got := strings.Join(PeerAddArgs(PeerOptions{NameB64: "eA=="}), " ")
	if !strings.Contains(got, "--endpoint="+profile.DefaultListen) {
		t.Errorf("без endpoint должен подставляться локальный сокет ядра: %q", got)
	}
	if strings.Contains(got, "--dns=") || strings.Contains(got, "--client-id=") {
		t.Errorf("пустые параметры не должны попадать в команду: %q", got)
	}
}

// "unexpected exit 1" сам по себе ничего не объясняет: в ответе есть шаг и
// журнал команды, и они должны попадать в текст ошибки.
func TestExplainIncludesStageAndLog(t *testing.T) {
	r := Response{
		Result: "err", Code: "internal", Msg: "unexpected exit 1", Stage: "peer_add",
		Logs: []string{"init", "awg0.conf not found"},
	}
	got := Explain(r)

	for _, want := range []string{"прервал команду", "unexpected exit 1", "peer_add", "awg0.conf not found"} {
		if !strings.Contains(got, want) {
			t.Errorf("в %q нет %q", got, want)
		}
	}
}

// die в install.sh пишет причину в stderr, а не в logs: без неё пользователь
// видит только "unexpected exit 1".
func TestExplainFallsBackToStderr(t *testing.T) {
	r := Response{
		Result: "err", Code: "internal", Msg: "unexpected exit 1", Stage: "peer_add",
		Stderr: "warning: ignored\nНе удалось сгенерировать ключи VPN.",
	}
	if got := Explain(r); !strings.Contains(got, "Не удалось сгенерировать ключи VPN.") {
		t.Errorf("сообщение из stderr потеряно: %q", got)
	}
}

// Заводить пиров можно только на AmneziaWG: обычный WireGuard тоже даёт
// wg_backend=true, но peer-add на нём срывается.
func TestCanAddPeers(t *testing.T) {
	if (ShareInfoData{WGBackend: true}).CanAddPeers() {
		t.Error("обычный WireGuard не годится для peer-add")
	}
	if !(ShareInfoData{WGBackend: true, BackendType: BackendAWG}).CanAddPeers() {
		t.Error("на AmneziaWG пиров заводить можно")
	}
}

func TestPort(t *testing.T) {
	tests := map[string]int{
		"0.0.0.0:56293":   56293,
		"127.0.0.1:51471": 51471,
		"1.2.3.4":         0,
		"1.2.3.4:0":       0,
		"1.2.3.4:70000":   0,
		"1.2.3.4:порт":    0,
	}
	for addr, want := range tests {
		if got := Port(addr); got != want {
			t.Errorf("Port(%q) = %d, want %d", addr, got, want)
		}
	}
}

// Пароль sudo не должен попадать в журнал и в текст ошибки: если sudo его
// не спросит, лишняя строка уходит в stderr.
func TestRedactHidesPassword(t *testing.T) {
	const pass = "s3cret-пароль"
	text := "bash: line 1: " + pass + ": command not found"

	got := redact(text, pass)
	if strings.Contains(got, pass) {
		t.Fatalf("пароль остался в тексте: %q", got)
	}
	if !strings.Contains(got, "<скрыт>") {
		t.Errorf("ожидалась замена на заглушку: %q", got)
	}
	if redact("обычный текст", "") != "обычный текст" {
		t.Error("пустой пароль не должен менять текст")
	}
}

func TestSudoPasswordFallsBackToSSH(t *testing.T) {
	if got := sudoPassword(profile.SSH{Password: "ssh"}); got != "ssh" {
		t.Errorf("без отдельного пароля sudo берётся пароль SSH, получено %q", got)
	}
	if got := sudoPassword(profile.SSH{Password: "ssh", SudoPassword: "sudo"}); got != "sudo" {
		t.Errorf("отдельный пароль sudo важнее, получено %q", got)
	}
}

// Отказ команды и сбой самой сессии - разные вещи: по коду возврата
// проверяется, например, пускает ли sudo без пароля.
func TestAsOutput(t *testing.T) {
	// Команда отработала успешно.
	if out, err := asOutput("{\"result\":\"ok\"}", "", nil); err != nil || out != "{\"result\":\"ok\"}" {
		t.Errorf("успешный вызов изменён: %q, %v", out, err)
	}

	// Команда завершилась с ненулевым кодом и ничего не напечатала.
	exit := &ssh.ExitError{Waitmsg: ssh.Waitmsg{}}
	out, err := asOutput("", "sudo: a password is required", exit)
	if err != nil {
		t.Errorf("отказ команды не должен быть ошибкой вызова: %v", err)
	}
	if !strings.HasPrefix(out, "ERROR: ") || !strings.Contains(out, "password is required") {
		t.Errorf("причина отказа потеряна: %q", out)
	}

	// Ненулевой код, но JSON всё же дошёл - он и важен.
	if out, err := asOutput(`{"result":"err"}`, "шум", exit); err != nil || out != `{"result":"err"}` {
		t.Errorf("ответ сервера должен доходить: %q, %v", out, err)
	}

	// Сбой самой сессии - настоящая ошибка.
	if _, err := asOutput("", "", errors.New("connection lost")); err == nil {
		t.Error("сбой сессии должен пробрасываться как ошибка")
	}
}

func TestExplainWGSetupHint(t *testing.T) {
	text := Explain(Response{
		Result: "err",
		Code:   "internal",
		Stage:  "wg_setup",
		Msg:    "unexpected exit 1",
	})
	if !strings.Contains(text, "ключи AmneziaWG") {
		t.Fatalf("нет подсказки про ключи: %s", text)
	}
}

func TestClassifyWGTools(t *testing.T) {
	if installed, err := classifyWGTools("present\n", "", nil); installed || err != nil {
		t.Fatalf("готовый сервер: installed=%v err=%v", installed, err)
	}
	if installed, err := classifyWGTools("installed\n", "", nil); !installed || err != nil {
		t.Fatalf("установка пакета: installed=%v err=%v", installed, err)
	}
	// Вывод sudo может приехать вместе с меткой - это не должно мешать.
	_, err := classifyWGTools("", "sudo: unable to resolve host\nno-package-manager\n", errors.New("exit 2"))
	if err == nil || !strings.Contains(err.Error(), "вручную") {
		t.Fatalf("нераспознанный пакетный менеджер: %v", err)
	}
	_, err = classifyWGTools("", "docker-image-missing", errors.New("exit 4"))
	if err == nil || !strings.Contains(err.Error(), "образ AmneziaWG") {
		t.Fatalf("недоступный образ: %v", err)
	}
	_, err = classifyWGTools("", "install-failed", errors.New("exit 3"))
	if err == nil || !strings.Contains(err.Error(), "wireguard-tools") {
		t.Fatalf("неудачная установка: %v", err)
	}
	// Пустой ответ - тоже отказ, но объяснимый.
	_, err = classifyWGTools("", "", nil)
	if err == nil || !strings.Contains(err.Error(), "ничего не ответил") {
		t.Fatalf("пустой ответ: %v", err)
	}
}
