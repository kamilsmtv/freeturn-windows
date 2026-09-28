//go:build !windows

package socks

import (
	"errors"
	"syscall"
)

func isAddrInUse(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }

func isRefused(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }

func isUnreachable(err error) bool {
	return errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH)
}
