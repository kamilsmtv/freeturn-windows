// Package autostart включает и выключает запуск FreeTurn при входе в систему.
//
// Обычный способ - ключ реестра Run - здесь не годится: приложение требует
// прав администратора (манифест requireAdministrator), и запуск из Run
// упрётся в запрос UAC, которого при входе в систему никто не увидит.
// Поэтому заводим задачу в планировщике с флагом «с наивысшими правами»:
// она стартует без запроса.
package autostart

// TaskName - имя задачи в планировщике.
const TaskName = "FreeTurnAutostart"

// Enabled сообщает, включён ли автозапуск.
func Enabled() bool { return enabled() }

// Enable создаёт задачу автозапуска для текущего исполняемого файла.
func Enable() error { return enable() }

// Disable удаляет задачу автозапуска.
func Disable() error { return disable() }

// Apply приводит автозапуск к нужному состоянию.
func Apply(want bool) error {
	if want == Enabled() {
		return nil
	}
	if want {
		return Enable()
	}
	return Disable()
}
