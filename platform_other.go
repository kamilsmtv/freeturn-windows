//go:build !windows

package main

import "errors"

func openInExplorer(string) error {
	return errors.New("открытие каталога поддерживается только в Windows")
}
