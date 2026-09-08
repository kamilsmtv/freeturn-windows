package tray

import (
	"os"
	"testing"
)

// Иконка выбирается по состоянию: цвет значка и есть индикатор.
func TestIconForState(t *testing.T) {
	tests := []struct {
		name  string
		state State
		light bool
		want  string
	}{
		{"подключено", State{Connected: true}, false, "tray.ico"},
		{"переход", State{Busy: true}, false, "tray-busy.ico"},
		{"переход на тёмной панели", State{Busy: true}, true, "tray-busy.ico"},
		{"подключено важнее перехода", State{Connected: true, Busy: true}, false, "tray.ico"},
		{"отключено на светлой панели", State{}, false, "tray-off.ico"},
		{"отключено на тёмной панели", State{}, true, "tray-off-light.ico"},
		{"ошибка", State{Failed: true}, false, "tray-error.ico"},
		{"ошибка важнее подключения", State{Connected: true, Failed: true}, false, "tray-error.ico"},
		// Цветные состояния читаются на любой панели и от темы не зависят.
		{"подключено на тёмной панели", State{Connected: true}, true, "tray.ico"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, err := os.ReadFile(tt.want)
			if err != nil {
				t.Fatalf("не прочитан %s: %v", tt.want, err)
			}
			if got := iconForState(tt.state, tt.light); len(got) != len(want) || string(got) != string(want) {
				t.Errorf("для состояния %+v выбрана не та иконка", tt.state)
			}
		})
	}
}

// Светлый и тёмный варианты «отключено» обязаны различаться: иначе смысл
// выбора по теме теряется, а заметить это в глаза трудно.
func TestIdleIconsDiffer(t *testing.T) {
	if string(iconIdle) == string(iconIdleLight) {
		t.Error("варианты значка «отключено» совпадают")
	}
}

// Каталог .ico содержит несколько размеров; нужный выбираем сами - система
// за нас этого не делает.
func TestPickIconEntry(t *testing.T) {
	ico, err := os.ReadFile("tray.ico")
	if err != nil {
		t.Fatalf("не прочитан tray.ico: %v", err)
	}

	entry, ok := pickIconEntry(ico, 16)
	if !ok || len(entry) == 0 {
		t.Fatal("изображение для 16 px не найдено")
	}
	// Записи в наших наборах - PNG.
	if string(entry[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Errorf("ожидался PNG, получено % x", entry[:8])
	}

	big, ok := pickIconEntry(ico, 32)
	if !ok {
		t.Fatal("изображение для 32 px не найдено")
	}
	if len(big) <= len(entry) {
		t.Error("для большего размера должно выбираться большее изображение")
	}
}

func TestPickIconEntryRejectsGarbage(t *testing.T) {
	for name, data := range map[string][]byte{
		"пусто":          nil,
		"обрезанный":     {0, 0, 1, 0},
		"не ico":         []byte("\x89PNG\r\n\x1a\n0000"),
		"нулевой размер": {0, 0, 1, 0, 1, 0, 16, 16, 0, 0, 1, 0, 32, 0, 0, 0, 0, 0, 22, 0, 0, 0},
	} {
		if _, ok := pickIconEntry(data, 16); ok {
			t.Errorf("%s: испорченные данные не должны давать изображение", name)
		}
	}
}
