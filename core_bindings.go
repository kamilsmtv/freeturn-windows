package main

import (
	"context"
	"errors"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/core"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/settings"
	"github.com/kamilsmtv/freeturn-windows/internal/updater"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// События, которые фронтенд слушает через runtime.EventsOn.
const (
	EventCoreState    = "core:state"
	EventCoreLog      = "core:log"
	EventUpdateStatus = "update:status"
	EventUpdateStep   = "update:progress"
)

// CoreStatus отдаёт текущее состояние ядра.
func (a *App) CoreStatus() core.Status { return a.core.Status() }

// CoreLog отдаёт накопленный журнал.
func (a *App) CoreLog() []core.LogLine { return a.core.Log() }

// CoreClearLog очищает журнал.
func (a *App) CoreClearLog() { a.core.ClearLog() }

// CoreStart запускает ядро с переданным профилем.
func (a *App) CoreStart(p profile.Profile) error {
	if !a.updates.Installed() {
		return errors.New("ядро ещё не скачано - нажмите «Скачать ядро» на вкладке обновлений")
	}
	// В режиме VPN ядру нужен DNS физической сети: DNS туннеля станет ему
	// недоступен ровно тогда, когда туннель поднимется.
	p = withPhysicalDNS(p)
	a.lastProfile.Store(&p)
	a.tunnelErr.Store("")

	if err := a.core.Start(p); err != nil {
		return err
	}
	// Поколение меняем только после успешного запуска: иначе повторное
	// нажатие «Подключить» при уже работающем ядре отменило бы отложенный
	// подъём туннеля текущего сеанса.
	gen := a.connectGen.Add(1)
	// Туннель поднимаем после ядра: до того как оно займёт локальный
	// сокет, отправлять в него пакеты бессмысленно.
	switch {
	case wantsTunnel(p):
		go a.raiseTunnelWhenReady(gen, p)
	case p.Client.TunnelTransport == "wireguard":
		// Режим VPN выбран, а конфигурации нет: ядро запустится прокси, и
		// без этого предупреждения интерфейс ждал бы туннель бесконечно.
		a.tunnelErr.Store("в профиле выбран режим VPN, но нет конфигурации WireGuard - " +
			"запросите её у сервера; пока работает только режим прокси")
		a.core.AppendLog("warn", a.tunnelErr.Load())
		a.emit(EventTunnel, a.TunnelStatus())
	}
	s := a.settings.Get()
	if s.LastProfileID != p.ID {
		s.LastProfileID = p.ID
		_ = a.settings.Save(s)
	}
	return nil
}

// CoreStop снимает туннель и останавливает ядро.
func (a *App) CoreStop() error {
	// Смена поколения останавливает отложенный подъём туннеля, если он
	// ещё ждёт готовности ядра.
	a.connectGen.Add(1)
	a.stopTunnel()

	err := a.core.Stop()
	// Ядро останавливается принудительно и свои маршруты к TURN снять не
	// успевает - делаем это за него.
	a.routes.Release()
	return err
}

// UpdateStatus - последняя известная информация об обновлении ядра.
func (a *App) UpdateStatus() updater.Status {
	return a.updates.Check(a.bg(), false)
}

// CheckCoreUpdate обращается к GitHub принудительно, минуя интервал кэша.
func (a *App) CheckCoreUpdate() updater.Status {
	st := a.updates.Check(a.bg(), true)
	a.emit(EventUpdateStatus, st)
	return st
}

// InstallCore скачивает и ставит последнюю версию ядра.
func (a *App) InstallCore() (updater.Status, error) {
	st, err := a.updates.Install(a.bg(), true, func(p updater.Progress) {
		a.emit(EventUpdateStep, p)
	})
	a.emit(EventUpdateStatus, st)
	return st, err
}

// RollbackCore возвращает предыдущую версию ядра.
func (a *App) RollbackCore() (updater.Status, error) {
	st, err := a.updates.Rollback(a.bg())
	a.emit(EventUpdateStatus, st)
	return st, err
}

// CheckGUIUpdate проверяет обновление самого приложения (только уведомление).
func (a *App) CheckGUIUpdate() updater.GUIStatus {
	return updater.CheckGUI(a.bg(), a.github, GUIRepo, version, true)
}

// bg возвращает контекст фоновых операций: он живёт, пока живёт окно.
func (a *App) bg() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) emit(event string, data any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, event, data)
}

// watchUpdates обслуживает автопроверку обновлений: первая - сразу после
// старта, дальше по интервалу из настроек.
func (a *App) watchUpdates() {
	// Небольшая задержка, чтобы окно успело подписаться на события.
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-a.bg().Done():
			return
		case <-timer.C:
		}

		s := a.settings.Get()
		a.runUpdateCheck(s)
		timer.Reset(time.Duration(s.CoreUpdateHours) * time.Hour)
	}
}

// runUpdateCheck выполняет одну проверку с учётом выбранного режима.
func (a *App) runUpdateCheck(s settings.Settings) {
	// Без ядра качаем всегда: первый запуск должен закончиться рабочим клиентом.
	if !a.updates.Installed() {
		if _, err := a.InstallCore(); err != nil {
			a.emit(EventUpdateStatus, updater.Status{Error: err.Error()})
		}
		return
	}
	if s.CoreUpdateMode == settings.UpdateNever {
		return
	}

	st := a.updates.Check(a.bg(), false)
	a.emit(EventUpdateStatus, st)

	// В режиме "ask" решение за пользователем: баннер уже показан.
	if st.UpdateReady && s.CoreUpdateMode == settings.UpdateAuto {
		if _, err := a.InstallCore(); err != nil {
			a.emit(EventUpdateStatus, updater.Status{Error: err.Error()})
		}
	}
	if s.CheckGUIUpdates {
		if gui := updater.CheckGUI(a.bg(), a.github, GUIRepo, version, false); gui.UpdateReady {
			a.emit("update:gui", gui)
		}
	}
}
