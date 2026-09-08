package tunnel

import (
	"errors"
	"net/netip"
	"sync"
)

// Options - что подставить в конфиг при подъёме туннеля.
type Options struct {
	// Name - имя сетевого адаптера в системе.
	Name string
	// Endpoint переопределяет адрес узла: туннель должен идти в локальный
	// сокет ядра, а не напрямую на VPS.
	Endpoint string
	// AllowedIPs переопределяет маршруты туннеля (раздельное туннелирование).
	AllowedIPs []netip.Prefix
	// DNS переопределяет DNS-серверы адаптера.
	DNS []netip.Addr
	// WintunDir - каталог с библиотекой wintun.dll (см. internal/wintun).
	WintunDir string
	// Log получает сообщения устройства.
	Log func(string)
}

// Stats - счётчики туннеля.
type Stats struct {
	RxBytes   uint64 `json:"rxBytes"`
	TxBytes   uint64 `json:"txBytes"`
	Handshake int64  `json:"handshake"`
}

// Tunnel - поднятый туннель.
type Tunnel struct {
	mu   sync.Mutex
	impl *impl
	name string
}

// DefaultDNS - резолвер по умолчанию для адаптера туннеля. Тот же, что
// подставляет серверная часть ядра в конфигурации клиентов.
var DefaultDNS = netip.MustParseAddr("1.1.1.1")

// ErrUnsupported - туннель доступен только в Windows.
var ErrUnsupported = errors.New("встроенный туннель поддерживается только в Windows")

// ErrAlreadyUp - туннель уже поднят.
var ErrAlreadyUp = errors.New("туннель уже поднят")

// New создаёт объект туннеля; сеть при этом не трогается.
func New() *Tunnel { return &Tunnel{} }

// Up поднимает туннель по конфигурации.
func (t *Tunnel) Up(cfg *Config, o Options) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.impl != nil {
		return ErrAlreadyUp
	}
	applyOptions(cfg, o)

	impl, err := up(cfg, o)
	if err != nil {
		return err
	}
	t.impl, t.name = impl, o.Name
	return nil
}

// Down снимает туннель и удаляет адаптер.
func (t *Tunnel) Down() {
	t.mu.Lock()
	impl := t.impl
	t.impl = nil
	t.mu.Unlock()

	if impl != nil {
		down(impl)
	}
}

// Up сообщает, поднят ли туннель.
func (t *Tunnel) Running() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.impl != nil
}

// Name возвращает имя адаптера.
func (t *Tunnel) Name() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.name
}

// Stats возвращает счётчики туннеля.
func (t *Tunnel) Stats() (Stats, bool) {
	t.mu.Lock()
	impl := t.impl
	t.mu.Unlock()
	if impl == nil {
		return Stats{}, false
	}
	return stats(impl)
}

// applyOptions накладывает на конфиг то, что решает приложение: адрес
// локального сокета ядра, маршруты раздельного туннелирования и DNS.
func applyOptions(cfg *Config, o Options) {
	for i := range cfg.Peers {
		if o.Endpoint != "" {
			cfg.Peers[i].Endpoint = o.Endpoint
		}
		if len(o.AllowedIPs) > 0 {
			cfg.Peers[i].AllowedIPs = o.AllowedIPs
		}
		// Через TURN-релей молчащий пир быстро теряет привязку канала.
		if cfg.Peers[i].Keepalive == 0 {
			cfg.Peers[i].Keepalive = 25
		}
	}
	if len(o.DNS) > 0 {
		cfg.DNS = o.DNS
	}
	// Без своего DNS адаптер туннеля не получает резолвер, и система
	// продолжает спрашивать провайдера - тот на заблокированные домены
	// отвечает NXDOMAIN, и сайты не открываются при рабочем туннеле.
	if len(cfg.DNS) == 0 {
		cfg.DNS = []netip.Addr{DefaultDNS}
	}
	if cfg.MTU == 0 {
		cfg.MTU = DefaultMTU
	}
}
