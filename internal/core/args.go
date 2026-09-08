// Package core запускает бинарь ядра free-turn-proxy и следит за ним.
package core

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// BuildArgs собирает командную строку клиента ядра.
//
// Правило то же, что у ClientArgs ядра (internal/config/args.go): флаг,
// равный дефолту, не пишется - так короче лог и меньше шансов разойтись с
// дефолтами новой версии. Отличие одно: -routes ClientArgs не эмитит, а
// GUI им управляет явно.
func BuildArgs(p profile.Profile) []string {
	c, o := p.Client, p.Opts
	var args []string
	add := func(flag string, value ...string) { args = append(args, append([]string{flag}, value...)...) }

	add("-peer", strings.TrimSpace(c.ServerAddress))

	if link := strings.TrimSpace(c.VKLink); link != "" {
		add("-links", link)
	}
	if c.Provider != "" && c.Provider != profile.ProviderVK {
		add("-provider", c.Provider)
	}
	if listen := strings.TrimSpace(c.LocalPort); listen != "" && listen != profile.DefaultListen {
		add("-listen", listen)
	}
	if c.MagicSwitch {
		if turn := strings.TrimSpace(c.MagicTurn); turn != "" {
			add("-turn", turn)
		}
		if port := strings.TrimSpace(c.MagicPort); port != "" {
			add("-port", port)
		}
	}
	if c.Threads > 0 && c.Threads != profile.DefaultN {
		add("-n", strconv.Itoa(c.Threads))
	}
	if c.StreamsPerCred > 0 && c.StreamsPerCred != profile.DefaultStreamsPerCred {
		add("-streams-per-cred", strconv.Itoa(c.StreamsPerCred))
	}
	// Дефолт транспорта - tcp, поэтому пишем только udp.
	if c.UseUDP {
		add("-transport", profile.TransportUDP)
	}
	if o.TCPMode() {
		add("-mode", profile.ModeTCP)
		args = append(args, kcpArgs(o.KCP)...)
	}
	if o.ObfEnabled() {
		add("-obf-profile", o.ObfProfile)
		add("-obf-key", o.ObfKey)
	}
	// Без профиля ядро отвергает ненулевой пейсинг.
	if o.ObfEnabled() && o.ObfTimingMs > 0 {
		add("-obf-timing", fmt.Sprintf("%dms", o.ObfTimingMs))
	}
	if c.ManualCaptcha {
		args = append(args, "-manual-captcha")
	}
	if c.Platform != "" && c.Platform != profile.PlatformDesktop {
		add("-platform", c.Platform)
	}
	if c.DNSMode != "" && c.DNSMode != profile.DNSAuto {
		add("-dns-mode", c.DNSMode)
	}
	if dns := normalizeList(c.CustomDNS); dns != "" {
		add("-dns-servers", dns)
	}
	if c.ClientID != "" {
		add("-client-id", c.ClientID)
	}
	if c.Routes {
		args = append(args, "-routes")
	}
	if c.DebugMode {
		args = append(args, "-debug")
	}
	return args
}

// kcpArgs пишет только те -kcp-*, что отличаются от дефолта ядра.
func kcpArgs(k profile.KCP) []string {
	def := profile.DefaultKCP()
	var args []string
	num := func(flag string, v, d int) {
		if v != d {
			args = append(args, flag, strconv.Itoa(v))
		}
	}
	num("-kcp-nodelay", k.NoDelay, def.NoDelay)
	num("-kcp-interval", k.Interval, def.Interval)
	num("-kcp-resend", k.Resend, def.Resend)
	num("-kcp-nc", k.NC, def.NC)
	num("-kcp-sndwnd", k.SndWnd, def.SndWnd)
	num("-kcp-rcvwnd", k.RcvWnd, def.RcvWnd)
	num("-kcp-mtu", k.MTU, def.MTU)
	if k.ACKNoDelay != def.ACKNoDelay {
		// Булев флаг ядра принимает значение только через "=" .
		args = append(args, "-kcp-acknodelay="+strconv.FormatBool(k.ACKNoDelay))
	}
	return args
}

// normalizeList приводит список через запятую к виду без пробелов и пустых элементов.
func normalizeList(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\r' || r == '\t' || r == ';'
	})
	return strings.Join(parts, ",")
}

var hostPortRe = regexp.MustCompile(`^[\w.\-]+:\d{1,5}$`)

// ValidHostPort повторяет проверку ядра: IPv6 в скобках оно не принимает.
func ValidHostPort(v string) bool {
	if !hostPortRe.MatchString(v) {
		return false
	}
	port, err := strconv.Atoi(v[strings.LastIndex(v, ":")+1:])
	return err == nil && port >= 1 && port <= 65535
}

// Validate проверяет профиль до запуска, чтобы не ловить ошибку от ядра постфактум.
func Validate(p profile.Profile) error {
	c, o := p.Client, p.Opts

	if strings.TrimSpace(c.ServerAddress) == "" {
		return errors.New("не задан адрес сервера (peer)")
	}
	if !ValidHostPort(strings.TrimSpace(c.ServerAddress)) {
		return errors.New("адрес сервера должен быть вида host:port, например 1.2.3.4:56000")
	}
	if listen := strings.TrimSpace(c.LocalPort); listen != "" && !ValidHostPort(listen) {
		return errors.New("локальный адрес должен быть вида ip:port, например 127.0.0.1:9000")
	}
	if c.Provider == profile.ProviderVK && strings.TrimSpace(c.VKLink) == "" {
		return errors.New("не указана ссылка на звонок VK Calls")
	}
	if o.ObfEnabled() && !profile.ValidObfKey(o.ObfKey) {
		return errors.New("ключ обфускации должен быть 64 hex-символа; профиль и ключ обязаны совпадать с сервером")
	}
	if o.ObfTimingMs < 0 || o.ObfTimingMs > profile.ObfTimingMax {
		return fmt.Errorf("пейсинг мимикрии допустим в диапазоне 0..%d мс", profile.ObfTimingMax)
	}
	if c.ClientID != "" && !profile.ValidClientID(c.ClientID) {
		return errors.New("client-id должен быть 32 hex-символа")
	}
	if o.TCPMode() && !o.KCP.Valid() {
		return errors.New("параметры KCP вне допустимых диапазонов (MTU 300..1350, окна > 0)")
	}
	return nil
}
