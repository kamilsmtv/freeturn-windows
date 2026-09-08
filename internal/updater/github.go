package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultAPIBase - точка входа GitHub API. Переопределяется в тестах.
const DefaultAPIBase = "https://api.github.com"

// Release - то, что нам нужно от релиза GitHub.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

// Asset - файл, приложенный к релизу.
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// AssetByName ищет ассет по точному имени.
func (r Release) AssetByName(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if strings.EqualFold(a.Name, name) {
			return a, true
		}
	}
	return Asset{}, false
}

// ErrRateLimited - GitHub отказал из-за лимита запросов.
var ErrRateLimited = errors.New("превышен лимит запросов к GitHub API; укажите токен в настройках или повторите позже")

// ErrOffline - до GitHub не достучаться (нет сети, прокси, DNS).
var ErrOffline = errors.New("нет связи с GitHub")

// GitHub - клиент релизов с кэшем ответов и поддержкой ETag.
type GitHub struct {
	APIBase   string
	HTTP      *http.Client
	Token     func() string
	CachePath string

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	ETag      string    `json:"etag"`
	FetchedAt time.Time `json:"fetchedAt"`
	Release   Release   `json:"release"`
}

// NewGitHub создаёт клиент. cacheDir - каталог, где лежит файл кэша ответов.
func NewGitHub(cacheDir string, token func() string) *GitHub {
	if token == nil {
		token = func() string { return "" }
	}
	return &GitHub{
		APIBase: DefaultAPIBase,
		// Таймаут общий на запрос: без сети запрос должен падать быстро,
		// а не держать проверку обновлений на старте.
		HTTP:      &http.Client{Timeout: 20 * time.Second},
		Token:     token,
		CachePath: filepath.Join(cacheDir, "github-cache.json"),
		cache:     map[string]cacheEntry{},
	}
}

// MinInterval - раньше этого срока повторно ходить в API незачем: даже при
// 304 ответ стоит одного запроса из лимита.
const MinInterval = 15 * time.Minute

// LatestRelease отдаёт последний релиз репозитория owner/repo.
//
// Сначала работает кэш (свежий ответ моложе MinInterval отдаётся сразу),
// затем условный запрос с If-None-Match: 304 не тратит лимит GitHub.
func (g *GitHub) LatestRelease(ctx context.Context, repo string, force bool) (Release, error) {
	g.loadCache()

	g.mu.Lock()
	entry, hasCache := g.cache[repo]
	g.mu.Unlock()

	if hasCache && !force && time.Since(entry.FetchedAt) < MinInterval {
		return entry.Release, nil
	}

	url := fmt.Sprintf("%s/repos/%s/releases/latest", strings.TrimRight(g.APIBase, "/"), repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "FreeTurn-Windows")
	if t := strings.TrimSpace(g.Token()); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	if hasCache && entry.ETag != "" {
		req.Header.Set("If-None-Match", entry.ETag)
	}

	resp, err := g.HTTP.Do(req)
	if err != nil {
		// Без сети отдаём кэш, если он есть: пусть UI покажет прошлые данные.
		if hasCache {
			return entry.Release, nil
		}
		return Release{}, fmt.Errorf("%w: %w", ErrOffline, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotModified:
		entry.FetchedAt = time.Now()
		g.store(repo, entry)
		return entry.Release, nil

	case http.StatusOK:
		var rel Release
		if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
			return Release{}, fmt.Errorf("неожиданный ответ GitHub: %w", err)
		}
		g.store(repo, cacheEntry{ETag: resp.Header.Get("ETag"), FetchedAt: time.Now(), Release: rel})
		return rel, nil

	case http.StatusForbidden, http.StatusTooManyRequests:
		if remaining(resp) == 0 {
			if hasCache {
				return entry.Release, nil
			}
			return Release{}, ErrRateLimited
		}
		return Release{}, fmt.Errorf("GitHub отказал: %s", resp.Status)

	case http.StatusNotFound:
		// Закрытый репозиторий отвечает тем же 404, что и несуществующий:
		// без подсказки причина выглядит загадочно.
		return Release{}, fmt.Errorf("релизы репозитория %s не найдены: "+
			"его нет, в нём ещё нет релизов или он закрыт (тогда нужен токен GitHub в настройках)", repo)

	default:
		return Release{}, fmt.Errorf("GitHub ответил %s", resp.Status)
	}
}

// remaining читает остаток лимита из заголовка; -1 - заголовка нет.
func remaining(resp *http.Response) int {
	v := resp.Header.Get("X-RateLimit-Remaining")
	if v == "" {
		return -1
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return -1
	}
	return n
}

func (g *GitHub) store(repo string, e cacheEntry) {
	g.mu.Lock()
	g.cache[repo] = e
	data, err := json.MarshalIndent(g.cache, "", "  ")
	g.mu.Unlock()

	if err != nil || g.CachePath == "" {
		return
	}
	// Кэш - не критичные данные: ошибку записи молча игнорируем.
	_ = os.WriteFile(g.CachePath, data, 0o600)
}

func (g *GitHub) loadCache() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.cache) > 0 || g.CachePath == "" {
		return
	}
	data, err := os.ReadFile(g.CachePath)
	if err != nil {
		return
	}
	loaded := map[string]cacheEntry{}
	if json.Unmarshal(data, &loaded) == nil {
		g.cache = loaded
	}
}
