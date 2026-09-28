package vps

import "strings"

// Port возвращает порт из адреса вида host:port; 0 - разобрать не удалось.
func Port(addr string) int {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return 0
	}
	n := 0
	for _, r := range addr[idx+1:] {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	if n < 1 || n > 65535 {
		return 0
	}
	return n
}
