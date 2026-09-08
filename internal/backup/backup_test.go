package backup

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// Пароль от testdata/android-v4.json - конверта, собранного независимой
// реализацией по логике BackupCrypto.kt (PBKDF2-HMAC-SHA256 210k, AES-256-GCM).
const goldenPassword = "пароль123"

func goldenBackup(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/android-v4.json")
	if err != nil {
		t.Fatalf("не прочитан эталонный бэкап: %v", err)
	}
	return data
}

func TestImportAndroidBackup(t *testing.T) {
	d, err := Import(goldenBackup(t), goldenPassword)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if d.OwnClientID != "0123456789abcdef0123456789abcdef" {
		t.Errorf("client-id устройства = %q", d.OwnClientID)
	}
	if d.ActiveID != "srv-1" || len(d.Profiles) != 1 {
		t.Fatalf("ожидался один профиль и активный srv-1, получено %d / %q", len(d.Profiles), d.ActiveID)
	}

	p := d.Profiles[0]
	if p.ID != "srv-1" || p.Name != "RU-1" {
		t.Errorf("профиль прочитан неверно: %s / %s", p.ID, p.Name)
	}
	if p.Client.ServerAddress != "1.2.3.4:56000" || p.Client.Threads != 12 || p.Client.StreamsPerCred != 6 {
		t.Errorf("клиентские параметры разошлись: %+v", p.Client)
	}
	if p.SSH.Password != "secret" || p.SSH.HostFingerprint != "SHA256:abc" {
		t.Errorf("параметры SSH разошлись: %+v", p.SSH)
	}
	if p.Opts.ObfProfile != profile.ObfRtpOpus3 || p.Opts.ObfTimingMs != 20 {
		t.Errorf("опции обфускации разошлись: %+v", p.Opts)
	}
	if !strings.Contains(p.Client.WireGuardConfig, "[Interface]") {
		t.Error("конфиг WireGuard потерян")
	}
	// Платформа Android в бэкапе есть, на Windows профиль всегда desktop.
	if p.Client.Platform != profile.PlatformDesktop {
		t.Errorf("платформа = %q, want desktop", p.Client.Platform)
	}
	// Поля, которых нет в модели Windows, должны сохраниться нетронутыми.
	if _, ok := p.Extra["splitTunnelApps"]; !ok {
		t.Error("список приложений Android должен переноситься через Extra")
	}
	if _, ok := p.Extra["useCarrierDns"]; !ok {
		t.Error("useCarrierDns должен переноситься через Extra")
	}
}

func TestImportWrongPassword(t *testing.T) {
	if _, err := Import(goldenBackup(t), "не тот пароль"); !errors.Is(err, ErrBadPassword) {
		t.Fatalf("ожидалась ошибка пароля, получено %v", err)
	}
}

func TestImportNotABackup(t *testing.T) {
	if _, err := Import([]byte(`{"hello":"world"}`), "x"); !errors.Is(err, ErrFormat) {
		t.Fatalf("ожидалась ошибка формата, получено %v", err)
	}
}

func TestExportRoundTripPreservesAndroidFields(t *testing.T) {
	orig, err := Import(goldenBackup(t), goldenPassword)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	blob, err := Export(orig, "новый пароль")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	back, err := Import(blob, "новый пароль")
	if err != nil {
		t.Fatalf("повторный Import: %v", err)
	}

	if len(back.Profiles) != 1 {
		t.Fatalf("после круга профилей = %d", len(back.Profiles))
	}
	a, b := orig.Profiles[0], back.Profiles[0]
	if a.Client != b.Client || a.SSH != b.SSH || a.Opts != b.Opts || a.Name != b.Name || a.ID != b.ID {
		t.Errorf("профиль изменился после круга:\n%+v\n%+v", a, b)
	}
	if string(a.Extra["splitTunnelApps"]) != string(b.Extra["splitTunnelApps"]) {
		t.Error("поля Android потерялись при экспорте")
	}
	if back.Toggles["nerdMode"] != orig.Toggles["nerdMode"] {
		t.Error("тогглы интерфейса потерялись при экспорте")
	}
}

// Экспорт должен собирать полезную нагрузку, которую Android разберёт:
// версия 4, обязательный ownClientId и профили в раскладке ServerJson.
func TestExportPayloadShape(t *testing.T) {
	p := profile.New("test")
	p.Client.ServerAddress = "1.2.3.4:56000"
	d := Data{Profiles: []profile.Profile{p}, ActiveID: p.ID, OwnClientID: profile.NewClientID()}

	blob, err := Export(d, "pass")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	payload, err := Decrypt(blob, "pass")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(payload, &obj); err != nil {
		t.Fatalf("полезная нагрузка не разобралась: %v", err)
	}
	var v int
	_ = json.Unmarshal(obj["v"], &v)
	if v != FormatVersion {
		t.Errorf("версия формата = %d, want %d", v, FormatVersion)
	}
	for _, key := range []string{"servers", "ownClientId", "activeId", "dynamicTheme", "nerdMode"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("в бэкапе нет ключа %q, ожидаемого Android-клиентом", key)
		}
	}
}

func TestExportRequiresValidClientID(t *testing.T) {
	if _, err := Export(Data{OwnClientID: "мусор"}, "pass"); !errors.Is(err, ErrClientID) {
		t.Fatalf("ожидалась проверка client-id, получено %v", err)
	}
}
