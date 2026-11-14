package main

import (
	"encoding/base64"
	"testing"

	"github.com/kamilsmtv/freeturn-windows/internal/link"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/vps"
)

const testConf = "[Interface]\nPrivateKey = a\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = b\n"

func serverLink(t *testing.T, cid string, withConf bool) string {
	t.Helper()
	l := link.Link{V: link.Version, Provider: "vk", Peer: "1.2.3.4:56000", ClientID: cid, Name: "guest"}
	if withConf {
		l = l.WithWGConf(testConf)
	}
	return l.Encode()
}

func TestGuestLinkKeepsCompleteServerLink(t *testing.T) {
	cid := profile.NewClientID()
	raw := serverLink(t, cid, true)
	got := guestLink(profile.New("p"), "guest", vps.ClientData{Link: raw})
	if got != raw {
		t.Error("полную ссылку сервера пересобирать нельзя")
	}
}

// Сервер отдал ссылку без client-id: без него гость не пройдёт allowlist.
func TestGuestLinkAddsMissingClientID(t *testing.T) {
	cid := profile.NewClientID()
	d := vps.ClientData{Link: serverLink(t, "", true)}
	d.Client.ClientID = cid
	l, err := link.Parse(guestLink(profile.New("p"), "guest", d))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if l.ClientID != cid {
		t.Errorf("client-id = %q, want %q", l.ClientID, cid)
	}
	if l.WGConf == "" {
		t.Error("конфигурация из ссылки сервера потерялась")
	}
}

func TestGuestLinkAddsConfig(t *testing.T) {
	cid := profile.NewClientID()
	d := vps.ClientData{Link: serverLink(t, cid, false), ConfB64: base64.StdEncoding.EncodeToString([]byte(testConf))}
	l, err := link.Parse(guestLink(profile.New("p"), "guest", d))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if l.ClientID != cid || l.WGConf == "" {
		t.Errorf("ссылка неполная: cid=%q conf=%v", l.ClientID, l.WGConf != "")
	}
}
