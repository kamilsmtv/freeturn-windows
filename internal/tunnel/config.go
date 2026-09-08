// Package tunnel поднимает туннель WireGuard/AmneziaWG внутри приложения.
//
// У консольного клиента ядра встроенного туннеля нет (он есть только в
// мобильной сборке), поэтому на Windows туннель ведём сами: тот же
// amneziawg-go, что и в ядре, плюс адаптер Wintun. Внешний клиент
// WireGuard не нужен - это и есть паритет с Android-версией.
package tunnel

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// DefaultMTU - безопасный MTU для WireGuard поверх TURN: стандартные 1420
// фрагментируются на пути DTLS + TURN + UDP + IP. Совпадает с ядром и
// серверной частью.
const DefaultMTU = 1280

// KeyLen - длина ключа WireGuard.
const KeyLen = 32

// Key - 32-байтный ключ.
type Key [KeyLen]byte

// IsZero сообщает, что ключ не задан.
func (k Key) IsZero() bool { return k == Key{} }

// ParseKey читает ключ из base64, как он записан в .conf.
func ParseKey(s string) (Key, error) {
	var k Key
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return k, fmt.Errorf("ключ не в формате base64: %w", err)
	}
	if len(raw) != KeyLen {
		return k, fmt.Errorf("длина ключа %d байт вместо %d", len(raw), KeyLen)
	}
	copy(k[:], raw)
	return k, nil
}

// Peer - удалённый узел туннеля.
type Peer struct {
	PublicKey    Key
	PresharedKey Key
	AllowedIPs   []netip.Prefix
	Endpoint     string
	Keepalive    int
}

// Config - разобранная конфигурация туннеля.
type Config struct {
	PrivateKey Key
	Addresses  []netip.Prefix
	DNS        []netip.Addr
	MTU        int
	Peers      []Peer
	Amnezia    AmneziaParams
}

// AmneziaParams - параметры обфускации AmneziaWG. Пустая структура
// означает обычный WireGuard.
type AmneziaParams struct {
	Jc   int
	Jmin int
	Jmax int
	S1   int
	S2   int
	S3   int
	S4   int
	H1   string
	H2   string
	H3   string
	H4   string
	I    [5]string

	// Параметры AWG 3+.
	HeaderProtectionKey    string
	ContentPaddingAddition string
	RekeyAfterTime         string
	RekeyTimeout           string
	RejectAfterTime        string
	KeepaliveTimeout       string
	MaxHandshakeAttempts   string
	RandomTrailers         bool
	DisableCookies         bool
}

// Enabled сообщает, заданы ли параметры обфускации.
func (p AmneziaParams) Enabled() bool { return p != AmneziaParams{} }

// ParseConfig разбирает conf-файл WireGuard или AmneziaWG.
func ParseConfig(text string) (*Config, error) {
	cfg := &Config{MTU: DefaultMTU}
	section := ""

	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			if section == "peer" {
				cfg.Peers = append(cfg.Peers, Peer{})
			}
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("строка %d не похожа на параметр: %q", i+1, line)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		var err error
		switch section {
		case "interface":
			err = cfg.applyInterface(key, value)
		case "peer":
			err = cfg.applyPeer(key, value)
		default:
			// Параметры вне секций игнорируем: так же поступает wg-quick.
		}
		if err != nil {
			return nil, fmt.Errorf("строка %d (%s): %w", i+1, key, err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyInterface(key, value string) error {
	switch key {
	case "privatekey":
		k, err := ParseKey(value)
		if err != nil {
			return err
		}
		c.PrivateKey = k
	case "address":
		for _, part := range splitList(value) {
			p, err := netip.ParsePrefix(part)
			if err != nil {
				// Одиночный адрес без маски - тоже допустимая запись.
				addr, aerr := netip.ParseAddr(part)
				if aerr != nil {
					return fmt.Errorf("не удалось разобрать адрес %q", part)
				}
				p = netip.PrefixFrom(addr, addr.BitLen())
			}
			c.Addresses = append(c.Addresses, p)
		}
	case "dns":
		for _, part := range splitList(value) {
			addr, err := netip.ParseAddr(part)
			if err != nil {
				// Поисковые домены в DNS= допустимы и нам не нужны.
				continue
			}
			c.DNS = append(c.DNS, addr)
		}
	case "mtu":
		mtu, err := strconv.Atoi(value)
		if err != nil || mtu < 576 || mtu > 9000 {
			return fmt.Errorf("некорректный MTU %q", value)
		}
		c.MTU = mtu
	case "listenport", "table", "preup", "postup", "predown", "postdown", "saveconfig":
		// Параметры wg-quick, к самому туннелю отношения не имеющие.
	default:
		return c.Amnezia.apply(key, value)
	}
	return nil
}

func (c *Config) applyPeer(key, value string) error {
	if len(c.Peers) == 0 {
		return errors.New("параметр вне секции [Peer]")
	}
	p := &c.Peers[len(c.Peers)-1]

	switch key {
	case "publickey":
		k, err := ParseKey(value)
		if err != nil {
			return err
		}
		p.PublicKey = k
	case "presharedkey":
		k, err := ParseKey(value)
		if err != nil {
			return err
		}
		p.PresharedKey = k
	case "allowedips":
		for _, part := range splitList(value) {
			prefix, err := netip.ParsePrefix(part)
			if err != nil {
				return fmt.Errorf("не удалось разобрать AllowedIPs %q", part)
			}
			p.AllowedIPs = append(p.AllowedIPs, prefix)
		}
	case "endpoint":
		p.Endpoint = value
	case "persistentkeepalive":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("некорректный PersistentKeepalive %q", value)
		}
		p.Keepalive = n
	}
	return nil
}

// apply разбирает ключи AmneziaWG; неизвестные ключи игнорируются, чтобы
// новая версия сервера не ломала импорт конфига.
func (p *AmneziaParams) apply(key, value string) error {
	num := func(dst *int) error {
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("ожидалось число, получено %q", value)
		}
		*dst = n
		return nil
	}

	switch key {
	case "jc":
		return num(&p.Jc)
	case "jmin":
		return num(&p.Jmin)
	case "jmax":
		return num(&p.Jmax)
	case "s1":
		return num(&p.S1)
	case "s2":
		return num(&p.S2)
	case "s3":
		return num(&p.S3)
	case "s4":
		return num(&p.S4)
	case "h1":
		p.H1 = value
	case "h2":
		p.H2 = value
	case "h3":
		p.H3 = value
	case "h4":
		p.H4 = value
	case "i1", "i2", "i3", "i4", "i5":
		idx := int(key[1] - '1')
		p.I[idx] = value
	case "hp", "headerprotectionkey":
		p.HeaderProtectionKey = value
	case "cpa", "contentpaddingaddition":
		p.ContentPaddingAddition = value
	case "rekeyaftertime":
		p.RekeyAfterTime = value
	case "rekeytimeout":
		p.RekeyTimeout = value
	case "rejectaftertime":
		p.RejectAfterTime = value
	case "keepalivetimeout":
		p.KeepaliveTimeout = value
	case "maxhandshakeattempts":
		p.MaxHandshakeAttempts = value
	case "randomtrailers", "rt":
		p.RandomTrailers = isTrue(value)
	case "disablecookies", "dc":
		p.DisableCookies = isTrue(value)
	}
	return nil
}

// Validate проверяет минимум, без которого туннель не поднимется.
func (c *Config) Validate() error {
	if c.PrivateKey.IsZero() {
		return errors.New("в конфигурации нет PrivateKey")
	}
	if len(c.Addresses) == 0 {
		return errors.New("в конфигурации нет Address")
	}
	if len(c.Peers) == 0 {
		return errors.New("в конфигурации нет секции [Peer]")
	}
	for i := range c.Peers {
		if c.Peers[i].PublicKey.IsZero() {
			return errors.New("у узла [Peer] не задан PublicKey")
		}
		if len(c.Peers[i].AllowedIPs) == 0 {
			return errors.New("у узла [Peer] не заданы AllowedIPs")
		}
	}
	if c.MTU == 0 {
		c.MTU = DefaultMTU
	}
	return nil
}

func splitList(v string) []string {
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
