// Package errs переводит сообщения ядра в понятные пользователю причины.
//
// Правила собраны по docs/troubleshooting.md ядра и по строкам логов из
// internal/session, internal/proxy/udprelay и internal/provider/vk.
package errs

import (
	"fmt"
	"strings"
)

type rule struct {
	// any - срабатывает, если встретилась любая из подстрок.
	any []string
	// all - дополнительное условие: все подстроки должны присутствовать.
	all  []string
	text string
}

// Порядок важен: более частные правила стоят раньше общих.
var rules = []rule{
	{
		any:  []string{"address already in use", "bind: Только одно использование", "Only one usage of each socket address"},
		text: "Локальный порт занят: его уже слушает другой процесс (часто - вторая копия ядра). Освободите порт или укажите другой в поле «Локальный адрес».",
	},
	{
		any:  []string{"route manager disabled", "route manager:"},
		all:  []string{"denied"},
		text: "Не удалось добавить маршруты к TURN-серверам: нужны права администратора. Перезапустите FreeTurn от имени администратора.",
	},
	{
		any:  []string{"route manager disabled"},
		text: "Маршруты к TURN-серверам не добавлены, весь трафик может уйти в туннель. Проверьте права администратора или отключите «Автоматические маршруты».",
	},
	{
		any:  []string{"no TURN candidates", "all TURN candidates failed", "connect to TURN server"},
		text: "TURN-сервер недоступен: провайдер не отдал ни одного рабочего кандидата. Попробуйте другой транспорт (TCP вместо UDP) или уменьшите число потоков.",
	},
	{
		any:  []string{"get TURN creds", "vk provider", "Fatal provider error"},
		text: "Не удалось получить TURN-данные у VK. Обычно это значит, что ссылка на звонок истекла или звонок завершён - создайте новую ссылку VK Calls и не закрывайте звонок.",
	},
	{
		any:  []string{"vk: no links configured"},
		text: "Не указана ссылка на звонок VK Calls - без неё клиент не получит TURN-данные.",
	},
	{
		any:  []string{"captcha"},
		text: "VK требует captcha. Включите «Ручная captcha» в дополнительных параметрах, решите её в браузере и запуститесь снова.",
	},
	{
		any:  []string{"failed to connect DTLS", "failed to write client ID"},
		text: "Сервер не принял соединение. Проверьте, что на VPS запущено ядро, совпадают -mode и профиль обфускации, а client-id добавлен в allowlist.",
	},
	{
		any:  []string{"OBF init failed", "OBF unwrap failed", "OBF wrap failed"},
		text: "Обфускация не сошлась: профиль и ключ обязаны совпадать на клиенте и сервере.",
	},
	{
		any:  []string{"resolve peer addr", "no such host", "lookup"},
		text: "Не удалось разрешить адрес сервера. Проверьте адрес и настройки DNS (поля «Режим DNS» и «Свои DNS»).",
	},
	{
		any:  []string{"kcp"},
		all:  []string{"udp"},
		text: "Параметры KCP заданы в режиме udp - ядро это запрещает. Переключите режим на tcp или верните KCP к стандартным значениям.",
	},
	{
		any:  []string{"flag provided but not defined", "unknown subcommand"},
		text: "Версия ядра не понимает переданный флаг. Обновите ядро на вкладке обновлений.",
	},
	{
		any:  []string{"peer is required", "peer:"},
		text: "Не задан адрес сервера (peer).",
	},
}

// Explain выбирает причину падения по хвосту журнала. Если ничего не
// подошло, возвращает последнюю содержательную строку и код выхода.
func Explain(lines []string, waitErr error) string {
	joined := strings.ToLower(strings.Join(lines, "\n"))
	for _, r := range rules {
		if !matchAny(joined, r.any) {
			continue
		}
		if len(r.all) > 0 && !matchAll(joined, r.all) {
			continue
		}
		return r.text
	}
	if last := lastMeaningful(lines); last != "" {
		return last
	}
	if waitErr != nil {
		return fmt.Sprintf("ядро завершилось с ошибкой: %v", waitErr)
	}
	return "ядро неожиданно завершилось"
}

// ExplainLine объясняет одну строку; "" - правила не сработали.
func ExplainLine(line string) string {
	low := strings.ToLower(line)
	for _, r := range rules {
		if matchAny(low, r.any) && (len(r.all) == 0 || matchAll(low, r.all)) {
			return r.text
		}
	}
	return ""
}

func matchAny(hay string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(hay, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

func matchAll(hay string, needles []string) bool {
	for _, n := range needles {
		if !strings.Contains(hay, strings.ToLower(n)) {
			return false
		}
	}
	return true
}

// lastMeaningful возвращает последнюю строку уровня ERROR/fatal.
func lastMeaningful(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.Contains(l, "[ERROR]") || strings.Contains(strings.ToLower(l), "fatal") {
			return l
		}
	}
	return ""
}
