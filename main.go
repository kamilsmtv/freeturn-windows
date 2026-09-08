// Команда freeturn - десктопный клиент ядра free-turn-proxy для Windows.
package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/kamilsmtv/freeturn-windows/internal/paths"
	"github.com/kamilsmtv/freeturn-windows/internal/settings"
	"github.com/kamilsmtv/freeturn-windows/internal/singleton"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
)

// version подставляется при сборке: -ldflags "-X main.version=..."
var version = "dev"

// instanceName - имя объектов ядра для признака единственного экземпляра.
const instanceName = "FreeTurn.SingleInstance"

// Репозитории для проверки обновлений (ядро и сам GUI).
const (
	CoreRepo = "samosvalishe/free-turn-proxy"
	GUIRepo  = "kamilsmtv/freeturn-windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// guiLogger пишет журнал самого окна в %APPDATA%\FreeTurn\logs\gui.log.
// Без него отказ WebView2 виден только как пустое окно.
func guiLogger() logger.Logger {
	dir, err := paths.Logs()
	if err != nil {
		return logger.NewDefaultLogger()
	}
	path := filepath.Join(dir, "gui.log")
	// Файл открывается на дозапись; недоступен - молча остаёмся без файла.
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		_ = f.Close()
		return logger.NewFileLogger(path)
	}
	return logger.NewDefaultLogger()
}

func main() {
	// Вся работа - в run: main только печатает причину выхода, чтобы
	// отложенные освобождения ресурсов успели отработать.
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

func run() error {
	// Вторая копия подралась бы с первой за локальный порт ядра и повесила
	// бы в трей второй значок, поэтому она лишь будит первую и выходит.
	instance, ok := singleton.Acquire(instanceName)
	if !ok {
		_ = singleton.Signal(instanceName)
		return nil
	}
	defer instance.Release()

	store, err := settings.Open()
	if err != nil {
		return fmt.Errorf("не удалось прочитать настройки: %w", err)
	}
	app, err := NewApp(store)
	if err != nil {
		return fmt.Errorf("не удалось подготовить приложение: %w", err)
	}

	// Рабочий каталог WebView2 - только в %LOCALAPPDATA%: рядом с .exe его
	// класть нельзя (см. paths.WebView2).
	webviewDir, err := paths.WebView2()
	if err != nil {
		return fmt.Errorf("не удалось подготовить каталог WebView2: %w", err)
	}

	// Повторный запуск ярлыка возвращает окно уже работающей копии.
	instance.OnSecondLaunch(app.ShowWindow)

	if err := wails.Run(appOptions(store, app, webviewDir)); err != nil {
		return fmt.Errorf("не удалось запустить окно: %w", err)
	}
	return nil
}
