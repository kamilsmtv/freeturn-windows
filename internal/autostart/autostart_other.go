//go:build !windows

package autostart

import "errors"

var errUnsupported = errors.New("автозапуск поддерживается только в Windows")

func enabled() bool  { return false }
func enable() error  { return errUnsupported }
func disable() error { return nil }
