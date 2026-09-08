// Package routes держит маршруты-исключения мимо туннеля.
//
// Ядро само пиннит маршруты только к TURN-серверам (-routes). На Android
// остальной его трафик исключает из VPN сам VpnService.protect(), а на
// Windows такого механизма нет: с маршрутом 0.0.0.0/0 в туннель уходят и
// запросы ядра к VK, и его DNS - получается замкнутый круг, где туннель
// ждёт ядро, а ядро не может выйти в сеть. Поэтому адреса, нужные самому
// ядру, мы пинним на физический шлюз.
package routes

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// coreRouteRe ловит строку ядра о добавленном маршруте к TURN-серверу.
var coreRouteRe = regexp.MustCompile(`Ensuring route to (\d+\.\d+\.\d+\.\d+)`)

// CoreRouteFromLog достаёт адрес из строки журнала ядра о добавленном
// маршруте; невалидная строка даёт неготовый адрес.
func CoreRouteFromLog(line string) (netip.Addr, bool) {
	m := coreRouteRe.FindStringSubmatch(line)
	if m == nil {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(m[1])
	return addr, err == nil
}

// VKHosts - узлы, с которыми ядро работает помимо TURN: авторизация и API
// провайдера. Их адреса тоже должны идти мимо туннеля.
var VKHosts = []string{"vk.ru", "login.vk.ru", "api.vk.ru", "vk.com", "login.vk.com"}

// Pinner добавляет и снимает маршруты-исключения.
type Pinner struct {
	// Log получает сообщения о добавленных маршрутах.
	Log func(string)
	// StatePath - файл со списком добавленных маршрутов. Нужен, чтобы
	// снять их после аварийного завершения: при taskkill ни мы, ни ядро
	// прибраться за собой не успеваем.
	StatePath string

	mu      sync.Mutex
	gateway string
	pinned  map[netip.Addr]bool
}

// NewPinner создаёт пиннер маршрутов. statePath может быть пустым - тогда
// список никуда не сохраняется.
func NewPinner(statePath string, log func(string)) *Pinner {
	return &Pinner{Log: log, StatePath: statePath, pinned: map[netip.Addr]bool{}}
}

// Adopt берёт на себя уборку чужого маршрута.
//
// Ядро само пиннит маршруты к TURN-серверам и снимает их при штатном
// завершении, но на Windows мы останавливаем его принудительно, и убрать
// их оно уже не успевает.
func (p *Pinner) Adopt(addr netip.Addr) {
	if !usableForPin(addr) {
		return
	}
	p.mu.Lock()
	already := p.pinned[addr]
	if !already {
		p.pinned[addr] = true
	}
	p.mu.Unlock()

	if !already {
		p.persist()
	}
}

// CleanStale снимает маршруты, оставшиеся от прошлого запуска.
func (p *Pinner) CleanStale() {
	list := p.loadState()
	if len(list) == 0 {
		return
	}
	for _, addr := range list {
		_ = delRoute(addr.String())
	}
	p.logf("сняты маршруты, оставшиеся от прошлого запуска: %d", len(list))
	p.clearState()
}

// persist сохраняет текущий список маршрутов.
func (p *Pinner) persist() {
	if p.StatePath == "" {
		return
	}
	p.mu.Lock()
	list := make([]string, 0, len(p.pinned))
	for addr := range p.pinned {
		list = append(list, addr.String())
	}
	p.mu.Unlock()

	sort.Strings(list)
	data, err := json.Marshal(list)
	if err != nil {
		return
	}
	// Список - вспомогательные данные: ошибку записи молча игнорируем.
	_ = os.WriteFile(p.StatePath, data, 0o600)
}

func (p *Pinner) loadState() []netip.Addr {
	if p.StatePath == "" {
		return nil
	}
	data, err := os.ReadFile(p.StatePath)
	if err != nil {
		return nil
	}
	var list []string
	if json.Unmarshal(data, &list) != nil {
		return nil
	}

	out := make([]netip.Addr, 0, len(list))
	for _, s := range list {
		if addr, err := netip.ParseAddr(s); err == nil && usableForPin(addr) {
			out = append(out, addr)
		}
	}
	return out
}

func (p *Pinner) clearState() {
	if p.StatePath != "" {
		_ = os.Remove(p.StatePath)
	}
}

func (p *Pinner) logf(format string, args ...any) {
	if p.Log != nil {
		p.Log(sprintf(format, args...))
	}
}

// Prepare запоминает текущий шлюз по умолчанию. Вызывать нужно до подъёма
// туннеля: после него шлюзом станет сам туннель.
func (p *Pinner) Prepare() error {
	gw, err := defaultGateway()
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.gateway = gw
	p.mu.Unlock()
	p.logf("физический шлюз: %s", gw)
	return nil
}

// Gateway возвращает запомненный шлюз.
func (p *Pinner) Gateway() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gateway
}

// PinAddrs добавляет маршруты к адресам через физический шлюз.
func (p *Pinner) PinAddrs(addrs []netip.Addr) {
	p.mu.Lock()
	gw := p.gateway
	p.mu.Unlock()
	if gw == "" {
		return
	}

	var added []string
	for _, addr := range addrs {
		if !usableForPin(addr) {
			continue
		}
		p.mu.Lock()
		already := p.pinned[addr]
		p.mu.Unlock()
		if already {
			continue
		}
		if err := addRoute(addr.String(), gw); err != nil {
			p.logf("не удалось добавить маршрут к %s: %v", addr, err)
			continue
		}
		p.mu.Lock()
		p.pinned[addr] = true
		p.mu.Unlock()
		added = append(added, addr.String())
	}
	if len(added) > 0 {
		sort.Strings(added)
		p.logf("мимо туннеля пущены адреса: %s", strings.Join(added, ", "))
		p.persist()
	}
}

// PinHosts резолвит имена и пиннит их адреса. Резолв делается до подъёма
// туннеля, пока системный резолвер ещё доступен.
func (p *Pinner) PinHosts(ctx context.Context, hosts []string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var resolver net.Resolver
	var addrs []netip.Addr
	for _, host := range hosts {
		ips, err := resolver.LookupNetIP(ctx, "ip4", host)
		if err != nil {
			p.logf("не удалось разрешить %s: %v", host, err)
			continue
		}
		addrs = append(addrs, ips...)
	}
	p.PinAddrs(addrs)
}

// Release снимает все добавленные маршруты.
func (p *Pinner) Release() {
	p.mu.Lock()
	list := make([]netip.Addr, 0, len(p.pinned))
	for addr := range p.pinned {
		list = append(list, addr)
	}
	p.pinned = map[netip.Addr]bool{}
	p.mu.Unlock()

	for _, addr := range list {
		_ = delRoute(addr.String())
	}
	p.clearState()
	if len(list) > 0 {
		p.logf("снято маршрутов-исключений: %d", len(list))
	}
}

// usableForPin отсеивает адреса, для которых маршрут не нужен или вреден.
func usableForPin(addr netip.Addr) bool {
	if !addr.Is4() || !addr.IsValid() {
		return false
	}
	// Локальные и служебные адреса и так идут мимо туннеля.
	return !addr.IsLoopback() && !addr.IsPrivate() && !addr.IsLinkLocalUnicast() &&
		!addr.IsMulticast() && !addr.IsUnspecified()
}
