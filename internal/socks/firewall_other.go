//go:build !windows

package socks

// Вне Windows брандмауэром не управляем: сборка под другие ОС нужна
// только для тестов.
func OpenPort(int) error { return nil }
func ClosePort()         {}
