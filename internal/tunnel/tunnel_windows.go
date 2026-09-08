package tunnel

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	"github.com/amnezia-vpn/amneziawg-go/v3/tun"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"

	"github.com/kamilsmtv/freeturn-windows/internal/wintun"
)

// impl держит поднятый туннель: устройство amneziawg-go поверх адаптера
// Wintun плюс его LUID, по которому настраиваются адреса и маршруты.
type impl struct {
	dev  *device.Device
	tun  tun.Device
	luid winipcfg.LUID
}

func up(cfg *Config, o Options) (*impl, error) {
	name := o.Name
	if name == "" {
		name = "FreeTurn"
	}

	// Библиотека лежит в каталоге приложения, а не рядом с .exe, поэтому
	// её нужно загрузить самим до первого обращения к Wintun.
	if o.WintunDir != "" {
		if err := wintun.Preload(o.WintunDir); err != nil {
			return nil, err
		}
	}

	// Wintun создаёт адаптер и требует прав администратора; они у нас есть
	// по манифесту.
	tunDev, err := tun.CreateTUN(name, cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать сетевой адаптер: %w", err)
	}

	native, ok := tunDev.(*tun.NativeTun)
	if !ok {
		_ = tunDev.Close()
		return nil, fmt.Errorf("неожиданный тип адаптера %T", tunDev)
	}
	luid := winipcfg.LUID(native.LUID())

	logger := device.NewLogger(device.LogLevelError, "")
	if o.Log != nil {
		logger = &device.Logger{
			Verbosef: func(format string, args ...any) { o.Log(fmt.Sprintf(format, args...)) },
			Errorf: func(format string, args ...any) {
				o.Log("ошибка туннеля: " + fmt.Sprintf(format, args...))
			},
		}
	}

	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	uapi, err := UAPI(cfg)
	if err != nil {
		dev.Close()
		return nil, err
	}
	if err := dev.IpcSet(uapi); err != nil {
		dev.Close()
		return nil, fmt.Errorf("не удалось настроить туннель: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("не удалось поднять туннель: %w", err)
	}

	t := &impl{dev: dev, tun: tunDev, luid: luid}
	if err := configureInterface(luid, cfg); err != nil {
		down(t)
		return nil, err
	}
	return t, nil
}

// configureInterface выдаёт адаптеру адреса, маршруты, DNS и MTU - то, что
// в Linux делает wg-quick, а в Windows приходится делать самим.
func configureInterface(luid winipcfg.LUID, cfg *Config) error {
	if err := luid.SetIPAddressesForFamily(windows.AF_INET, ipv4Only(cfg.Addresses)); err != nil {
		return fmt.Errorf("не удалось назначить адрес адаптеру: %w", err)
	}

	routes := make([]*winipcfg.RouteData, 0, len(cfg.Peers))
	for i := range cfg.Peers {
		for _, allowed := range cfg.Peers[i].AllowedIPs {
			if !allowed.Addr().Is4() {
				// IPv6 туннель пока не ведём: серверная часть раздаёт IPv4.
				continue
			}
			routes = append(routes, &winipcfg.RouteData{
				Destination: allowed.Masked(),
				// Нулевой следующий узел = маршрут через сам интерфейс.
				NextHop: netip.IPv4Unspecified(),
				Metric:  0,
			})
		}
	}
	if err := luid.SetRoutesForFamily(windows.AF_INET, routes); err != nil {
		return fmt.Errorf("не удалось добавить маршруты туннеля: %w", err)
	}

	if len(cfg.DNS) > 0 {
		if err := luid.SetDNS(windows.AF_INET, ipv4Addrs(cfg.DNS), nil); err != nil {
			return fmt.Errorf("не удалось задать DNS туннеля: %w", err)
		}
	}

	// MTU задаётся и адаптеру: Wintun принимает его при создании, но
	// системная запись интерфейса живёт отдельно.
	iface, err := luid.IPInterface(windows.AF_INET)
	if err != nil {
		return fmt.Errorf("не удалось прочитать параметры интерфейса: %w", err)
	}
	iface.NLMTU = uint32(cfg.MTU)
	// Метрику ставим вручную: с автоматической маршрут по умолчанию через
	// туннель может проиграть физическому подключению.
	iface.UseAutomaticMetric = false
	iface.Metric = 0
	if err := iface.Set(); err != nil {
		return fmt.Errorf("не удалось применить параметры интерфейса: %w", err)
	}
	return nil
}

func down(t *impl) {
	if t == nil {
		return
	}
	// Закрытие устройства снимает и адаптер Wintun вместе с его маршрутами.
	if t.dev != nil {
		t.dev.Close()
	}
}

// stats читает счётчики у самого устройства: они точнее адаптерных, потому
// что считают именно трафик туннеля.
func stats(t *impl) (Stats, bool) {
	if t == nil || t.dev == nil {
		return Stats{}, false
	}
	raw, err := t.dev.IpcGet()
	if err != nil {
		return Stats{}, false
	}

	var s Stats
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "rx_bytes":
			s.RxBytes += parseUint(value)
		case "tx_bytes":
			s.TxBytes += parseUint(value)
		case "last_handshake_time_sec":
			if v := int64(parseUint(value)); v > s.Handshake {
				s.Handshake = v
			}
		}
	}
	return s, true
}

func parseUint(v string) uint64 {
	n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func ipv4Only(list []netip.Prefix) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(list))
	for _, p := range list {
		if p.Addr().Is4() {
			out = append(out, p)
		}
	}
	return out
}

func ipv4Addrs(list []netip.Addr) []netip.Addr {
	out := make([]netip.Addr, 0, len(list))
	for _, a := range list {
		if a.Is4() {
			out = append(out, a)
		}
	}
	return out
}
