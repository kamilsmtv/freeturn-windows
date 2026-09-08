//go:build !windows

package netstat

// Вне Windows список DNS не нужен: сборка под другие ОС существует для тестов.
func systemDNS(string) []string { return nil }
