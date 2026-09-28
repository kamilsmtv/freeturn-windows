package vps

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"golang.org/x/crypto/ssh"
)

func TestParseOutputOK(t *testing.T) {
	out := `{"proto":3,"result":"ok","stage":"probe","data":{"installed":true,"running":true,` +
		`"version":"v4.0.1","method":"docker","config":{"backend":"new","listen_port":56000,` +
		`"mode":"udp","obf_profile":"rtpopus3","obf_key":"ab","wg_port":51820}},"logs":["готово"]}`

	r := ParseOutput(out)
	if !r.OK() || r.Stage != "probe" {
		t.Fatalf("ответ разобран неверно: %+v", r)
	}
	var d ProbeData
	if err := r.Decode(&d); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !d.Installed || d.Version != "v4.0.1" || d.Method != "docker" {
		t.Errorf("данные probe разобраны неверно: %+v", d)
	}
	if d.Config.ListenPort != 56000 || d.Config.ObfProfile != "rtpopus3" || d.Config.WGPort != 51820 {
		t.Errorf("параметры сервера разобраны неверно: %+v", d.Config)
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

func TestApplyArgs(t *testing.T) {
	o := ApplyOptions{
		Host: "203.0.113.10", ListenPort: 56000, Mode: profile.ModeUDP,
		ObfProfile: profile.ObfRtpOpus3, ObfKey: strings.Repeat("ab", 32),
	}
	got := strings.Join(ApplyArgs(o), " ")

	for _, want := range []string{
		"apply", "--backend=new", "--host=203.0.113.10", "--listen-port=56000",
		"--mode=udp", "--obf-profile=rtpopus3", "--obf-key=",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("в %q нет %q", got, want)
		}
	}
	if strings.Contains(got, "--kcp-") {
		t.Error("в режиме udp профиль ARQ передавать нельзя")
	}
	if strings.Contains(got, "--connect=") {
		t.Error("своему бэкенду адрес чужого VPN не нужен")
	}
}

// Режим проброса tcp живёт только поверх чужого бэкенда, и адрес его
// обязателен: без --connect серверу некуда отдавать трафик.
func TestApplyArgsExternalBackend(t *testing.T) {
	o := ApplyOptions{
		Backend: BackendExternal, Connect: "127.0.0.1:443", Mode: profile.ModeTCP,
		KCP: profile.MobileKCP(), KCPCustom: true,
	}
	got := strings.Join(ApplyArgs(o), " ")
	for _, want := range []string{"--backend=external", "--connect=127.0.0.1:443", "--mode=tcp",
		"--kcp-interval=40", "--kcp-acknodelay=false"} {
		if !strings.Contains(got, want) {
			t.Errorf("в %q нет %q", got, want)
		}
	}

	o.Mode = profile.ModeUDP
	if strings.Contains(strings.Join(ApplyArgs(o), " "), "--kcp-") {
		t.Error("профиль ARQ уехал в udp-режим")
	}
}

func TestApplyArgsWithoutObf(t *testing.T) {
	got := strings.Join(ApplyArgs(ApplyOptions{ListenPort: 56000, ObfProfile: profile.ObfNone}), " ")
	if strings.Contains(got, "--obf-key") {
		t.Errorf("выключенной обфускации ключ не нужен: %q", got)
	}
	if !strings.Contains(got, "--obf-profile=none") {
		t.Errorf("профиль обфускации должен быть передан явно: %q", got)
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

// Сервер сам заводит доступ, генерирует Client ID и собирает ссылку -
// от нас нужно только имя.
func TestClientListDecode(t *testing.T) {
	out := `{"proto":3,"result":"ok","stage":"client_list","data":{"clients":[` +
		`{"name":"owner","client_id":"aa","self":true,"ip":"10.66.0.2","pub":"K1","hs":1750000000},` +
		`{"name":"phone","client_id":"bb","self":false,"ip":"10.66.0.3","pub":"K2","hs":0}],` +
		`"share":{"backend":"new","host":"203.0.113.10","port":56000,"mode":"udp",` +
		`"obf_profile":"rtpopus3","obf_key":"cc"}},"logs":[]}`

	var d ClientListData
	if err := ParseOutput(out).Decode(&d); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(d.Clients) != 2 {
		t.Fatalf("доступов разобрано %d", len(d.Clients))
	}
	if !d.Clients[0].Self || d.Clients[0].Name != OwnerName {
		t.Errorf("владелец не распознан: %+v", d.Clients[0])
	}
	if d.Clients[1].HS != 0 || d.Clients[1].IP != "10.66.0.3" {
		t.Errorf("доступ гостя разобран неверно: %+v", d.Clients[1])
	}
	if d.Share.Port != 56000 || d.Share.ObfProfile != "rtpopus3" {
		t.Errorf("параметры раздачи разобраны неверно: %+v", d.Share)
	}
}

// Готовую ссылку и конфигурацию сервер отдаёт прямо в ответе client-add.
func TestClientAddDecode(t *testing.T) {
	out := `{"proto":3,"result":"ok","stage":"client_add","data":{` +
		`"client":{"name":"phone","client_id":"bb","ip":"10.66.0.3"},` +
		`"conf_b64":"W0ludGVyZmFjZV0=","link":"freeturn://eyJ2IjoxfQ"},"logs":[]}`

	var d ClientData
	if err := ParseOutput(out).Decode(&d); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if d.Client.Name != "phone" || d.Link == "" {
		t.Fatalf("доступ разобран неверно: %+v", d)
	}
	if DecodeBase64(d.ConfB64) != "[Interface]" {
		t.Errorf("конфигурация не развернулась: %q", d.ConfB64)
	}
}

// Журнал в протоколе 3 приходит массивом строк, а не одним base64-полем.
func TestLogsDecode(t *testing.T) {
	out := `{"proto":3,"result":"ok","stage":"logs","data":{"lines":["первая","вторая"]},"logs":[]}`
	var d LogsData
	if err := ParseOutput(out).Decode(&d); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(d.Lines) != 2 || d.Lines[1] != "вторая" {
		t.Errorf("журнал разобран неверно: %+v", d.Lines)
	}
}

// Команды протокола 2 новый скрипт не знает, и объяснение должно вести к
// переустановке, а не к загадочному "неверный аргумент".
func TestExplainOldServer(t *testing.T) {
	text := Explain(Response{Result: "err", Code: "bad_arg", Msg: "unknown command: install"})
	if !strings.Contains(text, "старой версией install.sh") {
		t.Errorf("нет подсказки про устаревший сервер: %s", text)
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

// Чужая версия протокола означает, что поля data мы поняли бы неверно:
// такой ответ обязан отвергаться, а не разбираться наугад.
func TestCheckProto(t *testing.T) {
	if err := checkProto(Response{Proto: ProtoVersion, Result: "ok"}); err != nil {
		t.Errorf("своя версия отвергнута: %v", err)
	}
	// Транспортные ошибки поля proto не несут.
	if err := checkProto(Response{Result: "err", Code: "transport"}); err != nil {
		t.Errorf("ответ без версии отвергнут: %v", err)
	}

	old := checkProto(Response{Proto: 2, Result: "ok"})
	if old == nil || !strings.Contains(old.Error(), "старой версией") {
		t.Errorf("старый сервер не распознан: %v", old)
	}
	newer := checkProto(Response{Proto: 99, Result: "ok"})
	if newer == nil || !strings.Contains(newer.Error(), "обновите FreeTurn") {
		t.Errorf("новый сервер не распознан: %v", newer)
	}
}

// Имя доступа уходит в имена файлов на сервере, и правила там жёсткие.
// Проверяем до отправки: иначе скрипт ответит bad_arg, а тот по коду
// объясняется как «версии разошлись» - и причина теряется.
func TestValidClientName(t *testing.T) {
	ok := []string{"phone", "Kamil-PC", "nb_2026", "a", strings.Repeat("x", 32)}
	for _, name := range ok {
		if !ValidClientName(name) {
			t.Errorf("имя %q отвергнуто", name)
		}
	}
	bad := []string{"", "Телефон", "phone Камиля", "with space", "-starts", ".dot",
		strings.Repeat("x", 33), "semi;colon", "../etc"}
	for _, name := range bad {
		if ValidClientName(name) {
			t.Errorf("имя %q принято", name)
		}
	}
}

// Значения бэкенда живут в двух местах: в профиле (что выбрал человек) и
// в протоколе (что понимает скрипт). Разойдясь, они молча переключили бы
// сервер не туда, поэтому равенство закрепляем тестом.
func TestBackendConstantsMatchProfile(t *testing.T) {
	if BackendNew != profile.ServerBackendNew {
		t.Errorf("new: протокол %q, профиль %q", BackendNew, profile.ServerBackendNew)
	}
	if BackendExternal != profile.ServerBackendExternal {
		t.Errorf("external: протокол %q, профиль %q", BackendExternal, profile.ServerBackendExternal)
	}
}

// Молчащий сервер не должен держать интерфейс вечно: сервер, который
// принял соединение и замолчал, - обычное дело (файрвол, перегрузка), а
// поле Timeout пакета ssh на этот путь не распространяется.
func TestDialTimesOutOnSilentServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("слушатель: %v", err)
	}
	defer func() { _ = ln.Close() }()
	// Принимаем и молчим: рукопожатие не начнётся никогда.
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer func() { _ = c.Close() }()
		}
	}()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	n, _ := strconv.Atoi(port)
	cfg := profile.SSH{IP: host, Port: n, Username: "root",
		Password: "x", AuthType: profile.AuthPassword, HostFingerprint: "SHA256:x"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	_, err = dial(ctx, cfg)
	spent := time.Since(start)

	if err == nil {
		t.Fatal("молчащий сервер должен давать ошибку, а не висеть")
	}
	if spent > 5*time.Second {
		t.Errorf("рукопожатие висело %s, а предел команды был 2 с", spent)
	}
	if !strings.Contains(err.Error(), "не ответил") {
		t.Errorf("невнятная причина: %v", err)
	}
	if !strings.Contains(err.Error(), "подождите") {
		t.Errorf("нет подсказки, что делать: %v", err)
	}
}

// Установка тянет пакеты минутами, а чтение состояния - секунды: один
// предел на всё либо рвал бы установку, либо не спасал от зависания.
func TestCommandTimeout(t *testing.T) {
	if got := commandTimeout([]string{"apply"}); got != applyTimeout {
		t.Errorf("apply: %s", got)
	}
	if got := commandTimeout([]string{"probe"}); got != defaultTimeout {
		t.Errorf("probe: %s", got)
	}
	if got := commandTimeout(nil); got != defaultTimeout {
		t.Errorf("без команды: %s", got)
	}
}

func TestTimeoutMessage(t *testing.T) {
	if msg := TimeoutMessage(context.DeadlineExceeded); !strings.Contains(msg, "не ответил") {
		t.Errorf("нет объяснения: %q", msg)
	}
	if msg := TimeoutMessage(errors.New("другое")); msg != "" {
		t.Errorf("чужая ошибка объяснена как таймаут: %q", msg)
	}
}
