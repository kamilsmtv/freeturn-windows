package main

import (
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/netstat"
	"github.com/kamilsmtv/freeturn-windows/internal/paths"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/routes"
	"github.com/kamilsmtv/freeturn-windows/internal/socks"
	"github.com/kamilsmtv/freeturn-windows/internal/tunnel"
	"github.com/kamilsmtv/freeturn-windows/internal/wg"
	"github.com/kamilsmtv/freeturn-windows/internal/wintun"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// TunnelStatus - состояние встроенного туннеля для UI.
type TunnelStatus struct {
	Enabled bool   `json:"enabled"`
	Up      bool   `json:"up"`
	Adapter string `json:"adapter"`
	Error   string `json:"error"`
}

// EventTunnel - изменение состояния туннеля.
const EventTunnel = "tunnel:state"

// EventSocks - изменение состояния раздачи в локальную сеть.
const EventSocks = "socks:state"

// SocksStatus - состояние раздачи в локальную сеть для UI.
type SocksStatus struct {
	Enabled bool     `json:"enabled"`
	Up      bool     `json:"up"`
	Addr    string   `json:"addr"`
	Hints   []string `json:"hints"`
	Active  int      `json:"active"`
	Error   string   `json:"error"`
}

// SocksStatus отдаёт текущее состояние раздачи.
func (a *App) SocksStatus() SocksStatus {
	st := SocksStatus{
		Up:     a.socks.Running(),
		Addr:   a.socks.Addr(),
		Active: a.socks.Active(),
		Error:  a.socksErr.Load(),
	}
	if st.Up {
		st.Hints = netstat.LANAddrs(a.socks.Addr(), a.tunnel.Name())
	}

	if a.core.Running() {
		if p := a.lastProfile.Load(); p != nil {
			st.Enabled = wantsSocks(*p)
			return st
		}
	}
	if p, ok := a.profiles.All().Active(); ok {
		st.Enabled = wantsSocks(p)
	}
	return st
}

// wantsSocks сообщает, должен ли профиль раздавать туннель в сеть.
// Без туннеля раздача бессмысленна: заворачивать соседей некуда.
func wantsSocks(p profile.Profile) bool {
	return p.Client.SocksHotspot && wantsTunnel(p)
}

// startSocks поднимает SOCKS5-сервер поверх уже работающего туннеля и
// открывает порт в брандмауэре.
//
// gen - поколение сеанса. Сокет открывается не мгновенно, и за это время
// пользователь мог успеть отключиться. Раздача, пережившая отключение,
// опаснее всего: туннеля уже нет, и трафик соседей пошёл бы в сеть
// напрямую - ровно мимо того, ради чего они подключались.
func (a *App) startSocks(gen int64, p profile.Profile) error {
	listen := strings.TrimSpace(p.Client.SocksListen)
	if listen == "" {
		listen = profile.DefaultSocksListen
	}

	err := a.socks.Start(socks.Options{
		Listen:   listen,
		User:     strings.TrimSpace(p.Client.SocksUser),
		Password: p.Client.SocksPassword,
		// UDP нужен соседям для QUIC, звонков и игр. Отдельного
		// переключателя нет: раздача и так только по паролю.
		UDP: true,
		Log: a.core.AppendLog,
	})
	if err != nil {
		return err
	}
	if a.connectGen.Load() != gen {
		a.socks.Stop()
		return nil
	}

	// Правило брандмауэра ставим после сокета: если порт занят, открывать
	// его в системе незачем. Сокету на localhost правило не нужно вовсе -
	// снаружи до него всё равно не достучаться.
	if port, err := portOf(a.socks.Addr()); err == nil && !loopbackOnly(a.socks.Addr()) {
		if err := socks.OpenPort(port); err != nil {
			// Не повод снимать раздачу: в домашней сети правило может
			// уже быть, а в доменной его ставит политика.
			a.core.AppendLog("warn", "Раздача: "+err.Error())
		}
	}

	mode := "только TCP"
	if a.socks.UDP() {
		mode = "TCP и UDP"
	}
	a.core.AppendLog("info", "Раздача в локальную сеть включена: SOCKS5 на "+a.socks.Addr()+
		" ("+mode+"), вход по логину "+strings.TrimSpace(p.Client.SocksUser))
	return nil
}

// loopbackOnly сообщает, что сокет доступен только с этой машины.
func loopbackOnly(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

// stopSocks снимает раздачу и убирает правило брандмауэра.
func (a *App) stopSocks() {
	if !a.socks.Running() {
		a.socksErr.Store("")
		return
	}
	a.core.AppendLog("info", "Снимаю раздачу в локальную сеть")
	a.socks.Stop()
	socks.ClosePort()
	a.socksErr.Store("")
	a.emitSocks()
}

// emitSocks рассылает состояние раздачи.
func (a *App) emitSocks() { a.emit(EventSocks, a.SocksStatus()) }

// portOf достаёт номер порта из адреса вида ip:port.
func portOf(addr string) (int, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(port)
}

// TunnelStatus отдаёт текущее состояние туннеля.
func (a *App) TunnelStatus() TunnelStatus {
	st := TunnelStatus{Up: a.tunnel.Running(), Adapter: a.tunnel.Name(), Error: a.tunnelErr.Load()}

	// Пока ядро работает, состояние относится к запущенному профилю:
	// активным в списке к этому моменту мог стать уже другой.
	if a.core.Running() {
		if p := a.lastProfile.Load(); p != nil {
			st.Enabled = wantsTunnel(*p)
			return st
		}
	}
	if p, ok := a.profiles.All().Active(); ok {
		st.Enabled = wantsTunnel(p)
	}
	return st
}

// wantsTunnel сообщает, должен ли профиль поднимать туннель сам.
func wantsTunnel(p profile.Profile) bool {
	return p.Client.TunnelTransport == "wireguard" && strings.TrimSpace(p.Client.WireGuardConfig) != ""
}

// startTunnel поднимает туннель для профиля: разбирает конфиг сервера,
// направляет его в локальный сокет ядра и считает маршруты по правилам
// раздельного туннелирования.
func (a *App) startTunnel(p profile.Profile) error {
	cfg, err := tunnel.ParseConfig(p.Client.WireGuardConfig)
	if err != nil {
		return fmt.Errorf("конфигурация WireGuard не разобрана: %w", err)
	}

	allowed, err := wg.AllowedIPs(a.effectiveSplit(p))
	if err != nil {
		return err
	}

	dir, err := paths.Bin()
	if err != nil {
		return err
	}
	// Обычно библиотеку уже принёс prepareWintun; сюда доходит только
	// тот случай, когда скачать её не удалось.
	if !wintun.Installed(dir) {
		if err := wintun.Ensure(a.bg(), dir, nil); err != nil {
			return wintunDownloadError(err)
		}
	}

	name := p.Client.WireGuardTunnelName
	if name == "" {
		name = "FreeTurn"
	}

	// DNS туннеля берётся из конфигурации сервера; поле «Свои DNS» в
	// профиле относится к ядру и указывает на резолвер физической сети -
	// внутри туннеля он недоступен.
	err = a.tunnel.Up(cfg, tunnel.Options{
		Name:       name,
		Endpoint:   endpointFor(p),
		AllowedIPs: allowed,
		WintunDir:  dir,
		Log:        a.logTunnel,
	})
	if err != nil {
		return err
	}

	a.core.AppendLog("info", "Туннель поднят: адаптер "+name+
		", DNS "+formatAddrs(cfg.DNS)+", маршруты "+wg.FormatPrefixes(allowed))
	return nil
}

// prepareWintun докачивает библиотеку адаптера, если её ещё нет.
func (a *App) prepareWintun() {
	dir, err := paths.Bin()
	if err != nil || wintun.Installed(dir) {
		return
	}
	a.core.AppendLog("info", "Скачиваю компонент сетевого адаптера (wintun "+wintun.Version+")")
	if err := wintun.Ensure(a.bg(), dir, nil); err != nil {
		a.core.AppendLog("warn", "Компонент сетевого адаптера не скачан: "+err.Error())
	}
}

// tunnelNoise - служебные сообщения устройства. Их сотни на каждый подъём
// туннеля, и они вытесняют из журнала то, ради чего его открывают.
var tunnelNoise = []string{"Routine:", "UAPI:", "UDP bind has been updated"}

// isTunnelNoise отличает служебные сообщения устройства от полезных.
func isTunnelNoise(line string) bool {
	for _, noise := range tunnelNoise {
		if strings.Contains(line, noise) {
			return true
		}
	}
	return false
}

// logTunnel складывает в журнал только осмысленные сообщения туннеля.
func (a *App) logTunnel(line string) {
	if isTunnelNoise(line) {
		return
	}

	level := "debug"
	switch {
	case strings.Contains(line, "ошибка туннеля"):
		level = "error"
	case strings.Contains(line, "Handshake did not complete"),
		strings.Contains(line, "Received handshake response"),
		strings.Contains(line, "Interface state"):
		level = "info"
	}
	a.core.AppendLog(level, "туннель: "+line)
}

// formatAddrs собирает адреса в строку для журнала.
func formatAddrs(list []netip.Addr) string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.String())
	}
	if len(out) == 0 {
		return "не задан"
	}
	return strings.Join(out, ", ")
}

// endpointFor - адрес, куда туннель отправляет пакеты: локальный сокет ядра.
func endpointFor(p profile.Profile) string {
	listen := strings.TrimSpace(p.Client.LocalPort)
	if listen == "" {
		return profile.DefaultListen
	}
	// Ядро слушает 0.0.0.0 - подключаться всё равно нужно на localhost.
	if strings.HasPrefix(listen, "0.0.0.0:") {
		return "127.0.0.1:" + strings.TrimPrefix(listen, "0.0.0.0:")
	}
	return listen
}

func dnsServers(list string) []netip.Addr {
	var out []netip.Addr
	for _, part := range strings.FieldsFunc(list, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';' || r == '\n' || r == '\t'
	}) {
		// В поле DNS ядра допустим порт, адаптеру он не нужен.
		if host, _, ok := strings.Cut(part, ":"); ok {
			part = host
		}
		if addr, err := netip.ParseAddr(part); err == nil {
			out = append(out, addr)
		}
	}
	return out
}

// turnReadyMark - строка ядра, по которой видно, что канал через TURN
// установлен: до этого момента ядру нужен прямой выход в сеть.
const turnReadyMark = "TURN allocation up"

// raiseTunnelWhenReady ждёт, пока ядро поднимет канал через TURN, и только
// потом включает туннель.
//
// Порядок принципиален. Пока туннель не поднят, ядро свободно ходит к VK и
// TURN и само пиннит маршруты к TURN-серверам (-routes). Если включить
// туннель раньше, его маршрут 0.0.0.0/0 перехватит и эти запросы: ядро не
// достучится до провайдера, а туннель не поднимется без ядра.
func (a *App) raiseTunnelWhenReady(gen int64, p profile.Profile) {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		// Пользователь мог успеть отключиться или переключить профиль.
		if a.connectGen.Load() != gen {
			return
		}
		st := a.core.Status()
		switch st.State {
		case "stopped", "failed":
			return
		case "running":
			if a.core.HasLogLine(turnReadyMark) {
				a.raiseTunnel(gen, p)
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	if a.connectGen.Load() != gen {
		return
	}
	a.tunnelErr.Store("ядро не установило канал через TURN за 90 секунд - туннель не поднят")
	a.core.AppendLog("error", a.tunnelErr.Load())
	a.emitTunnel()
}

// raiseTunnel готовит маршруты-исключения и поднимает туннель, а следом
// раздачу в локальную сеть.
func (a *App) raiseTunnel(gen int64, p profile.Profile) {
	// Библиотека адаптера при первом запуске качается из сети, и это
	// единственный долгий шаг подъёма. Под замком держать его нельзя:
	// «Отключить», нажатое в это время, ждало бы конца скачивания.
	// Ошибку здесь не разбираем - её покажет сам подъём туннеля.
	a.prepareWintun()

	a.raiseMu.Lock()
	defer a.raiseMu.Unlock()

	// Пока ждали замок, отключение могло уже пройти целиком.
	if a.connectGen.Load() != gen {
		return
	}
	a.tunnelErr.Store("")

	// Маршруты для собственного трафика ядра нужно проложить до подъёма
	// туннеля, пока шлюзом по умолчанию остаётся физическая сеть.
	a.pinCoreRoutes(p)

	if a.connectGen.Load() != gen {
		a.routes.Release()
		return
	}

	if err := a.startTunnel(p); err != nil {
		a.tunnelErr.Store(err.Error())
		a.core.AppendLog("error", "Туннель не поднят: "+err.Error())
		a.routes.Release()
		a.emitTunnel()
		return
	}
	a.emitTunnel()

	// Раздача идёт поверх туннеля, поэтому поднимается только после него
	// и только если он действительно встал.
	if wantsSocks(p) {
		a.socksErr.Store("")
		if err := a.startSocks(gen, p); err != nil {
			a.socksErr.Store(err.Error())
			a.core.AppendLog("error", "Раздача в локальную сеть не включена: "+err.Error())
		}
		a.emitSocks()
	}
}

// withPhysicalDNS задаёт ядру DNS физической сети.
//
// Иначе после подъёма туннеля система отдаёт ядру DNS туннеля, и оно
// перестаёт резолвить узлы VK - а без них не переживёт обновление
// TURN-учёток. Пользовательский список, если он задан, не трогаем.
func withPhysicalDNS(p profile.Profile) profile.Profile {
	if !wantsTunnel(p) || strings.TrimSpace(p.Client.CustomDNS) != "" {
		return p
	}
	servers := netstat.SystemDNS(p.Client.WireGuardTunnelName)
	if len(servers) == 0 {
		return p
	}
	p.Client.CustomDNS = strings.Join(servers, ",")
	return p
}

// pinCoreRoutes пускает мимо туннеля то, без чего ядро перестанет работать:
// его DNS-серверы и узлы авторизации провайдера. TURN-серверы ядро пиннит
// само флагом -routes.
func (a *App) pinCoreRoutes(p profile.Profile) {
	if err := a.routes.Prepare(); err != nil {
		a.core.AppendLog("warn", "не удалось определить физический шлюз: "+err.Error())
		return
	}

	a.routes.PinAddrs(dnsServers(p.Client.CustomDNS))
	a.routes.PinHosts(a.bg(), routes.VKHosts)
}

// stopTunnelIfCoreDown снимает туннель вслед за упавшим ядром.
//
// Проверка нужна из-за задержки: сообщение об остановке приходит в
// отдельной горутине, и пользователь успевает нажать «Подключить» раньше,
// чем она доберётся до дела. Без проверки запоздалое сообщение снимало бы
// уже новый сеанс.
func (a *App) stopTunnelIfCoreDown() {
	if a.core.Running() {
		return
	}
	a.stopTunnel()
}

// stopTunnel снимает туннель, если он поднят.
//
// Поколение меняем первым делом, до замка: отложенный подъём, который
// сейчас идёт или только собирается, обязан увидеть, что он уже не нужен.
// Иначе снятие прошло бы по пустому месту, а следом поднялся бы туннель,
// который никто не ждёт.
func (a *App) stopTunnel() {
	a.connectGen.Add(1)

	a.raiseMu.Lock()
	defer a.raiseMu.Unlock()

	// Раздачу снимаем первой: её соединения идут через туннель, и жить
	// после его исчезновения им незачем.
	a.stopSocks()

	if !a.tunnel.Running() {
		// Маршруты-исключения могли остаться от неудачной попытки.
		a.routes.Release()
		return
	}
	a.core.AppendLog("info", "Снимаю туннель")
	a.tunnel.Down()
	a.routes.Release()
	a.tunnelErr.Store("")
	a.emitTunnel()
}

// emitTunnel рассылает состояние туннеля и подтягивает значок в трее.
//
// Значок должен говорить то же, что и окно: в режиме VPN связь появляется
// не с запуском ядра, а с подъёмом туннеля.
func (a *App) emitTunnel() {
	a.emit(EventTunnel, a.TunnelStatus())
	a.updateTray(a.core.Status())
}

// EnsureWintun докачивает библиотеку адаптера по запросу из настроек.
func (a *App) EnsureWintun() error {
	dir, err := paths.Bin()
	if err != nil {
		return err
	}
	if wintun.Installed(dir) {
		return nil
	}
	if err := wintun.Ensure(a.bg(), dir, nil); err != nil {
		return wintunDownloadError(err)
	}
	return nil
}

// ImportWintun ставит библиотеку адаптера из выбранного файла.
func (a *App) ImportWintun() error {
	path, err := a.openFileDialog("Выберите архив или библиотеку wintun", []wailsruntime.FileFilter{
		{DisplayName: "wintun (*.zip, *.dll)", Pattern: "*.zip;*.dll"},
		{DisplayName: "Все файлы", Pattern: "*.*"},
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}

	dir, err := paths.Bin()
	if err != nil {
		return err
	}
	if err := wintun.InstallFromFile(dir, path); err != nil {
		return err
	}
	a.core.AppendLog("info", "Компонент сетевого адаптера установлен из файла "+filepath.Base(path))
	return nil
}

// WintunSourceURL - адрес официального архива: его показывают рядом с
// кнопкой импорта, чтобы было что открыть на машине с доступом в сеть.
func (a *App) WintunSourceURL() string { return wintun.ArchiveURL }

// wintunDownloadError объясняет обход, когда сайт недоступен.
//
// Ситуация замкнутая: wintun.net у многих открывается только через VPN,
// а VPN без этой библиотеки не поднять. Молча оставлять человека с
// сетевой ошибкой в такой ловушке нельзя.
func wintunDownloadError(err error) error {
	return fmt.Errorf("%w.\n\nСайт может быть недоступен без VPN. Скачайте архив %s "+
		"на любой другой машине и укажите его на вкладке обновлений "+
		"кнопкой «Выбрать файл» - приложение проверит его по контрольной сумме", err, wintun.ArchiveURL)
}

// WintunInstalled сообщает, готов ли компонент адаптера.
func (a *App) WintunInstalled() bool {
	dir, err := paths.Bin()
	if err != nil {
		return false
	}
	return wintun.Installed(dir)
}
