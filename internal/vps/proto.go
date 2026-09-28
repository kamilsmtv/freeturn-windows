// Package vps управляет сервером по SSH через install.sh ядра.
//
// У install.sh есть встроенный протокол JSON-RPC (подкоманды probe, apply,
// stop, restart, logs, client-list, client-add, client-conf, client-remove,
// uninstall): на один запуск он печатает ровно один JSON-объект. Тот же
// протокол использует Android-клиент, но он гоняет собственный bundle под
// GPL, а мы берём скрипт самого ядра - см. docs/analysis.md.
//
// Протокол 3 (ядро 4.0) заменил три команды одной: apply ставит, настраивает
// и запускает сервер разом, сам поднимает WireGuard и сам возвращает конфиг
// владельца. Доступы гостям выдаёт client-add - он же генерирует Client ID и
// готовую ссылку freeturn://. Со сборками сервера на протоколе 2 приложение
// не работает: там нет ни apply, ни client-*.
package vps

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ProtoVersion - версия протокола, которую печатает install.sh.
const ProtoVersion = 3

// Response - конверт ответа: либо result=ok с data, либо result=err с кодом.
type Response struct {
	Proto  int             `json:"proto"`
	Result string          `json:"result"`
	Code   string          `json:"code"`
	Msg    string          `json:"msg"`
	Stage  string          `json:"stage"`
	Data   json.RawMessage `json:"data"`
	Logs   []string        `json:"logs"`

	// Stderr - то, что команда написала в поток ошибок. В протоколе его
	// нет, мы добавляем его сами: там оседают сообщения die.
	Stderr string `json:"stderr,omitempty"`
}

// TimeoutMessage объясняет прерванную по времени команду.
//
// Голое "context deadline exceeded" в интерфейсе бесполезно: человеку
// нужно знать, что именно не дождалось и что с этим делать.
func TimeoutMessage(err error) string {
	if !errors.Is(err, context.DeadlineExceeded) {
		return ""
	}
	return "сервер не ответил за отведённое время: проверьте, жив ли он и доступен ли SSH"
}

// checkProto сверяет версию протокола сервера с нашей.
//
// Ответ без поля proto - это наши же транспортные ошибки, их пропускаем.
func checkProto(r Response) error {
	switch {
	case r.Proto == 0 || r.Proto == ProtoVersion:
		return nil
	case r.Proto > ProtoVersion:
		return fmt.Errorf("сервер говорит на протоколе %d, а приложение понимает %d: "+
			"обновите FreeTurn", r.Proto, ProtoVersion)
	default:
		return fmt.Errorf("сервер установлен старой версией install.sh (протокол %d, нужен %d): "+
			"нажмите «Установить и запустить» - она развернёт сервер заново", r.Proto, ProtoVersion)
	}
}

// OK сообщает, что команда отработала успешно.
func (r Response) OK() bool { return r.Result == "ok" }

// Decode разбирает поле data в типизированную структуру.
func (r Response) Decode(v any) error {
	if len(r.Data) == 0 {
		return nil
	}
	return json.Unmarshal(r.Data, v)
}

// ParseOutput разбирает stdout команды.
//
// Перед нашим единственным JSON-объектом могут оказаться баннер входа,
// вывод .bashrc или остаток от sudo, поэтому берём последнюю строку,
// начинающуюся с '{' - так же поступает Android-клиент.
func ParseOutput(raw string) Response {
	text := strings.TrimSpace(raw)
	if text == "" {
		return errResponse("internal", "сервер ничего не ответил")
	}

	lines := strings.Split(text, "\n")
	if first := strings.TrimSpace(lines[0]); strings.HasPrefix(first, "ERROR:") {
		msg := strings.TrimSpace(strings.TrimPrefix(text, "ERROR:"))
		return errResponse(transportCode(msg), msg)
	}

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var r Response
		if err := json.Unmarshal([]byte(line), &r); err == nil {
			return r
		}
		return errResponse("internal", "непонятный ответ сервера: "+truncate(line, 200))
	}
	return errResponse(transportCode(text), truncate(text, 300))
}

// transportCode распознаёт типовые отказы транспорта до появления JSON.
func transportCode(msg string) string {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "sudo") && strings.Contains(low, "password"):
		return "sudo_auth_failed"
	case strings.Contains(low, "a terminal is required"), strings.Contains(low, "requiretty"):
		return "sudo_requiretty"
	default:
		return "transport"
	}
}

func errResponse(code, msg string) Response {
	return Response{Proto: ProtoVersion, Result: "err", Code: code, Msg: msg}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// DecodeBase64 разворачивает поле *_b64; пустое или битое - пустая строка.
func DecodeBase64(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(data)
}

// Типизированные пейлоады команд (поле data). Имена ключей - контракт с
// install.sh, см. cmd_probe и соседние функции.

// ProbeData - что сервер сообщает о себе.
type ProbeData struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	EUID      int    `json:"euid"`
	Arch      string `json:"arch"`
	// Method и Version заполняются только на установленном сервере.
	Method  string `json:"method"`
	Version string `json:"version"`
	// Config - содержимое install.conf: с этими параметрами сервер работает.
	Config ConfigData `json:"config"`
}

// ConfigData - параметры установленного сервера (config_json в install.sh).
type ConfigData struct {
	Method     string `json:"method"`
	Backend    string `json:"backend"`
	Connect    string `json:"connect"`
	Host       string `json:"host"`
	ListenPort int    `json:"listen_port"`
	WGPort     int    `json:"wg_port"`
	WGNet      string `json:"wg_net"`
	Mode       string `json:"mode"`
	ObfProfile string `json:"obf_profile"`
	ObfKey     string `json:"obf_key"`
	Version    string `json:"version"`
}

// Цели uninstall: только ядро, только наш WireGuard или всё сразу.
const (
	TargetFreeTurn = "freeturn"
	TargetWG       = "wg"
	TargetAll      = "all"
)

// OwnerName - имя доступа владельца сервера (OWNER в install.sh).
const OwnerName = "owner"

// clientNameRe повторяет valid_name из install.sh. Имя попадает в имена
// файлов на сервере, поэтому набор символов там узкий, а кириллицы нет.
var clientNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)

// ValidClientName проверяет имя доступа до отправки на сервер.
//
// Иначе скрипт отвечает bad_arg, а его объяснение по коду - «версии
// разошлись», что уводит совсем не туда.
func ValidClientName(name string) bool { return clientNameRe.MatchString(name) }

// Значения backend: свой WireGuard скрипта или чужой VPN пользователя.
const (
	BackendNew      = "new"
	BackendExternal = "external"
)

// ApplyData - результат apply: доступ владельца и признак перезапуска.
type ApplyData struct {
	Owner  ClientRef `json:"owner"`
	ObfKey string    `json:"obf_key"`
	// NeedsRestart сообщает, что параметры раздачи изменились и выданные
	// раньше конфигурации клиентов устарели.
	NeedsRestart bool `json:"needs_restart"`
}

// ClientRef - выданный сервером доступ: его Client ID, ключ и конфигурация.
type ClientRef struct {
	Name     string `json:"name"`
	ClientID string `json:"client_id"`
	Pub      string `json:"pub"`
	IP       string `json:"ip"`
	// Self отмечает доступ владельца сервера.
	Self bool `json:"self"`
	// HS - время последнего рукопожатия WireGuard, unix-секунды; 0 - не было.
	HS int64 `json:"hs"`
	// ConfB64 и Link приходят только в apply, client-add и client-conf.
	ConfB64 string `json:"conf_b64"`
	Link    string `json:"link"`
}

// ClientData - ответ client-add и client-conf.
type ClientData struct {
	Client  ClientRef `json:"client"`
	ConfB64 string    `json:"conf_b64"`
	Link    string    `json:"link"`
}

// ClientListData - список доступов и параметры раздачи.
type ClientListData struct {
	Clients []ClientRef `json:"clients"`
	Share   ShareData   `json:"share"`
}

// ShareData - параметры, которые сервер раздаёт клиентам.
type ShareData struct {
	Backend    string `json:"backend"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Mode       string `json:"mode"`
	ObfProfile string `json:"obf_profile"`
	ObfKey     string `json:"obf_key"`
}

// LogsData - хвост журнала сервера. В протоколе 3 строки приходят
// массивом, а не одним полем base64.
type LogsData struct {
	Lines []string `json:"lines"`
}
