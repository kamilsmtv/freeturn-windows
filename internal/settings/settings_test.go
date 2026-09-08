package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	s, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s, dir
}

func TestDefaultsAndSave(t *testing.T) {
	s, _ := tempStore(t)

	cur := s.Get()
	if cur.Theme != ThemeSystem || cur.CoreUpdateMode != UpdateAsk || cur.CoreUpdateHours != 6 {
		t.Errorf("неожиданные значения по умолчанию: %+v", cur)
	}
	if cur.Subscriptions == nil {
		t.Error("список подписок должен быть пустым срезом, а не nil")
	}

	cur.Theme = "нечто"
	cur.CoreUpdateHours = 0
	if err := s.Save(cur); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Негодные значения приводятся к допустимым, иначе интерфейс покажет пустоту.
	if got := s.Get(); got.Theme != ThemeSystem || got.CoreUpdateHours != 6 {
		t.Errorf("значения не нормализованы: %+v", got)
	}
}

// Повреждённый файл не должен мешать запуску: без этого одна испорченная
// запись лишает пользователя доступа к приложению целиком.
func TestOpenSurvivesBrokenFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := filepath.Join(dir, "FreeTurn", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{это не json"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Open()
	if err != nil {
		t.Fatalf("испорченный файл не должен давать ошибку запуска: %v", err)
	}
	if s.Get().Theme != ThemeSystem {
		t.Error("ожидались значения по умолчанию")
	}
	if !strings.Contains(s.Warning(), "повреждён") {
		t.Errorf("пользователь должен узнать о проблеме: %q", s.Warning())
	}
	if _, err := os.Stat(path + ".broken"); err != nil {
		t.Error("прежний файл должен сохраняться для разбора")
	}
}
