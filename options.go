package main

import (
	"github.com/kamilsmtv/freeturn-windows/internal/settings"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// appOptions собирает настройки окна.
//
// Вынесено из main отдельной функцией не ради красоты: забытый хук здесь
// ничего не ломает при сборке, приложение просто молча теряет поведение
// (так уже случалось с OnShutdown и HideWindowOnClose). Теперь набор
// хуков проверяется тестом.
func appOptions(store *settings.Store, app *App, webviewDir string) *options.App {
	s := store.Get()

	return &options.App{
		Title:                    "FreeTurn",
		Width:                    1100,
		Height:                   760,
		MinWidth:                 900,
		MinHeight:                600,
		StartHidden:              s.StartMinimized,
		AssetServer:              &assetserver.Options{Assets: assets},
		Bind:                     []any{app},
		EnableDefaultContextMenu: false,

		OnStartup:     app.startup,
		OnShutdown:    app.shutdown,
		OnBeforeClose: app.beforeClose,

		// Штатный для Wails способ держать приложение в трее: обработчик
		// крестика проверяет этот флаг раньше, чем зовёт OnBeforeClose.
		// Значение читается один раз при старте, поэтому смену настройки
		// на ходу дополнительно отрабатывает OnBeforeClose.
		HideWindowOnClose: s.MinimizeToTray,

		Logger:   guiLogger(),
		LogLevel: logger.INFO,

		// Фон окна виден, пока страница не отрисовалась. Нулевое значение -
		// чёрный, из-за чего любая заминка выглядит как поломка.
		BackgroundColour: &options.RGBA{R: 244, G: 244, B: 245, A: 255},

		Windows: &windows.Options{
			// Тему окна ведём сами: системная по умолчанию, переключение - в настройках.
			Theme: windows.SystemDefault,
			// Рабочий каталог WebView2 - только в %LOCALAPPDATA%: рядом с
			// .exe его класть нельзя (см. paths.WebView2).
			WebviewUserDataPath: webviewDir,
		},
	}
}
