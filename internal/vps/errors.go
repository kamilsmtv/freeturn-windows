package vps

import "strings"

// Коды ошибок install.sh (fail <code> "..."). Список снят с самого скрипта.
var codeText = map[string]string{
	"needs_root":        "нужны права root: выберите способ повышения прав (sudo) в настройках профиля",
	"not_installed":     "сервер ещё не установлен - нажмите «Установить и запустить»",
	"not_found":         "такого доступа на сервере нет",
	"exists":            "доступ с таким именем на сервере уже есть",
	"owner_protected":   "доступ владельца удалить нельзя",
	"unsupported_arch":  "архитектура сервера не поддерживается ядром",
	"download_failed":   "сервер не смог скачать ядро: проверьте сеть и доступ к GitHub с VPS",
	"port_busy":         "порт, который должен слушать сервер, уже занят другим процессом",
	"bad_arg":           "неверный аргумент команды - похоже, версии приложения и install.sh разошлись",
	"host_unknown":      "сервер не смог определить свой внешний адрес: укажите его вручную",
	"start_failed":      "сервер не запустился - посмотрите журнал",
	"docker_failed":     "не удалось запустить контейнер: проверьте docker на сервере",
	"no_systemd":        "на сервере нет systemd: выберите запуск через docker",
	"wg_install_failed": "не удалось поставить wireguard-tools и iptables",
	"wg_unsupported":    "ядро сервера не умеет WireGuard: выберите свой VPN (external)",
	"wg_conflict":       "на сервере уже есть чужой WireGuard с таким именем интерфейса",
	"backend_locked":    "на сервере ещё стоит поднятый нами WireGuard: сначала удалите его кнопкой «Удалить наш WireGuard с сервера»",
	"subnet_conflict":   "подсеть WireGuard пересекается с уже существующей на сервере",
	"subnet_full":       "в подсети WireGuard кончились адреса",
	"sudo_auth_failed":  "sudo не принял пароль: проверьте пароль для повышения прав и способ входа",
	"sudo_requiretty":   "sudo на сервере требует терминал (requiretty): отключите его или заходите под root",
	"transport":         "сервер не ответил ожидаемым образом - проверьте доступ по SSH",
	"internal":          "сервер прервал команду",
	"lock_busy":         "на сервере уже выполняется другая операция - подождите и повторите",
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
	// Команды протокола 2 новый скрипт не знает: это не «неверный
	// аргумент», а разошедшиеся версии, и лечится обновлением сервера.
	if r.Code == "bad_arg" && strings.Contains(r.Msg, "unknown command") {
		text = "сервер установлен старой версией install.sh: обновите его " +
			"кнопкой «Установить и запустить» - она развернёт сервер заново"
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
