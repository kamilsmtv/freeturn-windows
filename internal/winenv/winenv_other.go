//go:build !windows

package winenv

const isWindows = false

// IsAdmin вне Windows всегда false: сборка под другие ОС нужна только
// для тестов, боевой цели у неё нет.
func IsAdmin() bool { return false }

func webView2Version() string { return "" }
