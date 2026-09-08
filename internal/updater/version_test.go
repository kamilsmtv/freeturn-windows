package updater

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"v1.2.3", "1.2.3", 0},
		{"1.10.0", "1.9.0", 1}, // числовое сравнение, не строковое
		{"1.9.0", "1.10.0", -1},
		{"2.0.0", "1.99.99", 1},
		{"1.2", "1.2.0", 0},       // недостающие части = 0
		{"1.2.0", "1.2.0-rc1", 1}, // релиз новее пререлиза
		{"1.2.0-rc1", "1.2.0-rc2", -1},
		{"dev", "1.0.0", -1},
		{"1.0.0", "", 1},
	}
	for _, tt := range tests {
		if got := CompareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("1.2.4", "1.2.3") {
		t.Error("1.2.4 новее 1.2.3")
	}
	if IsNewer("1.2.3", "1.2.3") {
		t.Error("одинаковые версии не повод обновляться")
	}
	if !IsNewer("1.0.0", "") {
		t.Error("неизвестная текущая версия - повод предложить обновление")
	}
}

func TestParseChecksums(t *testing.T) {
	body := `deadbeef  client-linux-amd64
abc123  client-windows-amd64.exe
99  *server-windows-amd64.exe
`
	if got := ParseChecksums(body, "client-windows-amd64.exe"); got != "abc123" {
		t.Errorf("сумма клиента = %q, want abc123", got)
	}
	if got := ParseChecksums(body, "server-windows-amd64.exe"); got != "99" {
		t.Errorf("префикс * должен отсекаться, получено %q", got)
	}
	if got := ParseChecksums(body, "нет-такого"); got != "" {
		t.Errorf("для отсутствующего файла ожидалась пустая строка, получено %q", got)
	}
}
