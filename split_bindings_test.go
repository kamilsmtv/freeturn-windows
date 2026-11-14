package main

import (
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
