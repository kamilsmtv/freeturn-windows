//go:build !windows

package routes

import "errors"

// Вне Windows управление маршрутами не нужно: сборка под другие ОС
// существует ради тестов.
func defaultGateway() (string, error) {
	return "", errors.New("поддерживается только в Windows")
}
func addRoute(string, string) error {
	return errors.New("поддерживается только в Windows")
}
func delRoute(string) error { return nil }
