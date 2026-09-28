package socks

import (
	"errors"

	"golang.org/x/sys/windows"
)

// Коды сокетных ошибок Windows - это WSA-коды, а syscall.EADDRINUSE там
// значит совсем другое число. Сравнивать же тексты нельзя: на русской
// Windows они переведены, и проверка молча перестаёт срабатывать.
func isAddrInUse(err error) bool { return errors.Is(err, windows.WSAEADDRINUSE) }

func isRefused(err error) bool { return errors.Is(err, windows.WSAECONNREFUSED) }

func isUnreachable(err error) bool {
	return errors.Is(err, windows.WSAEHOSTUNREACH) || errors.Is(err, windows.WSAENETUNREACH)
}
