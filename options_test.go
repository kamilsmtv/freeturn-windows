package main

import "testing"

// Забытое поле в options.App не ломает сборку - приложение просто теряет
// поведение. Проверяем набор хуков, от которых зависит работа с треем и
// корректная остановка ядра.
func TestAppOptionsWiring(t *testing.T) {
	store := newTestSettings(t)
	app := &App{settings: store}

	opts := appOptions(store, app, `C:\tmp\webview2`)

	if opts.OnStartup == nil {
		t.Error("без OnStartup не поднимутся трей, автоподключение и проверка обновлений")
	}
	if opts.OnShutdown == nil {
		t.Error("без OnShutdown ядро переживёт выход и оставит за собой маршруты")
	}
	if opts.OnBeforeClose == nil {
		t.Error("без OnBeforeClose нельзя свернуть окно в трей после смены настройки")
	}
	if opts.Logger == nil {
		t.Error("без логгера отказ WebView2 виден только как пустое окно")
	}
	if opts.BackgroundColour == nil {
		t.Error("без цвета фона незагрузившаяся страница выглядит чёрным окном")
	}
	if opts.Windows == nil || opts.Windows.WebviewUserDataPath == "" {
		t.Error("каталог WebView2 обязан быть задан: иначе движок пишет рядом с .exe")
	}
	if opts.AssetServer == nil || opts.AssetServer.Assets == nil {
		t.Error("не задан источник ассетов интерфейса")
	}
	if len(opts.Bind) == 0 {
		t.Error("без Bind фронтенд не увидит ни одного метода бэкенда")
	}
}

func TestAppOptionsFollowSettings(t *testing.T) {
	store := newTestSettings(t)

	cur := store.Get()
	cur.MinimizeToTray, cur.StartMinimized = true, true
	if err := store.Save(cur); err != nil {
		t.Fatalf("Save: %v", err)
	}
	opts := appOptions(store, &App{settings: store}, "")
	if !opts.HideWindowOnClose || !opts.StartHidden {
		t.Error("настройки трея должны попадать в параметры окна")
	}

	cur.MinimizeToTray, cur.StartMinimized = false, false
	if err := store.Save(cur); err != nil {
		t.Fatalf("Save: %v", err)
	}
	opts = appOptions(store, &App{settings: store}, "")
	if opts.HideWindowOnClose || opts.StartHidden {
		t.Error("выключенные настройки трея тоже должны попадать в параметры окна")
	}
}
