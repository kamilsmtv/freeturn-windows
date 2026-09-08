// Package paths определяет расположение данных приложения.
//
// Контракт: рядом с исполняемым файлом не пишется ничего. Конфиги и логи -
// в %APPDATA%\FreeTurn, ядро и его резервная копия - в %LOCALAPPDATA%\FreeTurn\core.
package paths

import (
	"os"
	"path/filepath"
)

const appDir = "FreeTurn"

// Data - каталог конфигов и логов (%APPDATA%\FreeTurn).
func Data() (string, error) { return ensure(os.UserConfigDir) }

// Core - каталог с бинарём ядра (%LOCALAPPDATA%\FreeTurn\core).
func Core() (string, error) {
	base, err := ensure(os.UserCacheDir)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "core")
	return dir, os.MkdirAll(dir, 0o700)
}

// Logs - каталог логов (%APPDATA%\FreeTurn\logs).
func Logs() (string, error) {
	base, err := Data()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "logs")
	return dir, os.MkdirAll(dir, 0o700)
}

// WebView2 - рабочий каталог движка WebView2.
//
// По умолчанию WebView2 создаёт папку рядом с исполняемым файлом. Это
// нарушает наше правило «ничего не писать рядом с .exe» и просто не
// работает, когда .exe лежит на сетевом пути или в каталоге только для
// чтения: движок молча не стартует, и окно остаётся пустым.
func WebView2() (string, error) {
	base, err := ensure(os.UserCacheDir)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "webview2")
	return dir, os.MkdirAll(dir, 0o700)
}

// Bin - каталог вспомогательных библиотек (%LOCALAPPDATA%\FreeTurn\bin).
func Bin() (string, error) {
	base, err := ensure(os.UserCacheDir)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "bin")
	return dir, os.MkdirAll(dir, 0o700)
}

// ProfilesFile - путь к файлу профилей.
func ProfilesFile() (string, error) {
	base, err := Data()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "profiles.json"), nil
}

// SettingsFile - путь к файлу настроек приложения.
func SettingsFile() (string, error) {
	base, err := Data()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "settings.json"), nil
}

func ensure(base func() (string, error)) (string, error) {
	root, err := base()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, appDir)
	return dir, os.MkdirAll(dir, 0o700)
}
