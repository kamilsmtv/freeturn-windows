package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/core"
)

// Имена файлов в релизе ядра (.goreleaser.yaml: формат binary, шаблон
// {{.Binary}}-{{.Os}}-{{.Arch}}) и локальные имена в каталоге ядра.
const (
	AssetName     = "client-windows-amd64.exe"
	ChecksumsName = "checksums.txt"

	binName     = "client.exe"
	prevBinName = "client.prev.exe"
	metaName    = "meta.json"
)

// Meta - что мы знаем об установленном ядре.
type Meta struct {
	Version     string    `json:"version"`
	SHA256      string    `json:"sha256"`
	InstalledAt time.Time `json:"installedAt"`
	PrevVersion string    `json:"prevVersion"`
	Source      string    `json:"source"`
}

// Status - состояние ядра и доступного обновления для UI.
type Status struct {
	Installed      bool   `json:"installed"`
	Version        string `json:"version"`
	LatestVersion  string `json:"latestVersion"`
	UpdateReady    bool   `json:"updateReady"`
	Changelog      string `json:"changelog"`
	ReleaseURL     string `json:"releaseUrl"`
	CanRollback    bool   `json:"canRollback"`
	RollbackTarget string `json:"rollbackTarget"`
	CheckedAt      string `json:"checkedAt"`
	Error          string `json:"error"`
}

// Progress - ход скачивания.
type Progress struct {
	Stage      string `json:"stage"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
}

// StopFunc останавливает ядро перед заменой файла и сообщает, было ли оно запущено.
type StopFunc func() (wasRunning bool, err error)

// StartFunc поднимает ядро обратно после успешной замены.
type StartFunc func() error

// CoreUpdater ставит, обновляет и откатывает бинарь ядра.
type CoreUpdater struct {
	Dir    string
	Repo   string
	GitHub *GitHub
	HTTP   *http.Client

	Stop  StopFunc
	Start StartFunc
}

// NewCoreUpdater создаёт апдейтер ядра для каталога dir.
func NewCoreUpdater(dir, repo string, gh *GitHub) *CoreUpdater {
	return &CoreUpdater{
		Dir:    dir,
		Repo:   repo,
		GitHub: gh,
		// Скачивание бинаря длиннее обычного запроса к API.
		HTTP: &http.Client{Timeout: 10 * time.Minute},
	}
}

// BinPath - путь к бинарю ядра.
func (u *CoreUpdater) BinPath() string { return filepath.Join(u.Dir, binName) }

// prevPath - путь к предыдущей версии, оставленной для отката.
func (u *CoreUpdater) prevPath() string { return filepath.Join(u.Dir, prevBinName) }

func (u *CoreUpdater) metaPath() string { return filepath.Join(u.Dir, metaName) }

// Installed сообщает, лежит ли бинарь ядра на месте.
func (u *CoreUpdater) Installed() bool {
	st, err := os.Stat(u.BinPath())
	return err == nil && st.Mode().IsRegular() && st.Size() > 0
}

// Meta читает сведения об установленном ядре.
func (u *CoreUpdater) Meta() Meta {
	var m Meta
	data, err := os.ReadFile(u.metaPath())
	if err != nil {
		return m
	}
	_ = json.Unmarshal(data, &m)
	return m
}

func (u *CoreUpdater) saveMeta(m Meta) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(u.metaPath(), data, 0o600)
}

// CurrentVersion возвращает версию установленного ядра. Основной источник -
// meta.json, но если его нет (бинарь положили руками), ядро опрашивается.
func (u *CoreUpdater) CurrentVersion(ctx context.Context) string {
	if !u.Installed() {
		return ""
	}
	if v := u.Meta().Version; v != "" {
		return v
	}
	v, err := core.ProbeVersion(ctx, u.BinPath())
	if err != nil {
		return ""
	}
	m := u.Meta()
	m.Version, m.Source = v, "probe"
	_ = u.saveMeta(m)
	return v
}

// Check спрашивает GitHub о последнем релизе и сравнивает с установленной версией.
func (u *CoreUpdater) Check(ctx context.Context, force bool) Status {
	st := Status{
		Installed:      u.Installed(),
		Version:        u.CurrentVersion(ctx),
		CanRollback:    u.canRollback(),
		RollbackTarget: u.Meta().PrevVersion,
		CheckedAt:      time.Now().Format(time.RFC3339),
	}

	rel, err := u.GitHub.LatestRelease(ctx, u.Repo, force)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.LatestVersion = NormalizeVersion(rel.TagName)
	st.Changelog = rel.Body
	st.ReleaseURL = rel.HTMLURL
	st.UpdateReady = IsNewer(st.LatestVersion, st.Version)
	return st
}

// ErrNoAsset - в релизе нет нужного файла.
var ErrNoAsset = fmt.Errorf("в релизе нет файла %s", AssetName)

// Install скачивает последний релиз и заменяет им бинарь ядра.
//
// Порядок: скачать во временный файл -> сверить SHA256 -> остановить ядро ->
// подменить файл (старый сохранить как client.prev.exe) -> убедиться, что
// новый бинарь запускается -> при неудаче вернуть прежний.
func (u *CoreUpdater) Install(ctx context.Context, force bool, onProgress func(Progress)) (Status, error) {
	report := func(stage string, done, total int64) {
		if onProgress != nil {
			onProgress(Progress{Stage: stage, Downloaded: done, Total: total})
		}
	}

	rel, err := u.GitHub.LatestRelease(ctx, u.Repo, force)
	if err != nil {
		return u.Check(ctx, false), err
	}
	asset, ok := rel.AssetByName(AssetName)
	if !ok {
		return u.Check(ctx, false), ErrNoAsset
	}

	if err := os.MkdirAll(u.Dir, 0o700); err != nil {
		return u.Check(ctx, false), err
	}
	tmp := filepath.Join(u.Dir, "client.download")
	defer func() { _ = os.Remove(tmp) }()

	report("download", 0, asset.Size)
	sum, err := u.download(ctx, asset, tmp, func(done int64) { report("download", done, asset.Size) })
	if err != nil {
		return u.Check(ctx, false), err
	}

	report("verify", asset.Size, asset.Size)
	if want, err := u.expectedSum(ctx, rel); err != nil {
		return u.Check(ctx, false), err
	} else if want != "" && !strings.EqualFold(want, sum) {
		return u.Check(ctx, false), fmt.Errorf("контрольная сумма не совпала: ожидалась %s, получена %s", want, sum)
	}

	report("swap", asset.Size, asset.Size)
	prevVersion := u.CurrentVersion(ctx)
	wasRunning := false
	if u.Stop != nil {
		if wasRunning, err = u.Stop(); err != nil {
			return u.Check(ctx, false), fmt.Errorf("не удалось остановить ядро перед обновлением: %w", err)
		}
	}

	if err := u.swap(tmp); err != nil {
		return u.Check(ctx, false), err
	}

	// Новый бинарь обязан хотя бы дойти до строки с версией. Не дошёл -
	// откатываемся, чтобы не остаться без рабочего ядра.
	newVersion, perr := core.ProbeVersion(ctx, u.BinPath())
	if perr != nil {
		_ = u.restorePrev()
		return u.Check(ctx, false), fmt.Errorf("новая версия ядра не запустилась, выполнен откат: %w", perr)
	}

	if err := u.saveMeta(Meta{
		Version:     newVersion,
		SHA256:      sum,
		InstalledAt: time.Now(),
		PrevVersion: prevVersion,
		Source:      rel.HTMLURL,
	}); err != nil {
		return u.Check(ctx, false), err
	}

	if wasRunning && u.Start != nil {
		if err := u.Start(); err != nil {
			return u.Check(ctx, false), fmt.Errorf("ядро обновлено, но не запустилось: %w", err)
		}
	}
	report("done", asset.Size, asset.Size)
	return u.Check(ctx, false), nil
}

// canRollback - есть ли сохранённая предыдущая сборка.
func (u *CoreUpdater) canRollback() bool {
	st, err := os.Stat(u.prevPath())
	return err == nil && st.Size() > 0
}

// Rollback возвращает предыдущую сборку ядра.
func (u *CoreUpdater) Rollback(ctx context.Context) (Status, error) {
	if !u.canRollback() {
		return u.Check(ctx, false), errors.New("предыдущая версия ядра не сохранена")
	}
	wasRunning := false
	if u.Stop != nil {
		var err error
		if wasRunning, err = u.Stop(); err != nil {
			return u.Check(ctx, false), err
		}
	}
	if err := u.restorePrev(); err != nil {
		return u.Check(ctx, false), err
	}

	m := u.Meta()
	version, err := core.ProbeVersion(ctx, u.BinPath())
	if err != nil {
		version = m.PrevVersion
	}
	_ = u.saveMeta(Meta{Version: version, InstalledAt: time.Now(), Source: "rollback"})

	if wasRunning && u.Start != nil {
		if err := u.Start(); err != nil {
			return u.Check(ctx, false), err
		}
	}
	return u.Check(ctx, false), nil
}

// swap подменяет бинарь: текущий уезжает в client.prev.exe, новый встаёт на его место.
func (u *CoreUpdater) swap(tmp string) error {
	bin, prev := u.BinPath(), u.prevPath()

	if u.Installed() {
		_ = os.Remove(prev)
		if err := os.Rename(bin, prev); err != nil {
			return fmt.Errorf("не удалось сохранить прежнюю версию ядра: %w", err)
		}
	}
	if err := os.Rename(tmp, bin); err != nil {
		// Файл на месте не появился - возвращаем прежний и сообщаем причину.
		_ = os.Rename(prev, bin)
		return fmt.Errorf("не удалось заменить бинарь ядра: %w", err)
	}
	return nil
}

// restorePrev возвращает сохранённую сборку на место рабочей.
func (u *CoreUpdater) restorePrev() error {
	bin, prev := u.BinPath(), u.prevPath()
	if _, err := os.Stat(prev); err != nil {
		return errors.New("предыдущая версия ядра не сохранена")
	}
	_ = os.Remove(bin)
	return os.Rename(prev, bin)
}

// download качает ассет во временный файл и попутно считает SHA256.
func (u *CoreUpdater) download(ctx context.Context, a Asset, dst string, onBytes func(int64)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.BrowserDownloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "FreeTurn-Windows")

	resp, err := u.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrOffline, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("скачивание не удалось: %s", resp.Status)
	}

	// 0700: файл предстоит запускать (на Windows права не значат ничего,
	// но на POSIX без бита x бинарь не стартует).
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	counter := &countingWriter{onWrite: onBytes}
	if _, err := io.Copy(io.MultiWriter(f, h, counter), resp.Body); err != nil {
		return "", fmt.Errorf("скачивание прервано: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// expectedSum достаёт сумму нужного файла из checksums.txt релиза.
// Файла нет - возвращает "", проверка пропускается.
func (u *CoreUpdater) expectedSum(ctx context.Context, rel Release) (string, error) {
	asset, ok := rel.AssetByName(ChecksumsName)
	if !ok {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "FreeTurn-Windows")
	resp, err := u.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("не удалось получить %s: %w", ChecksumsName, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("не удалось получить %s: %s", ChecksumsName, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return ParseChecksums(string(data), AssetName), nil
}

// ParseChecksums ищет сумму файла в формате sha256sum: "<hex>  <имя>".
func ParseChecksums(body, name string) string {
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		// Второе поле бывает с префиксом "*" (бинарный режим sha256sum).
		if strings.TrimPrefix(fields[1], "*") == name {
			return fields[0]
		}
	}
	return ""
}

// countingWriter считает байты, не храня их.
type countingWriter struct {
	n       int64
	onWrite func(int64)
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	if c.onWrite != nil {
		c.onWrite(c.n)
	}
	return len(p), nil
}
