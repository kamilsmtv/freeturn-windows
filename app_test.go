package main

import (
	"strings"
	"testing"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// Служебные сообщения устройства идут сотнями на каждый подъём туннеля и
// вытесняют из журнала то, ради чего его открывают.
func TestTunnelLogFiltersNoise(t *testing.T) {
	noise := []string{
		"Routine: encryption worker 12 - started",
		"UAPI: Updating private key",
		"UDP bind has been updated",
	}
	keep := []string{
		"peer(HEsq…OWmo) - Received handshake response",
		"peer(HEsq…OWmo) - Handshake did not complete after 5 seconds, retrying (try 2)",
		"Interface state was Down, requested Up, now Up",
		"ошибка туннеля: something broke",
	}

	for _, line := range noise {
		if !isTunnelNoise(line) {
			t.Errorf("строка должна отсеиваться: %q", line)
		}
	}
	for _, line := range keep {
		if isTunnelNoise(line) {
			t.Errorf("строка должна попадать в журнал: %q", line)
		}
	}
}

// Раздача имеет смысл только поверх поднятого туннеля: в режиме прокси
// соседа заворачивать некуда, и включать сервер нельзя.
func TestWantsSocks(t *testing.T) {
	vpn := profile.New("vpn")
	vpn.Client.TunnelTransport = "wireguard"
	vpn.Client.WireGuardConfig = "[Interface]"
	vpn.Client.SocksHotspot = true
	if !wantsSocks(vpn) {
		t.Error("раздача не включилась в режиме VPN")
	}

	noConf := vpn
	noConf.Client.WireGuardConfig = ""
	if wantsSocks(noConf) {
		t.Error("раздача включилась без конфигурации WireGuard")
	}

	proxy := vpn
	proxy.Client.TunnelTransport = "none"
	if wantsSocks(proxy) {
		t.Error("раздача включилась в режиме прокси")
	}

	off := vpn
	off.Client.SocksHotspot = false
	if wantsSocks(off) {
		t.Error("раздача включилась при выключенном переключателе")
	}
}

// Правило брандмауэра нужно только сокету, доступному снаружи.
func TestLoopbackOnly(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:1080":   true,
		"[::1]:1080":       true,
		"0.0.0.0:1080":     false,
		"192.168.1.5:1080": false,
		"мусор":            false,
	}
	for addr, want := range cases {
		if got := loopbackOnly(addr); got != want {
			t.Errorf("loopbackOnly(%q) = %v, ожидалось %v", addr, got, want)
		}
	}
}

// Режим проброса tcp решает за пользователя: свой WireGuard скрипт
// поднимает только для udp, и выбор в профиле тут ничего не значит.
func TestExternalBackend(t *testing.T) {
	p := profile.New("сервер")
	if externalBackend(p) {
		t.Error("по умолчанию за релеем должен быть свой WireGuard")
	}

	p.ServerBackend = profile.ServerBackendExternal
	if !externalBackend(p) {
		t.Error("выбор своего VPN не сработал")
	}

	p.ServerBackend = profile.ServerBackendNew
	p.Opts.ProxyMode = profile.ModeTCP
	if !externalBackend(p) {
		t.Error("режим tcp обязан включать чужой бэкенд")
	}
}

// Профиль остаётся с мёртвой конфигурацией, когда сервер уходит на чужой
// VPN: об этом нужно сказать, иначе виноватым выглядит приложение.
func TestStaleTunnelNote(t *testing.T) {
	p := profile.New("сервер")
	if staleTunnelNote(p) != "" {
		t.Error("профилю без туннеля предупреждать не о чем")
	}

	p.Client.TunnelTransport = "wireguard"
	p.Client.WireGuardConfig = "[Interface]"
	if !strings.Contains(staleTunnelNote(p), "больше не подойдёт") {
		t.Errorf("нет предупреждения: %q", staleTunnelNote(p))
	}
}
