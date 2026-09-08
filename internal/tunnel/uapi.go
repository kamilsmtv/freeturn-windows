package tunnel

import (
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
)

// UAPI сериализует конфигурацию в текстовый протокол IPC amneziawg-go
// (device.IpcSet). Имена ключей - контракт с библиотекой, см. device/uapi.go.
func UAPI(c *Config) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}

	var b strings.Builder
	line := func(k, v string) {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v)
		b.WriteByte('\n')
	}

	line("private_key", hex.EncodeToString(c.PrivateKey[:]))
	// Порт исходящего сокета выбирает система: туннель стучится в локальный
	// сокет ядра, входящих подключений у него не бывает.
	line("listen_port", "0")
	writeAmnezia(line, c.Amnezia)
	line("replace_peers", "true")

	for i := range c.Peers {
		p := &c.Peers[i]
		line("public_key", hex.EncodeToString(p.PublicKey[:]))
		if !p.PresharedKey.IsZero() {
			line("preshared_key", hex.EncodeToString(p.PresharedKey[:]))
		}
		if p.Endpoint != "" {
			line("endpoint", p.Endpoint)
		}
		if p.Keepalive > 0 {
			line("persistent_keepalive_interval", strconv.Itoa(p.Keepalive))
		}
		line("replace_allowed_ips", "true")
		for _, ip := range p.AllowedIPs {
			line("allowed_ip", ip.String())
		}
	}
	return b.String(), nil
}

func writeAmnezia(line func(k, v string), p AmneziaParams) {
	if !p.Enabled() {
		return
	}

	for _, kv := range []struct {
		key string
		val int
	}{
		{"jc", p.Jc}, {"jmin", p.Jmin}, {"jmax", p.Jmax},
		{"s1", p.S1}, {"s2", p.S2}, {"s3", p.S3}, {"s4", p.S4},
	} {
		if kv.val > 0 {
			line(kv.key, strconv.Itoa(kv.val))
		}
	}

	for _, kv := range []struct{ key, val string }{
		{"h1", p.H1}, {"h2", p.H2}, {"h3", p.H3}, {"h4", p.H4},
		{"i1", p.I[0]}, {"i2", p.I[1]}, {"i3", p.I[2]}, {"i4", p.I[3]}, {"i5", p.I[4]},
		{"content_padding_addition", p.ContentPaddingAddition},
		{"rekey_after_time", p.RekeyAfterTime},
		{"rekey_timeout", p.RekeyTimeout},
		{"reject_after_time", p.RejectAfterTime},
		{"keepalive_timeout", p.KeepaliveTimeout},
		{"max_handshake_attempts", p.MaxHandshakeAttempts},
	} {
		if kv.val != "" {
			line(kv.key, kv.val)
		}
	}

	// Ключ защиты заголовка в conf-файле записан в base64, а IPC ждёт hex.
	if key := strings.TrimSpace(p.HeaderProtectionKey); key != "" {
		if raw, err := base64.StdEncoding.DecodeString(key); err == nil && len(raw) == KeyLen {
			line("header_protection_key", hex.EncodeToString(raw))
		} else {
			line("header_protection_key", key)
		}
	}
	if p.RandomTrailers {
		line("random_trailers", "true")
	}
	if p.DisableCookies {
		line("disable_cookies", "true")
	}
}
