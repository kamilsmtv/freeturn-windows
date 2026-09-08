package vps

import "strings"

// Коды ошибок install.sh (fail <code> "..."). Список снят с самого скрипта.
var codeText = map[string]string{
	"needs_root":             "нужны права root: выберите способ повышения прав (sudo) в настройках профиля",
	"not_writable":           "не удалось создать каталог /opt/free-turn-proxy - проверьте права и место на диске",
	"unsupported_arch":       "архитектура сервера не поддерживается ядром",
	"version_resolve_failed": "сервер не смог узнать последнюю версию ядра: проверьте доступ к GitHub с VPS",
	"download_failed":        "сервер не смог скачать ядро: проверьте сеть на VPS",
	"checksum_mismatch":      "контрольная сумма скачанного ядра не совпала",
	"listen_port_busy":       "порт, который должен слушать сервер, уже занят другим процессом",
	"bad_arg":                "неверный аргумент команды - похоже, версии приложения и install.sh разошлись",
	"sudo_auth_failed":       "sudo не принял пароль: проверьте пароль для повышения прав и способ входа",
	"sudo_requiretty":        "sudo на сервере требует терминал (requiretty): отключите его или заходите под root",
	"transport":              "сервер не ответил ожидаемым образом - проверьте доступ по SSH",
	"internal":               "сервер прервал команду",
	"wg_missing":             "на сервере нет WireGuard: разрешите установку пакета",
	"lock_busy":              "на сервере уже выполняется другая операция - подождите и повторите",
	"no_backend":             "не задан локальный бэкенд (-connect): укажите порт WireGuard или Xray",
}

// Explain переводит отказ сервера в понятную причину.
//
// Подробности от сервера приходят сразу из трёх мест - msg, журнал команды
// и stderr, - и часто повторяют друг друга. Дубли отсеиваем: читать
// «sudo: a password is required» трижды подряд бессмысленно.
func Explain(r Response) string {
	if r.OK() {
		return ""
	}

	text := codeText[r.Code]
	if text == "" {
		text = "сервер вернул ошибку"
	}

	var details []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		for _, seen := range details {
			if strings.Contains(seen, v) || strings.Contains(v, seen) {
				return
			}
		}
		details = append(details, v)
	}

	add(r.Msg)
	add(lastLog(r.Logs))
	add(lastLog(strings.Split(r.Stderr, "\n")))

	if r.Stage != "" {
		text += ", шаг «" + r.Stage + "»"
	}
	if len(details) > 0 {
		text += ": " + strings.Join(details, "; ")
	}
	// Самая частая причина срыва wg-setup - на сервере нечем сгенерировать
	// ключи интерфейса, а сам скрипт про это не сообщает.
	if r.Code == "internal" && r.Stage == "wg_setup" {
		text += ". Похоже, серверу нечем создать ключи AmneziaWG: нужны wg, awg или docker"
	}
	return text
}

// lastLog возвращает последнюю содержательную строку журнала команды.
func lastLog(logs []string) string {
	for i := len(logs) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(logs[i]); line != "" {
			return line
		}
	}
	return ""
}
