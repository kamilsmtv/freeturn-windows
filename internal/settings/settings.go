// Package settings хранит настройки приложения (не профили) в %APPDATA%\FreeTurn\settings.json.
package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/kamilsmtv/freeturn-windows/internal/paths"
)

// Режимы автообновления ядра.
const (
	UpdateAsk   = "ask"
	UpdateAuto  = "auto"
	UpdateNever = "never"
)

// Тема интерфейса.
const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"
)

// Settings - пользовательские настройки приложения.
type Settings struct {
	Theme           string   `json:"theme"`
	MinimizeToTray  bool     `json:"minimizeToTray"`
	StartMinimized  bool     `json:"startMinimized"`
	Autostart       bool     `json:"autostart"`
	AutoConnectLast bool     `json:"autoConnectLast"`
	CoreUpdateMode  string   `json:"coreUpdateMode"`
	CoreUpdateHours int      `json:"coreUpdateHours"`
	CheckGUIUpdates bool     `json:"checkGuiUpdates"`
	GitHubToken     string   `json:"githubToken"`
	SubRefreshHours int      `json:"subRefreshHours"`
	Subscriptions   []string `json:"subscriptions"`
	SubLastRefresh  string   `json:"subLastRefresh"`
	// OwnClientID - постоянная личность устройства, которую владелец сервера
	// добавляет в allowlist. Переносится бэкапом (см. internal/backup).
	OwnClientID        string `json:"ownClientId"`
	LastProfileID      string `json:"lastProfileId"`
	KeepLogLines       int    `json:"keepLogLines"`
	SuppressAdminWarn  bool   `json:"suppressAdminWarn"`
	CoreVersionChecked string `json:"coreVersionChecked"`
}

// Default возвращает настройки по умолчанию: интервал проверки ядра 6 часов,
// обновление с подтверждением (см. docs/plan.md).
func Default() Settings {
	return Settings{
		Theme:           ThemeSystem,
		MinimizeToTray:  true,
		CoreUpdateMode:  UpdateAsk,
		CoreUpdateHours: 6,
		CheckGUIUpdates: true,
		SubRefreshHours: 24,
		KeepLogLines:    5000,
	}
}

// Store - потокобезопасное хранилище настроек.
type Store struct {
	mu   sync.RWMutex
	cur  Settings
	path string
	// warning заполняется, когда файл пришлось откатить к значениям
	// по умолчанию.
	warning string
}

// Open читает настройки с диска; отсутствующий файл - не ошибка.
func Open() (*Store, error) {
	p, err := paths.SettingsFile()
	if err != nil {
		return nil, err
	}
	s := &Store{cur: Default(), path: p}

	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	s.cur, s.warning = parse(data, p)
	return s, nil
}

// parse разбирает файл настроек. Испорченный файл не повод не запускаться:
// откладываем его в сторону и продолжаем со значениями по умолчанию.
func parse(data []byte, path string) (Settings, string) {
	// Пустые поля остаются дефолтными: cur заранее заполнен Default().
	cur := Default()
	if err := json.Unmarshal(data, &cur); err != nil {
		return Default(), "файл настроек повреждён и заменён на значения по умолчанию; " +
			"прежний сохранён как " + keepBroken(path)
	}
	cur.normalize()
	return cur, ""
}

// keepBroken откладывает испорченный файл рядом и возвращает его имя.
func keepBroken(path string) string {
	broken := path + ".broken"
	if err := os.Rename(path, broken); err != nil {
		return "(сохранить не удалось: " + err.Error() + ")"
	}
	return broken
}

// Warning возвращает предупреждение, если при чтении что-то пошло не так.
func (s *Store) Warning() string { return s.warning }

// Get возвращает копию текущих настроек. Списки отдаются непустыми
// срезами: nil в JSON становится null, и фронтенд теряет методы массива.
func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cur := s.cur
	if cur.Subscriptions == nil {
		cur.Subscriptions = []string{}
	}
	return cur
}

// Save нормализует и атомарно записывает настройки.
func (s *Store) Save(v Settings) error {
	v.normalize()

	s.mu.Lock()
	s.cur = v
	s.mu.Unlock()

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Clean(s.path))
}

func (v *Settings) normalize() {
	switch v.Theme {
	case ThemeLight, ThemeDark, ThemeSystem:
	default:
		v.Theme = ThemeSystem
	}
	switch v.CoreUpdateMode {
	case UpdateAsk, UpdateAuto, UpdateNever:
	default:
		v.CoreUpdateMode = UpdateAsk
	}
	if v.CoreUpdateHours < 1 {
		v.CoreUpdateHours = 6
	}
	if v.SubRefreshHours < 1 {
		v.SubRefreshHours = 24
	}
	if v.KeepLogLines < 100 {
		v.KeepLogLines = 5000
	}
}
