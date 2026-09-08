package wg

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/kamilsmtv/freeturn-windows/internal/tunnel"
)

func TestAllowedIPsAll(t *testing.T) {
	got, err := AllowedIPs(SplitAll, "10.0.0.0/8")
	if err != nil {
		t.Fatalf("AllowedIPs: %v", err)
	}
	if FormatPrefixes(got) != "0.0.0.0/0" {
		t.Errorf("режим all должен давать весь трафик, получено %s", FormatPrefixes(got))
	}
}

func TestAllowedIPsInclude(t *testing.T) {
	got, err := AllowedIPs(SplitInclude, "10.0.0.0/8, 192.168.1.5")
	if err != nil {
		t.Fatalf("AllowedIPs: %v", err)
	}
	if FormatPrefixes(got) != "10.0.0.0/8, 192.168.1.5/32" {
		t.Errorf("получено %s", FormatPrefixes(got))
	}
	if _, err := AllowedIPs(SplitInclude, ""); err == nil {
		t.Error("пустой список в режиме include - ошибка")
	}
}

func TestAllowedIPsExclude(t *testing.T) {
	got, err := AllowedIPs(SplitExclude, "10.0.0.0/8")
	if err != nil {
		t.Fatalf("AllowedIPs: %v", err)
	}

	// Результат обязан покрывать всё, кроме исключения, и не пересекаться с ним.
	ex := netip.MustParsePrefix("10.0.0.0/8")
	for _, p := range got {
		if p.Overlaps(ex) {
			t.Fatalf("%s пересекается с исключением", p)
		}
	}
	for _, addr := range []string{"1.1.1.1", "9.255.255.255", "11.0.0.1", "192.168.0.1", "255.255.255.255"} {
		if !covers(got, netip.MustParseAddr(addr)) {
			t.Errorf("адрес %s должен остаться в туннеле", addr)
		}
	}
	for _, addr := range []string{"10.0.0.1", "10.255.255.255"} {
		if covers(got, netip.MustParseAddr(addr)) {
			t.Errorf("адрес %s должен быть исключён", addr)
		}
	}
	// Набор должен быть минимальным: вычитание /8 из /0 даёт 8 префиксов.
	if len(got) != 8 {
		t.Errorf("префиксов = %d, ожидалось 8: %s", len(got), FormatPrefixes(got))
	}
}

func TestAllowedIPsExcludeSeveral(t *testing.T) {
	got, err := AllowedIPs(SplitExclude, "192.168.0.0/16\n10.0.0.0/8 172.16.0.0/12")
	if err != nil {
		t.Fatalf("AllowedIPs: %v", err)
	}
	for _, addr := range []string{"192.168.1.1", "10.1.2.3", "172.16.5.5"} {
		if covers(got, netip.MustParseAddr(addr)) {
			t.Errorf("адрес %s должен быть исключён", addr)
		}
	}
	if !covers(got, netip.MustParseAddr("8.8.8.8")) {
		t.Error("публичный адрес должен остаться в туннеле")
	}
}

func TestAllowedIPsExcludeEverything(t *testing.T) {
	got, err := AllowedIPs(SplitExclude, "0.0.0.0/0")
	if err != nil {
		t.Fatalf("AllowedIPs: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("исключение всего адресного пространства не оставляет маршрутов, получено %s", FormatPrefixes(got))
	}
}

func TestParseSubnetsRejectsGarbage(t *testing.T) {
	for _, s := range []string{"не подсеть", "10.0.0.0/33", "2001:db8::/32"} {
		if _, err := ParseSubnets(s); err == nil {
			t.Errorf("%q должно давать ошибку", s)
		}
	}
}

func covers(list []netip.Prefix, addr netip.Addr) bool {
	for _, p := range list {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

const sampleConf = `# выдан сервером
[Interface]
PrivateKey = aaa=
Address = 10.13.13.2/32
DNS = 1.1.1.1
MTU = 1420
Jc = 4

[Peer]
PublicKey = bbb=
AllowedIPs = 0.0.0.0/0
Endpoint = 1.2.3.4:56000
`

func TestPrepare(t *testing.T) {
	out, err := Prepare(sampleConf, Options{Listen: "127.0.0.1:9000", SplitMode: SplitAll})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	c := Parse(out)

	if c.Endpoint() != "127.0.0.1:9000" {
		t.Errorf("Endpoint = %q, должен смотреть в локальный сокет ядра", c.Endpoint())
	}
	if c.InterfaceValue("MTU") != "1280" {
		t.Errorf("MTU = %q, want 1280", c.InterfaceValue("MTU"))
	}
	if c.InterfaceValue("PrivateKey") != "aaa=" || c.InterfaceValue("Address") != "10.13.13.2/32" {
		t.Error("исходные параметры интерфейса потерялись")
	}
	// Ключи AmneziaWG нам незнакомы, но обязаны дожить до файла.
	if c.InterfaceValue("Jc") != "4" {
		t.Error("незнакомые ключи должны сохраняться")
	}
	if !strings.Contains(out, "PersistentKeepalive = 25") {
		t.Error("через TURN нужен keepalive, иначе канал теряется")
	}
}

func TestPrepareSplitTunnel(t *testing.T) {
	out, err := Prepare(sampleConf, Options{
		Listen: "127.0.0.1:9100", SplitMode: SplitExclude, Subnets: "10.0.0.0/8",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	allowed := Parse(out).Peer
	var value string
	for _, l := range allowed {
		if strings.EqualFold(l.Key, "AllowedIPs") {
			value = l.Value
		}
	}
	if strings.Contains(value, "0.0.0.0/0") || !strings.Contains(value, "128.0.0.0/1") {
		t.Errorf("AllowedIPs = %q, ожидался список без 10.0.0.0/8", value)
	}
}

func TestPrepareWithoutInterface(t *testing.T) {
	if _, err := Prepare("[Peer]\nPublicKey = x", Options{}); err == nil {
		t.Error("конфиг без [Interface] нужно отвергать")
	}
}

func TestTemplate(t *testing.T) {
	conf, keys, err := Template(TemplateOptions{Endpoint: "127.0.0.1:9100", DNS: "1.1.1.1"})
	if err != nil {
		t.Fatalf("Template: %v", err)
	}
	if keys.Private == "" || keys.Public == "" || keys.Private == keys.Public {
		t.Fatalf("ключи выглядят неправдоподобно: %+v", keys)
	}

	c := Parse(conf)
	if c.InterfaceValue("PrivateKey") != keys.Private {
		t.Error("в конфиг попал не тот приватный ключ")
	}
	if c.InterfaceValue("MTU") != "1280" {
		t.Errorf("MTU = %q, want 1280", c.InterfaceValue("MTU"))
	}
	if c.InterfaceValue("Address") != DefaultClientAddress {
		t.Errorf("адрес = %q", c.InterfaceValue("Address"))
	}
	if c.Endpoint() != "127.0.0.1:9100" {
		t.Errorf("Endpoint = %q, туннель должен идти в сокет ядра", c.Endpoint())
	}
	if !strings.Contains(conf, "PersistentKeepalive = 25") {
		t.Error("через TURN нужен keepalive")
	}
	// Без ключа сервера файл остаётся заготовкой - это должно быть видно.
	if !strings.Contains(conf, "ВСТАВЬТЕ_ПУБЛИЧНЫЙ_КЛЮЧ_СЕРВЕРА") {
		t.Error("отсутствие ключа сервера должно быть явным")
	}
}

func TestTemplateKeysAreUnique(t *testing.T) {
	_, a, err := Template(TemplateOptions{})
	if err != nil {
		t.Fatalf("Template: %v", err)
	}
	_, b, err := Template(TemplateOptions{})
	if err != nil {
		t.Fatalf("Template: %v", err)
	}
	if a.Private == b.Private {
		t.Fatal("две заготовки получили одинаковый приватный ключ")
	}
}

func TestTemplateWithServerKey(t *testing.T) {
	const serverKey = "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg="
	conf, _, err := Template(TemplateOptions{ServerPublicKey: serverKey})
	if err != nil {
		t.Fatalf("Template: %v", err)
	}
	if !strings.Contains(conf, "PublicKey = "+serverKey) {
		t.Error("публичный ключ сервера не подставлен")
	}
	// Готовый конфиг обязан разбираться нашим же парсером туннеля.
	if _, err := tunnel.ParseConfig(conf); err != nil {
		t.Fatalf("созданный конфиг не разобрался: %v", err)
	}
}

func TestClientConf(t *testing.T) {
	conf := ClientConf(ClientConfOptions{
		PrivateKey:      "QFtZkBQ0d1wY0aWJ8Vd0AGTe0HeNiKJRnEHXbUYXV1c=",
		Address:         "10.13.13.5/32",
		ServerPublicKey: "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=",
		Endpoint:        "127.0.0.1:9000",
		DNS:             "1.1.1.1",
	})

	// Конфиг, собранный по данным сервера, обязан разбираться туннелем.
	cfg, err := tunnel.ParseConfig(conf)
	if err != nil {
		t.Fatalf("собранный конфиг не разобрался: %v", err)
	}
	if cfg.MTU != 1280 || len(cfg.Peers) != 1 {
		t.Errorf("неожиданные параметры: MTU=%d, узлов=%d", cfg.MTU, len(cfg.Peers))
	}
	if cfg.Peers[0].Keepalive != 25 {
		t.Error("через TURN нужен keepalive")
	}
	if !strings.Contains(conf, "Address = 10.13.13.5/32") {
		t.Error("адрес клиента с сервера не подставлен")
	}
}
