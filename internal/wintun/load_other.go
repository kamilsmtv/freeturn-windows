//go:build !windows

package wintun

import "errors"

// Preload вне Windows не нужен: адаптеров Wintun там не бывает.
func Preload(string) error {
	return errors.New("wintun поддерживается только в Windows")
}
