package tunnel

import (
	"net/netip"
	"strings"
	"testing"
)

const sampleConf = `# выдан сервером
[Interface]
PrivateKey = QFtZkBQ0d1wY0aWJ8Vd0AGTe0HeNiKJRnEHXbUYXV1c=
Address = 10.13.13.2/32
DNS = 1.1.1.1, 8.8.8.8
MTU = 1420
Jc = 4
Jmin = 40
Jmax = 70
S1 = 15
H1 = 1234567890

[Peer]
PublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=
PresharedKey = 2P9J3zGjqYQKLYrJx8xnGz2Vv1kY7VFVGPpVWOFYbHo=
AllowedIPs = 0.0.0.0/0
Endpoint = 1.2.3.4:56000
PersistentKeepalive = 25
`

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig(sampleConf)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	if cfg.PrivateKey.IsZero() {
		t.Error("приватный ключ не разобран")
	}
	if len(cfg.Addresses) != 1 || cfg.Addresses[0].String() != "10.13.13.2/32" {
		t.Errorf("адреса разобраны неверно: %v", cfg.Addresses)
	}
	if len(cfg.DNS) != 2 {
		t.Errorf("DNS разобраны неверно: %v", cfg.DNS)
	}
	if cfg.MTU != 1420 {
		t.Errorf("MTU = %d, ожидалось значение из файла", cfg.MTU)
	}
	if len(cfg.Peers) != 1 {
		t.Fatalf("узлов = %d", len(cfg.Peers))
	}
	p := cfg.Peers[0]
	if p.PublicKey.IsZero() || p.PresharedKey.IsZero() {
		t.Error("ключи узла не разобраны")
	}
	if p.Endpoint != "1.2.3.4:56000" || p.Keepalive != 25 {
		t.Errorf("параметры узла разобраны неверно: %+v", p)
	}
	if !cfg.Amnezia.Enabled() || cfg.Amnezia.Jc != 4 || cfg.Amnezia.S1 != 15 || cfg.Amnezia.H1 != "1234567890" {
		t.Errorf("параметры AmneziaWG разобраны неверно: %+v", cfg.Amnezia)
	}
}

func TestParseConfigRejectsIncomplete(t *testing.T) {
	tests := map[string]string{
		"без ключа":   "[Interface]\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=\nAllowedIPs = 0.0.0.0/0",
		"без адреса":  "[Interface]\nPrivateKey = QFtZkBQ0d1wY0aWJ8Vd0AGTe0HeNiKJRnEHXbUYXV1c=\n[Peer]\nPublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=\nAllowedIPs = 0.0.0.0/0",
		"без узла":    "[Interface]\nPrivateKey = QFtZkBQ0d1wY0aWJ8Vd0AGTe0HeNiKJRnEHXbUYXV1c=\nAddress = 10.0.0.2/32",
		"кривой ключ": "[Interface]\nPrivateKey = не base64\nAddress = 10.0.0.2/32",
	}
	for name, conf := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConfig(conf); err == nil {
				t.Error("ожидалась ошибка разбора")
			}
		})
	}
}

func TestUAPI(t *testing.T) {
	cfg, err := ParseConfig(sampleConf)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	out, err := UAPI(cfg)
	if err != nil {
		t.Fatalf("UAPI: %v", err)
	}

	for _, want := range []string{
		"private_key=", "listen_port=0", "replace_peers=true", "public_key=",
		"preshared_key=", "endpoint=1.2.3.4:56000", "persistent_keepalive_interval=25",
		"replace_allowed_ips=true", "allowed_ip=0.0.0.0/0",
		"jc=4", "jmin=40", "jmax=70", "s1=15", "h1=1234567890",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("в протоколе IPC нет %q", want)
		}
	}
	// Ключи передаются в hex: base64 из conf-файла устройство не примет.
	if strings.Contains(out, "QFtZkBQ0d1wY") {
		t.Error("ключ уехал в IPC в base64")
	}
}

func TestApplyOptions(t *testing.T) {
	cfg, err := ParseConfig(sampleConf)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	applyOptions(cfg, Options{
		Endpoint:   "127.0.0.1:9000",
		AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
		DNS:        []netip.Addr{netip.MustParseAddr("9.9.9.9")},
	})

	p := cfg.Peers[0]
	if p.Endpoint != "127.0.0.1:9000" {
		t.Errorf("трафик должен идти в локальный сокет ядра, а не на %s", p.Endpoint)
	}
	if len(p.AllowedIPs) != 1 || p.AllowedIPs[0].String() != "10.0.0.0/8" {
		t.Errorf("маршруты раздельного туннелирования не применились: %v", p.AllowedIPs)
	}
	if len(cfg.DNS) != 1 || cfg.DNS[0].String() != "9.9.9.9" {
		t.Errorf("DNS не переопределён: %v", cfg.DNS)
	}
}

func TestApplyOptionsSetsKeepalive(t *testing.T) {
	cfg := &Config{Peers: []Peer{{}}}
	applyOptions(cfg, Options{})
	// Через TURN-релей молчащий пир теряет привязку канала.
	if cfg.Peers[0].Keepalive != 25 {
		t.Errorf("keepalive = %d, ожидалось значение по умолчанию", cfg.Peers[0].Keepalive)
	}
	if cfg.MTU != DefaultMTU {
		t.Errorf("MTU = %d, ожидался %d", cfg.MTU, DefaultMTU)
	}
}

// Без DNS адаптер туннеля не получает резолвер, и система продолжает
// спрашивать провайдера - тот на заблокированные домены отвечает NXDOMAIN.
func TestApplyOptionsSetsDefaultDNS(t *testing.T) {
	cfg := &Config{Peers: []Peer{{}}}
	applyOptions(cfg, Options{})

	if len(cfg.DNS) != 1 || cfg.DNS[0] != DefaultDNS {
		t.Fatalf("ожидался резолвер по умолчанию, получено %v", cfg.DNS)
	}
}

func TestApplyOptionsKeepsServerDNS(t *testing.T) {
	cfg := &Config{Peers: []Peer{{}}, DNS: []netip.Addr{netip.MustParseAddr("10.13.13.1")}}
	applyOptions(cfg, Options{})

	if len(cfg.DNS) != 1 || cfg.DNS[0].String() != "10.13.13.1" {
		t.Errorf("DNS из конфигурации сервера должен сохраняться: %v", cfg.DNS)
	}
}
