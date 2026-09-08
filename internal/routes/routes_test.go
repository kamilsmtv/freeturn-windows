package routes

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

// Пиннить нужно только публичные адреса: локальные и служебные и так идут
// мимо туннеля, а лишний маршрут к ним может сломать локальную сеть.
func TestUsableForPin(t *testing.T) {
	tests := map[string]bool{
		"8.8.8.8":       true,
		"77.88.8.8":     true,
		"87.240.190.78": true,
		"192.168.31.1":  false,
		"10.13.13.1":    false,
		"172.16.0.1":    false,
		"127.0.0.1":     false,
		"169.254.1.1":   false,
		"224.0.0.1":     false,
		"0.0.0.0":       false,
	}
	for addr, want := range tests {
		if got := usableForPin(netip.MustParseAddr(addr)); got != want {
			t.Errorf("usableForPin(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestUsableForPinSkipsIPv6(t *testing.T) {
	// Туннель ведём только по IPv4, маршруты IPv6 нам не нужны.
	if usableForPin(netip.MustParseAddr("2001:db8::1")) {
		t.Error("адреса IPv6 пиннить не нужно")
	}
}

func TestCoreRouteFromLog(t *testing.T) {
	line := "2026/09/07 20:29:28 [INFO] Ensuring route to 91.231.135.143/32 via 192.168.31.1"
	addr, ok := CoreRouteFromLog(line)
	if !ok || addr.String() != "91.231.135.143" {
		t.Fatalf("адрес маршрута ядра разобран неверно: %v, %v", addr, ok)
	}
	if _, ok := CoreRouteFromLog("[INFO] provider=vk"); ok {
		t.Error("обычная строка журнала не должна давать адрес")
	}
}

// Маршруты должны переживать аварийное завершение: при taskkill ни мы, ни
// ядро прибраться не успеваем, и список нужен на следующем запуске.
func TestPinnerPersistsAdopted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pinned.json")

	p := NewPinner(path, nil)
	p.Adopt(netip.MustParseAddr("91.231.135.143"))
	p.Adopt(netip.MustParseAddr("90.156.236.94"))
	// Локальные адреса не наши: их пиннить незачем.
	p.Adopt(netip.MustParseAddr("192.168.31.1"))

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("список маршрутов не сохранён: %v", err)
	}

	next := NewPinner(path, nil)
	list := next.loadState()
	if len(list) != 2 {
		t.Fatalf("прочитано маршрутов: %d, ожидалось 2 (%v)", len(list), list)
	}

	next.CleanStale()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("после уборки список должен удаляться")
	}
}

func TestPinnerWithoutStatePath(t *testing.T) {
	p := NewPinner("", nil)
	p.Adopt(netip.MustParseAddr("8.8.8.8"))
	p.CleanStale()
	p.Release()
}
