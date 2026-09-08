package wg

import (
	"fmt"
	"strings"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// MTU - константа туннеля: WG идёт поверх TURN (STUN-обёртка, UDP, IP),
// дефолтные 1420 фрагментируются и теряются. 1280 - минимум IPv6, живёт
// везде. То же значение держат сервер (WG_MTU в install.sh) и Android.
const MTU = profile.WGMTU

// Config - разобранный конфиг WireGuard.
type Config struct {
	// Interface и Peer хранят строки секций в исходном порядке: чужие
	// ключи (AmneziaWG: Jc, Jmin, S1, H1 и прочие) должны дожить до файла.
	Interface []Line
	Peer      []Line
}

// Line - строка конфига: пара ключ-значение либо сырой текст.
type Line struct {
	Key   string
	Value string
	Raw   string
}

// Parse разбирает conf-файл WireGuard. Комментарии и неизвестные ключи
// сохраняются как есть.
func Parse(conf string) Config {
	var c Config
	section := ""

	for _, raw := range strings.Split(conf, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}

		l := Line{Raw: line}
		if key, value, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, ";") {
			l.Key = strings.TrimSpace(key)
			l.Value = strings.TrimSpace(value)
		}
		switch section {
		case "interface":
			c.Interface = append(c.Interface, l)
		case "peer":
			c.Peer = append(c.Peer, l)
		}
	}
	return c
}

// Get возвращает значение ключа без учёта регистра.
func get(lines []Line, key string) string {
	for _, l := range lines {
		if strings.EqualFold(l.Key, key) {
			return l.Value
		}
	}
	return ""
}

// set заменяет значение ключа или добавляет строку, если ключа не было.
func set(lines []Line, key, value string) []Line {
	for i, l := range lines {
		if strings.EqualFold(l.Key, key) {
			lines[i].Key, lines[i].Value, lines[i].Raw = key, value, ""
			return lines
		}
	}
	return append(lines, Line{Key: key, Value: value})
}

// Options - что подставить в конфиг при подготовке к работе через ядро.
type Options struct {
	Listen     string
	SplitMode  string
	Subnets    string
	DNSServers string
}

// Prepare правит конфиг под работу через локальный сокет ядра: Endpoint
// смотрит в -listen клиента, MTU опускается до 1280, AllowedIPs считается
// по правилам раздельного туннелирования.
func Prepare(conf string, o Options) (string, error) {
	c := Parse(conf)
	if len(c.Interface) == 0 {
		return "", fmt.Errorf("в конфиге нет секции [Interface]")
	}

	listen := strings.TrimSpace(o.Listen)
	if listen == "" {
		listen = profile.DefaultListen
	}

	c.Interface = set(c.Interface, "MTU", fmt.Sprintf("%d", MTU))
	if dns := strings.TrimSpace(o.DNSServers); dns != "" {
		c.Interface = set(c.Interface, "DNS", dns)
	}

	if len(c.Peer) > 0 {
		// Весь смысл прокси: WireGuard стучится в локальный сокет ядра,
		// а не напрямую на VPS.
		c.Peer = set(c.Peer, "Endpoint", listen)
		allowed, err := AllowedIPs(o.SplitMode, o.Subnets)
		if err != nil {
			return "", err
		}
		c.Peer = set(c.Peer, "AllowedIPs", FormatPrefixes(allowed))
		if get(c.Peer, "PersistentKeepalive") == "" {
			// Через TURN-релей молчащий пир быстро теряет привязку канала.
			c.Peer = set(c.Peer, "PersistentKeepalive", "25")
		}
	}
	return c.String(), nil
}

// String собирает конфиг обратно в текст.
func (c Config) String() string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	writeLines(&b, c.Interface)
	if len(c.Peer) > 0 {
		b.WriteString("\n[Peer]\n")
		writeLines(&b, c.Peer)
	}
	return b.String()
}

func writeLines(b *strings.Builder, lines []Line) {
	for _, l := range lines {
		if l.Key == "" {
			b.WriteString(l.Raw + "\n")
			continue
		}
		b.WriteString(l.Key + " = " + l.Value + "\n")
	}
}

// Endpoint возвращает Endpoint из секции [Peer].
func (c Config) Endpoint() string { return get(c.Peer, "Endpoint") }

// InterfaceValue возвращает значение ключа секции [Interface].
func (c Config) InterfaceValue(key string) string { return get(c.Interface, key) }
