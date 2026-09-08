package main

import (
	"context"
	"errors"

	"github.com/kamilsmtv/freeturn-windows/internal/autostart"
	"github.com/kamilsmtv/freeturn-windows/internal/core"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/tray"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// errNoActiveProfile - подключаться нечем.
var errNoActiveProfile = errors.New("не выбран профиль для подключения")

// setupTray поднимает значок в области уведомлений и связывает его с ядром.
func (a *App) setupTray() {
	// Действия логируются: если значок снова начнёт «молчать», по журналу
	// будет видно, дошёл ли клик до обработчика.
	action := func(name string, f func()) func() {
		return func() {
			a.logf("трей: %s", name)
			f()
		}
	}
	a.tray = tray.New(tray.Handlers{
		Show:       action("показать окно", a.ShowWindow),
		Connect:    action("подключить", func() { _ = a.ConnectActive() }),
		Disconnect: action("отключить", func() { _ = a.core.Stop() }),
		Quit:       action("выход", a.QuitApp),
	})
	a.tray.Start()
	a.updateTray(a.core.Status())
}

// updateTray показывает в значке текущее состояние.
func (a *App) updateTray(st core.Status) {
	if a.tray == nil {
		return
	}
	name := ""
	if p, err := a.profiles.Get(st.ProfileID); err == nil {
		name = p.Name
	} else if active, ok := a.profiles.All().Active(); ok {
		name = active.Name
	}
	a.tray.SetState(tray.State{
		Connected: st.State == core.StateRunning,
		Failed:    st.State == core.StateFailed,
		Profile:   name,
		Detail:    st.Error,
	})
}

// ShowWindow возвращает окно на экран из трея.
func (a *App) ShowWindow() {
	if a.ctx == nil {
		return
	}
	wailsruntime.WindowUnminimise(a.ctx)
	wailsruntime.WindowShow(a.ctx)
}

// HideWindow прячет окно в трей.
func (a *App) HideWindow() {
	if a.ctx != nil {
		wailsruntime.WindowHide(a.ctx)
	}
}

// QuitApp завершает работу: ядро останавливается в shutdown.
func (a *App) QuitApp() {
	a.quitting.Store(true)
	if a.ctx != nil {
		// Дальше сработает shutdown: он снимет туннель, маршруты и ядро.
		wailsruntime.Quit(a.ctx)
		return
	}
	// Окна ещё нет - прибираемся сами, иначе процесс уйдёт, оставив за
	// собой туннель, маршруты и работающее ядро.
	a.stopTunnel()
	a.core.Close()
	a.routes.Release()
}

// ConnectActive запускает ядро с активным профилем (из трея и автоподключения).
func (a *App) ConnectActive() error {
	p, ok := a.profiles.All().Active()
	if !ok {
		return errNoActiveProfile
	}
	return a.CoreStart(p)
}

// SetAutostart включает или выключает запуск при входе в систему.
func (a *App) SetAutostart(enable bool) error {
	if err := autostart.Apply(enable); err != nil {
		return err
	}
	s := a.settings.Get()
	s.Autostart = autostart.Enabled()
	return a.settings.Save(s)
}

// AutostartEnabled сообщает, заведена ли задача автозапуска.
func (a *App) AutostartEnabled() bool { return autostart.Enabled() }

// beforeClose отрабатывает закрытие окна, когда штатный HideWindowOnClose
// выключен: он читается один раз при старте, а настройку могли включить
// уже в работающем приложении.
func (a *App) beforeClose(context.Context) bool {
	if a.quitting.Load() {
		return false
	}
	if a.settings.Get().MinimizeToTray {
		a.HideWindow()
		a.logf("окно закрыто - свёрнуто в трей")
		return true
	}
	a.logf("окно закрыто - выход из приложения")
	return false
}

// autoConnect поднимает последний профиль после старта, если так настроено.
func (a *App) autoConnect() {
	s := a.settings.Get()
	if !s.AutoConnectLast {
		return
	}
	snap := a.profiles.All()
	// Активный профиль важнее последнего запущенного: пользователь мог
	// переключиться, не подключаясь.
	connect := func(p profile.Profile) {
		if err := a.CoreStart(p); err != nil {
			// Автоподключение не должно молчать: чаще всего это ещё не
			// скачанное ядро или незаполненный профиль.
			a.core.AppendLog("error", "Автоподключение не удалось: "+err.Error())
		}
	}
	if p, ok := snap.Active(); ok {
		connect(p)
		return
	}
	if p, err := a.profiles.Get(s.LastProfileID); err == nil {
		connect(p)
	}
}
