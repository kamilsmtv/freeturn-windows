package main

import (
	"path/filepath"
	"testing"

	"github.com/kamilsmtv/freeturn-windows/internal/settings"
)

// newTestSettings открывает хранилище настроек во временном каталоге.
func newTestSettings(t *testing.T) *settings.Store {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	store, err := settings.Open()
	if err != nil {
		t.Fatalf("settings.Open: %v", err)
	}
	return store
}
