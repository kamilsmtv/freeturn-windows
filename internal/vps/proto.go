// Package vps управляет сервером по SSH через install.sh ядра.
//
// У install.sh есть встроенный протокол JSON-RPC v2 (подкоманды probe,
// install, wg-setup, start, stop, logs, peer-*, client-*, uninstall): на
// один запуск он печатает ровно один JSON-объект. Тот же протокол
// использует Android-клиент, но он гоняет собственный bundle под GPL, а мы
// берём скрипт самого ядра - см. docs/analysis.md.
package vps

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// ProtoVersion - версия протокола, которую печатает install.sh.
const ProtoVersion = 2

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
	Installed bool      `json:"installed"`
	Version   string    `json:"version"`
	BinSHA256 string    `json:"bin_sha256"`
	Running   bool      `json:"running"`
	Mode      string    `json:"mode"`
	Obf       string    `json:"obf"`
	Runtime   string    `json:"runtime"`
	EUID      int       `json:"euid"`
	WG        WGInfo    `json:"wg"`
	Virt      string    `json:"virt"`
	WGKernel  bool      `json:"wg_kernel"`
	Conflicts Conflicts `json:"conflicts"`
}

// WGInfo - состояние WireGuard на сервере.
type WGInfo struct {
	Present bool `json:"present"`
	Port    int  `json:"port"`
}

// Conflicts - что на сервере может помешать работе.
type Conflicts struct {
	WARP        bool     `json:"warp"`
	X3UI        bool     `json:"x3ui"`
	WGEasy      bool     `json:"wgeasy"`
	Tailscale   bool     `json:"tailscale"`
	OtherIfaces []string `json:"other_ifaces"`
}

// InstallData - результат установки ядра.
type InstallData struct {
	Stage        string `json:"stage"`
	Bin          string `json:"bin"`
	Version      string `json:"version"`
	Runtime      string `json:"runtime"`
	NeedsRestart bool   `json:"needs_restart"`
}

// WGSetupData - результат настройки WireGuard.
type WGSetupData struct {
	WG            WGSetupInfo `json:"wg"`
	ClientConfB64 string      `json:"client_conf_b64"`
}

// WGSetupInfo - порт поднятого интерфейса.
type WGSetupInfo struct {
	Port    int  `json:"port"`
	Existed bool `json:"existed"`
}

// ShareInfoData - параметры, которые владелец раздаёт клиентам.
type ShareInfoData struct {
	// WGBackend означает «VPN-бэкенд наш», но не уточняет какой: он
	// истинен и для обычного WireGuard с нашей меткой.
	WGBackend bool `json:"wg_backend"`
	// BackendType сервер заполняет значением "awg", только когда поднята
	// именно AmneziaWG. Заводить пиров (peer-add) можно лишь в этом случае:
	// команда дописывает [Peer] в конфигурацию AmneziaWG.
	BackendType string `json:"backend_type"`
	Mode        string `json:"mode"`
	ObfProfile  string `json:"obf_profile"`
	ObfKey      string `json:"obf_key"`
}

// BackendAWG - значение BackendType для AmneziaWG.
const BackendAWG = "awg"

// CanAddPeers сообщает, готов ли сервер выдавать конфигурации клиентов.
func (d ShareInfoData) CanAddPeers() bool { return d.BackendType == BackendAWG }

// PeerAddData - добавленный пир WireGuard вместе с конфигом клиента.
type PeerAddData struct {
	Peer          PeerRef `json:"peer"`
	ClientID      string  `json:"client_id"`
	ClientConfB64 string  `json:"client_conf_b64"`
}

// PeerRef - публичный ключ и адрес пира.
type PeerRef struct {
	Pub string `json:"pub"`
	IP  string `json:"ip"`
}

// LogsData - хвост журнала сервера.
type LogsData struct {
	LogB64 string `json:"log_b64"`
	Lines  int    `json:"lines"`
}
