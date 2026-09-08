package main

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/netstat"
	"github.com/kamilsmtv/freeturn-windows/internal/paths"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/routes"
	"github.com/kamilsmtv/freeturn-windows/internal/tunnel"
	"github.com/kamilsmtv/freeturn-windows/internal/wg"
	"github.com/kamilsmtv/freeturn-windows/internal/wintun"
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

	allowed, err := wg.AllowedIPs(p.Client.SplitTunnelMode, p.Client.SplitTunnelSubnets)
	if err != nil {
		return err
	}

	dir, err := paths.Bin()
	if err != nil {
		return err
	}
	// Библиотеки может не быть при первом запуске - тогда качаем её.
	if !wintun.Installed(dir) {
		a.core.AppendLog("info", "Скачиваю компонент сетевого адаптера (wintun "+wintun.Version+")")
		if err := wintun.Ensure(a.bg(), dir, nil); err != nil {
			return err
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
	a.emit(EventTunnel, a.TunnelStatus())
}

// raiseTunnel готовит маршруты-исключения и поднимает туннель.
func (a *App) raiseTunnel(gen int64, p profile.Profile) {
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
	}
	a.emit(EventTunnel, a.TunnelStatus())
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

// stopTunnel снимает туннель, если он поднят.
func (a *App) stopTunnel() {
	if !a.tunnel.Running() {
		// Маршруты-исключения могли остаться от неудачной попытки.
		a.routes.Release()
		return
	}
	a.core.AppendLog("info", "Снимаю туннель")
	a.tunnel.Down()
	a.routes.Release()
	a.tunnelErr.Store("")
	a.emit(EventTunnel, a.TunnelStatus())
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
		return err
	}
	return nil
}

// WintunInstalled сообщает, готов ли компонент адаптера.
func (a *App) WintunInstalled() bool {
	dir, err := paths.Bin()
	if err != nil {
		return false
	}
	return wintun.Installed(dir)
}
