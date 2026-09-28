package netstat

import (
	"strings"
	"testing"
)

// Сокет на конкретном адресе подсказок не требует - он уже написан так,
// как его набирать.
func TestLANAddrsSkipsExplicitHost(t *testing.T) {
	if got := LANAddrs("192.168.1.5:1080", ""); got != nil {
		t.Errorf("LANAddrs = %v, ожидался пустой список", got)
	}
	if got := LANAddrs("127.0.0.1:1080", ""); got != nil {
		t.Errorf("LANAddrs = %v, ожидался пустой список", got)
	}
}

func TestLANAddrsBadInput(t *testing.T) {
	for _, in := range []string{"", "1080", "0.0.0.0", "0.0.0.0:порт"} {
		if got := LANAddrs(in, ""); got != nil {
			t.Errorf("LANAddrs(%q) = %v, ожидался пустой список", in, got)
		}
	}
}

// На 0.0.0.0 подсказки строятся по адресам машины; порт обязан совпадать
// с тем, что слушает сервер, иначе подсказка бесполезна.
func TestLANAddrsKeepsPort(t *testing.T) {
	for _, addr := range LANAddrs("0.0.0.0:1080", "") {
		if !strings.HasSuffix(addr, ":1080") {
			t.Errorf("адрес %q без нужного порта", addr)
		}
		if strings.HasPrefix(addr, "127.") {
			t.Errorf("localhost %q не годится соседу", addr)
		}
	}
}

// Первым показывается один адрес, и это должен быть адрес обычной сети,
// а не докера или виртуалки.
func TestLANAddrsOrder(t *testing.T) {
	got := order([]string{"172.17.0.1:1080", "10.8.0.2:1080", "192.168.1.42:1080"})
	if got[0] != "192.168.1.42:1080" {
		t.Errorf("первым идёт %q, ожидался адрес 192.168.x.x", got[0])
	}
	if got[len(got)-1] != "172.17.0.1:1080" {
		t.Errorf("последним идёт %q, ожидался адрес 172.x", got[len(got)-1])
	}
}

func TestSkipIface(t *testing.T) {
	cases := map[string]bool{
		"Ethernet":             false,
		"Беспроводная сеть":    false,
		"vEthernet (WSL)":      true,
		"Docker Desktop":       true,
		"VirtualBox Host-Only": true,
		"freeturn-wg":          true,
	}
	for name, want := range cases {
		if got := skipIface(name, "freeturn-wg"); got != want {
			t.Errorf("skipIface(%q) = %v, ожидалось %v", name, got, want)
		}
	}
}
