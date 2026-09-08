//go:build !windows

package netstat

// Вне Windows счётчиков адаптера нет: сборка под другие ОС нужна для тестов.
func read(adapter string) Counters {
	return Counters{Adapter: adapter, Reason: "счётчики адаптера доступны только в Windows"}
}
