package main

import (
	"context"
	"path/filepath"
	"sync/atomic"

	"github.com/kamilsmtv/freeturn-windows/internal/core"
	"github.com/kamilsmtv/freeturn-windows/internal/paths"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/routes"
	"github.com/kamilsmtv/freeturn-windows/internal/settings"
	"github.com/kamilsmtv/freeturn-windows/internal/tray"
	"github.com/kamilsmtv/freeturn-windows/internal/tunnel"
	"github.com/kamilsmtv/freeturn-windows/internal/updater"
	"github.com/kamilsmtv/freeturn-windows/internal/vps"
	"github.com/kamilsmtv/freeturn-windows/internal/winenv"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App - объект, методы которого Wails экспортирует во фронтенд.
type App struct {
	ctx      context.Context
	settings *settings.Store
	core     *core.Manager
	profiles *profile.Store
	updates  *updater.CoreUpdater
	vps      *vps.Client
	github   *updater.GitHub

	tray    *tray.Tray
	tunnel  *tunnel.Tunnel
	routes  *routes.Pinner
	traffic trafficSampler

	// tunnelErr хранит причину, по которой туннель не поднялся.
	tunnelErr atomicString

	// quitting отличает настоящий выход от закрытия окна в трей.
	quitting atomic.Bool

	// connectGen растёт на каждом подключении и отключении: по нему
	// отложенный подъём туннеля понимает, что его сеанс уже неактуален.
	connectGen atomic.Int64

	// lastProfile нужен апдейтеру: после замены бинаря ядро поднимается
	// с тем же профилем, с которым работало до обновления.
	lastProfile atomic.Pointer[profile.Profile]
}

// NewApp собирает приложение: хранилище настроек, менеджер ядра и апдейтер.
func NewApp(st *settings.Store) (*App, error) {
	dataDir, err := paths.Data()
	if err != nil {
		return nil, err
	}
	coreDir, err := paths.Core()
	if err != nil {
		return nil, err
	}

	profilesFile, err := paths.ProfilesFile()
	if err != nil {
		return nil, err
	}
	profiles, err := profile.OpenStore(profilesFile)
	if err != nil {
		return nil, err
	}

	a := &App{settings: st, profiles: profiles, tunnel: tunnel.New()}
	a.routes = routes.NewPinner(
		filepath.Join(dataDir, "pinned-routes.json"),
		func(line string) { a.core.AppendLog("info", line) },
	)
	// Маршруты от прошлого запуска могли пережить аварийное завершение.
	a.routes.CleanStale()
	a.github = updater.NewGitHub(dataDir, func() string { return a.settings.Get().GitHubToken })
	a.updates = updater.NewCoreUpdater(coreDir, CoreRepo, a.github)
	a.core = core.NewManager(func() (string, error) { return a.updates.BinPath(), nil }, st.Get().KeepLogLines)

	// Апдейтер сам гасит и поднимает ядро вокруг подмены файла.
	a.updates.Stop = func() (bool, error) {
		running := a.core.Running()
		return running, a.CoreStop()
	}
	// Через CoreStart, а не core.Start: иначе после обновления ядра
	// профиль в режиме VPN остался бы без туннеля.
	a.updates.Start = func() error {
		p := a.lastProfile.Load()
		if p == nil {
			return nil
		}
		return a.CoreStart(*p)
	}

	a.vps = vps.New(dataDir, func(line string) { a.emit(EventVPSLog, line) })

	a.core.OnState(func(s core.Status) {
		// Туннель без ядра - чёрная дыра: маршрут по умолчанию ведёт в
		// адаптер, за которым уже никого нет.
		if s.State == core.StateFailed || s.State == core.StateStopped {
			go a.stopTunnel()
		}
		a.emit(EventCoreState, s)
		a.updateTray(s)
	})
	a.core.OnLog(func(l core.LogLine) {
		// Маршруты к TURN ядро добавляет само, но снять их при
		// принудительной остановке уже не успевает - берём уборку на себя.
		if addr, ok := routes.CoreRouteFromLog(l.Text); ok {
			a.routes.Adopt(addr)
		}
		a.emit(EventCoreLog, l)
	})
	return a, nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.applyTheme(a.settings.Get().Theme)
	// Предупреждения о повреждённых файлах видно только в журнале - зато
	// приложение запускается, а не падает молча.
	for _, w := range []string{a.settings.Warning(), a.profiles.Warning()} {
		if w != "" {
			a.core.AppendLog("warn", w)
		}
	}

	a.setupTray()
	go a.watchUpdates()
	go a.watchSubscriptions()
	go a.autoConnect()
}

// shutdown останавливает ядро при выходе. Маршруты, добавленные флагом
// -routes, снимает само ядро при завершении, поэтому важно дать ему
// закончиться, а не убивать процесс на месте.
func (a *App) shutdown(context.Context) {
	a.quitting.Store(true)
	// Туннель снимаем первым: иначе система останется с маршрутом в
	// исчезнувший адаптер.
	a.stopTunnel()
	if a.tray != nil {
		a.tray.Stop()
	}
	a.core.Close()
	// Ядро остановлено принудительно: его маршруты к TURN снимаем сами.
	a.routes.Release()
}

// AppInfo - сведения о самом GUI для экрана «О программе».
type AppInfo struct {
	Version  string `json:"version"`
	DataDir  string `json:"dataDir"`
	CoreDir  string `json:"coreDir"`
	LogDir   string `json:"logDir"`
	CoreRepo string `json:"coreRepo"`
	GUIRepo  string `json:"guiRepo"`
}

// Environment возвращает результат стартовой проверки окружения.
func (a *App) Environment() winenv.Report { return winenv.Check() }

// Info отдаёт версию и пути, которыми пользуется приложение.
func (a *App) Info() AppInfo {
	data, _ := paths.Data()
	core, _ := paths.Core()
	logs, _ := paths.Logs()
	return AppInfo{
		Version:  version,
		DataDir:  data,
		CoreDir:  core,
		LogDir:   logs,
		CoreRepo: CoreRepo,
		GUIRepo:  GUIRepo,
	}
}

// GetSettings отдаёт текущие настройки.
func (a *App) GetSettings() settings.Settings { return a.settings.Get() }

// SaveSettings сохраняет настройки и применяет тему окна.
func (a *App) SaveSettings(v settings.Settings) error {
	if err := a.settings.Save(v); err != nil {
		return err
	}
	a.applyTheme(a.settings.Get().Theme)
	return nil
}

// logf пишет в журнал окна (%APPDATA%\FreeTurn\logs\gui.log).
func (a *App) logf(format string, args ...any) {
	if a.ctx == nil {
		return
	}
	runtime.LogInfof(a.ctx, format, args...)
}

// OpenURL открывает ссылку в браузере по умолчанию.
func (a *App) OpenURL(url string) { runtime.BrowserOpenURL(a.ctx, url) }

// OpenDataDir показывает каталог с конфигами в проводнике.
func (a *App) OpenDataDir() error {
	dir, err := paths.Data()
	if err != nil {
		return err
	}
	return openInExplorer(dir)
}

func (a *App) applyTheme(theme string) {
	if a.ctx == nil {
		return
	}
	switch theme {
	case settings.ThemeLight:
		runtime.WindowSetLightTheme(a.ctx)
	case settings.ThemeDark:
		runtime.WindowSetDarkTheme(a.ctx)
	default:
		runtime.WindowSetSystemDefaultTheme(a.ctx)
	}
}

// atomicString - потокобезопасная строка; atomic.Value не принимает
// пустое значение того же типа после nil, поэтому храним указатель.
type atomicString struct {
	v atomic.Pointer[string]
}

func (s *atomicString) Store(value string) { s.v.Store(&value) }

func (s *atomicString) Load() string {
	if p := s.v.Load(); p != nil {
		return *p
	}
	return ""
}
