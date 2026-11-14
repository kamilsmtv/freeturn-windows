package main

import (
	"net/netip"
	"testing"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/settings"
)

func TestSplitFor(t *testing.T) {
	p := profile.New("p")
	p.Client.SplitTunnelMode = "include"
	p.Client.SplitTunnelSubnets = "10.0.0.0/8"
	shared := &settings.SplitRules{Mode: "exclude", Subnets: "192.168.0.0/16"}

	if m, s := splitFor(p, shared); m != "include" || s != "10.0.0.0/8" {
		t.Errorf("без подписки взяты чужие правила: %s %s", m, s)
	}
	p.Client.SplitTunnelShared = true
	if m, s := splitFor(p, shared); m != "exclude" || s != "192.168.0.0/16" {
		t.Errorf("подписанный профиль не взял общие правила: %s %s", m, s)
	}
	if m, s := splitFor(p, nil); m != "include" || s != "10.0.0.0/8" {
		t.Errorf("без набора должны остаться свои правила: %s %s", m, s)
	}
}

func TestRoutedIntoTunnel(t *testing.T) {
	routes := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	if !routedIntoTunnel(&routes, netip.MustParseAddr("10.1.2.3")) {
		t.Error("адрес внутри подсети туннеля признан внешним")
	}
	if routedIntoTunnel(&routes, netip.MustParseAddr("142.250.1.1")) {
		t.Error("адрес вне подсетей туннеля признан туннельным")
	}
	if !routedIntoTunnel(nil, netip.MustParseAddr("142.250.1.1")) {
		t.Error("без известных маршрутов замер не должен уходить в «мимо туннеля»")
	}
}
