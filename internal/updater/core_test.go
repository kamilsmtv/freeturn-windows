package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeCore - подделка бинаря ядра: печатает строку версии в stderr, как это
// делает cmd/client, и завершается.
func fakeCore(version string) []byte {
	return []byte("#!/bin/sh\necho '[INFO] Free Turn Proxy client version=" + version + "' >&2\n")
}

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// mockGitHub поднимает API и раздачу ассетов одного релиза.
type mockGitHub struct {
	*httptest.Server
	apiCalls  int32
	tag       string
	binary    []byte
	checksums string
	omitAsset bool
	etag      string
}

func newMockGitHub(t *testing.T, tag string, binary []byte) *mockGitHub {
	t.Helper()
	m := &mockGitHub{tag: tag, binary: binary, etag: `W/"` + tag + `"`}
	m.checksums = fmt.Sprintf("%s  %s\n", sha(binary), AssetName)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&m.apiCalls, 1)
		if r.Header.Get("If-None-Match") == m.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		assets := []Asset{
			{Name: ChecksumsName, BrowserDownloadURL: m.URL + "/dl/" + ChecksumsName},
		}
		if !m.omitAsset {
			assets = append([]Asset{{
				Name:               AssetName,
				Size:               int64(len(m.binary)),
				BrowserDownloadURL: m.URL + "/dl/" + AssetName,
			}}, assets...)
		}
		w.Header().Set("ETag", m.etag)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Release{
			TagName: m.tag,
			Body:    "## Что нового\n- починили всё",
			HTMLURL: "https://example.invalid/release/" + m.tag,
			Assets:  assets,
		})
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ChecksumsName):
			_, _ = w.Write([]byte(m.checksums))
		default:
			_, _ = w.Write(m.binary)
		}
	})
	m.Server = httptest.NewServer(mux)
	t.Cleanup(m.Close)
	return m
}

func newTestUpdater(t *testing.T, m *mockGitHub) *CoreUpdater {
	t.Helper()
	dir := t.TempDir()
	gh := NewGitHub(dir, nil)
	gh.APIBase = m.URL
	u := NewCoreUpdater(dir, "owner/repo", gh)
	u.HTTP = m.Client()
	return u
}

func requireScriptSupport(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("подделка ядра - shell-скрипт; на Windows её не запустить")
	}
}

func TestInstallFreshDownload(t *testing.T) {
	requireScriptSupport(t)
	bin := fakeCore("1.4.2")
	m := newMockGitHub(t, "v1.4.2", bin)
	u := newTestUpdater(t, m)

	var lastProgress Progress
	st, err := u.Install(context.Background(), true, func(p Progress) { lastProgress = p })
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !u.Installed() {
		t.Fatal("бинарь ядра не появился")
	}
	got, _ := os.ReadFile(u.BinPath())
	if string(got) != string(bin) {
		t.Error("содержимое бинаря не совпало со скачанным")
	}
	if st.Version != "1.4.2" {
		t.Errorf("версия = %q, want 1.4.2", st.Version)
	}
	if u.Meta().SHA256 != sha(bin) {
		t.Error("в meta.json не записана контрольная сумма")
	}
	if lastProgress.Stage != "done" {
		t.Errorf("последний этап = %q, want done", lastProgress.Stage)
	}
}

func TestInstallRejectsBadChecksum(t *testing.T) {
	requireScriptSupport(t)
	m := newMockGitHub(t, "v1.4.2", fakeCore("1.4.2"))
	m.checksums = "0000  " + AssetName + "\n"
	u := newTestUpdater(t, m)

	if _, err := u.Install(context.Background(), true, nil); err == nil {
		t.Fatal("несовпадение SHA256 должно останавливать установку")
	}
	if u.Installed() {
		t.Error("после неудачной проверки бинарь не должен появляться")
	}
}

func TestInstallKeepsPreviousAndRollsBack(t *testing.T) {
	requireScriptSupport(t)
	old := fakeCore("1.0.0")
	m := newMockGitHub(t, "v1.0.0", old)
	u := newTestUpdater(t, m)
	if _, err := u.Install(context.Background(), true, nil); err != nil {
		t.Fatalf("первая установка: %v", err)
	}

	// Новый релиз: та же подделка, но другой версии.
	newBin := fakeCore("2.0.0")
	m.tag, m.binary, m.etag = "v2.0.0", newBin, `W/"v2.0.0"`
	m.checksums = fmt.Sprintf("%s  %s\n", sha(newBin), AssetName)

	stopped, started := false, false
	u.Stop = func() (bool, error) { stopped = true; return true, nil }
	u.Start = func() error { started = true; return nil }

	st, err := u.Install(context.Background(), true, nil)
	if err != nil {
		t.Fatalf("обновление: %v", err)
	}
	if !stopped || !started {
		t.Errorf("ядро должно быть остановлено и поднято обратно: stop=%v start=%v", stopped, started)
	}
	if st.Version != "2.0.0" {
		t.Errorf("версия после обновления = %q, want 2.0.0", st.Version)
	}
	if !st.CanRollback || st.RollbackTarget != "1.0.0" {
		t.Errorf("должен быть доступен откат на 1.0.0, получено %+v", st)
	}

	back, err := u.Rollback(context.Background())
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if back.Version != "1.0.0" {
		t.Errorf("после отката версия = %q, want 1.0.0", back.Version)
	}
	if got, _ := os.ReadFile(u.BinPath()); string(got) != string(old) {
		t.Error("после отката на месте должен лежать прежний бинарь")
	}
}

func TestInstallRollsBackWhenNewCoreDoesNotStart(t *testing.T) {
	requireScriptSupport(t)
	old := fakeCore("1.0.0")
	m := newMockGitHub(t, "v1.0.0", old)
	u := newTestUpdater(t, m)
	if _, err := u.Install(context.Background(), true, nil); err != nil {
		t.Fatalf("первая установка: %v", err)
	}

	// Битая сборка: запускается, но версию не печатает.
	broken := []byte("#!/bin/sh\nexit 1\n")
	m.tag, m.binary, m.etag = "v2.0.0", broken, `W/"v2.0.0"`
	m.checksums = fmt.Sprintf("%s  %s\n", sha(broken), AssetName)
	u.Stop = func() (bool, error) { return false, nil }

	if _, err := u.Install(context.Background(), true, nil); err == nil {
		t.Fatal("неработающая сборка должна приводить к ошибке")
	}
	if got, _ := os.ReadFile(u.BinPath()); string(got) != string(old) {
		t.Error("после неудачного старта должен вернуться прежний бинарь")
	}
}

func TestLatestReleaseUsesETagAndCache(t *testing.T) {
	m := newMockGitHub(t, "v1.4.2", fakeCore("1.4.2"))
	dir := t.TempDir()
	gh := NewGitHub(dir, nil)
	gh.APIBase, gh.HTTP = m.URL, m.Client()

	if _, err := gh.LatestRelease(context.Background(), "owner/repo", false); err != nil {
		t.Fatalf("первый запрос: %v", err)
	}
	// Второй запрос без force берёт кэш и в сеть не идёт.
	if _, err := gh.LatestRelease(context.Background(), "owner/repo", false); err != nil {
		t.Fatalf("второй запрос: %v", err)
	}
	if n := atomic.LoadInt32(&m.apiCalls); n != 1 {
		t.Errorf("к API должно уйти 1 обращение, ушло %d", n)
	}

	// force идёт в сеть, но получает 304 и отдаёт сохранённый релиз.
	rel, err := gh.LatestRelease(context.Background(), "owner/repo", true)
	if err != nil {
		t.Fatalf("принудительный запрос: %v", err)
	}
	if rel.TagName != "v1.4.2" {
		t.Errorf("по 304 должен вернуться закэшированный релиз, получен %q", rel.TagName)
	}
	if n := atomic.LoadInt32(&m.apiCalls); n != 2 {
		t.Errorf("ожидалось 2 обращения к API, было %d", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "github-cache.json")); err != nil {
		t.Error("кэш ответов не сохранён на диск")
	}
}

func TestInstallWithoutAsset(t *testing.T) {
	m := newMockGitHub(t, "v1.4.2", fakeCore("1.4.2"))
	m.omitAsset = true
	u := newTestUpdater(t, m)

	if _, err := u.Install(context.Background(), true, nil); err == nil {
		t.Fatal("релиз без нужного файла должен давать ошибку")
	}
}
