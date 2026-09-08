package main

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/backup"
	"github.com/kamilsmtv/freeturn-windows/internal/link"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/sub"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// EventProfiles - список профилей изменился.
const EventProfiles = "profiles:changed"

// Profiles отдаёт список профилей и активный.
func (a *App) Profiles() profile.Snapshot { return a.profiles.All() }

// SaveProfile создаёт или обновляет профиль.
func (a *App) SaveProfile(p profile.Profile) (profile.Profile, error) {
	saved, err := a.profiles.Save(p)
	a.emitProfiles()
	return saved, err
}

// NewProfile возвращает заготовку профиля с дефолтами (не сохраняя её).
func (a *App) NewProfile(name string) profile.Profile {
	p := profile.New(name)
	// Своя личность у каждого профиля: сервер различает клиентов по client-id.
	p.Client.ClientID = profile.NewClientID()
	return p
}

// DeleteProfile удаляет профиль.
func (a *App) DeleteProfile(id string) error {
	if st := a.core.Status(); st.ProfileID == id && a.core.Running() {
		return errors.New("нельзя удалить профиль, пока он подключён")
	}
	err := a.profiles.Delete(id)
	a.emitProfiles()
	return err
}

// DuplicateProfile клонирует профиль.
func (a *App) DuplicateProfile(id string) (profile.Profile, error) {
	p, err := a.profiles.Duplicate(id)
	a.emitProfiles()
	return p, err
}

// SetActiveProfile помечает профиль активным.
func (a *App) SetActiveProfile(id string) error {
	err := a.profiles.SetActive(id)
	a.emitProfiles()
	return err
}

// GenerateObfKey выдаёт новый ключ обфускации.
func (a *App) GenerateObfKey() string { return profile.NewObfKey() }

// GenerateClientID выдаёт новый client-id.
func (a *App) GenerateClientID() string { return profile.NewClientID() }

// ImportLink разбирает freeturn://-ссылку и сохраняет её как новый профиль.
func (a *App) ImportLink(raw string) (profile.Profile, error) {
	l, err := link.Parse(raw)
	if err != nil {
		return profile.Profile{}, err
	}
	p := l.ToProfile()
	if p.Client.ClientID == "" {
		p.Client.ClientID = profile.NewClientID()
	}
	saved, err := a.profiles.Save(p)
	a.emitProfiles()
	return saved, err
}

// ExportLink собирает share-ссылку профиля.
//
// Ссылка на звонок VK и client-id уникальны для получателя, поэтому
// вкладываются только по явному выбору пользователя.
func (a *App) ExportLink(id string, includeVKLink bool, clientID string) (string, error) {
	p, err := a.profiles.Get(id)
	if err != nil {
		return "", err
	}
	return link.FromProfile(p, includeVKLink, clientID).Encode(), nil
}

// ExportLinkToFile сохраняет ссылку в текстовый файл.
func (a *App) ExportLinkToFile(id string, includeVKLink bool, clientID string) (string, error) {
	url, err := a.ExportLink(id, includeVKLink, clientID)
	if err != nil {
		return "", err
	}
	p, _ := a.profiles.Get(id)
	path, err := a.saveFileDialog("Сохранить ссылку", safeFileName(p.Name)+".txt",
		[]wailsruntime.FileFilter{{DisplayName: "Текстовый файл (*.txt)", Pattern: "*.txt"}})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, []byte(url+"\n"), 0o600)
}

// SubPreview - результат разбора подписки до применения.
type SubPreview struct {
	Name     string            `json:"name"`
	Refresh  string            `json:"refresh"`
	Skipped  int               `json:"skipped"`
	Profiles []profile.Profile `json:"profiles"`
}

// FetchSubscription скачивает подписку и показывает, что в ней есть.
func (a *App) FetchSubscription(url string) (SubPreview, error) {
	s, err := sub.Fetch(a.bg(), strings.TrimSpace(url))
	if err != nil {
		return SubPreview{}, err
	}
	return SubPreview{Name: s.Name, Refresh: s.Refresh, Skipped: s.Skipped, Profiles: s.Profiles()}, nil
}

// ApplySubscription обновляет профили, пришедшие из подписки.
func (a *App) ApplySubscription(url string) (SubPreview, error) {
	url = strings.TrimSpace(url)
	preview, err := a.FetchSubscription(url)
	if err != nil {
		return SubPreview{}, err
	}
	if len(preview.Profiles) == 0 {
		return preview, errors.New("в подписке нет ни одного пригодного сервера")
	}
	if err := a.profiles.ReplaceBySub(url, preview.Profiles); err != nil {
		return preview, err
	}

	s := a.settings.Get()
	if !containsString(s.Subscriptions, url) {
		s.Subscriptions = append(s.Subscriptions, url)
	}
	s.SubLastRefresh = time.Now().Format(time.RFC3339)
	_ = a.settings.Save(s)

	a.emitProfiles()
	return preview, nil
}

// RemoveSubscription забывает подписку; её профили остаются на месте.
func (a *App) RemoveSubscription(url string) error {
	s := a.settings.Get()
	out := s.Subscriptions[:0]
	for _, u := range s.Subscriptions {
		if u != url {
			out = append(out, u)
		}
	}
	s.Subscriptions = out
	return a.settings.Save(s)
}

// RefreshSubscriptions обновляет все сохранённые подписки и возвращает
// список проблем (пустой срез, а не nil: nil уезжает во фронтенд как null).
func (a *App) RefreshSubscriptions() []string {
	problems := []string{}
	for _, url := range a.settings.Get().Subscriptions {
		if _, err := a.ApplySubscription(url); err != nil {
			problems = append(problems, url+": "+err.Error())
		}
	}
	return problems
}

// watchSubscriptions обновляет подписки по расписанию из настроек.
func (a *App) watchSubscriptions() {
	for {
		hours := a.settings.Get().SubRefreshHours
		timer := time.NewTimer(time.Duration(hours) * time.Hour)
		select {
		case <-a.bg().Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		a.RefreshSubscriptions()
	}
}

// ExportBackup сохраняет зашифрованный бэкап всех настроек.
func (a *App) ExportBackup(password string) (string, error) {
	if password == "" {
		return "", errors.New("задайте пароль: бэкап хранит ключи обфускации и пароли SSH")
	}
	snap := a.profiles.All()
	s := a.settings.Get()

	blob, err := backup.Export(backup.Data{
		Profiles:    snap.List,
		ActiveID:    snap.ActiveID,
		OwnClientID: a.ownClientID(),
		Toggles:     map[string]bool{"privacyMode": s.Theme == "dark"},
	}, password)
	if err != nil {
		return "", err
	}

	path, err := a.saveFileDialog("Сохранить бэкап",
		"freeturn-backup-"+time.Now().Format("2006-01-02")+".json",
		[]wailsruntime.FileFilter{{DisplayName: "Бэкап FreeTurn (*.json)", Pattern: "*.json"}})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, blob, 0o600)
}

// ImportBackup восстанавливает настройки из бэкапа.
func (a *App) ImportBackup(password string) (int, error) {
	path, err := a.openFileDialog("Выберите бэкап",
		[]wailsruntime.FileFilter{{DisplayName: "Бэкап FreeTurn (*.json)", Pattern: "*.json"}})
	if err != nil || path == "" {
		return 0, err
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	d, err := backup.Import(blob, password)
	if err != nil {
		return 0, err
	}
	if err := a.profiles.Replace(profile.Snapshot{List: d.Profiles, ActiveID: d.ActiveID}); err != nil {
		return 0, err
	}

	s := a.settings.Get()
	s.OwnClientID = d.OwnClientID
	_ = a.settings.Save(s)

	a.emitProfiles()
	return len(d.Profiles), nil
}

// ownClientID возвращает постоянную личность устройства, заводя её при первом обращении.
func (a *App) ownClientID() string {
	s := a.settings.Get()
	if profile.ValidClientID(s.OwnClientID) {
		return s.OwnClientID
	}
	s.OwnClientID = profile.NewClientID()
	_ = a.settings.Save(s)
	return s.OwnClientID
}

func (a *App) emitProfiles() {
	a.emit(EventProfiles, a.profiles.All())
}

func (a *App) saveFileDialog(title, name string, filters []wailsruntime.FileFilter) (string, error) {
	if a.ctx == nil {
		return "", errors.New("окно ещё не готово")
	}
	return wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: name,
		Filters:         filters,
	})
}

func (a *App) openFileDialog(title string, filters []wailsruntime.FileFilter) (string, error) {
	if a.ctx == nil {
		return "", errors.New("окно ещё не готово")
	}
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: title, Filters: filters})
}

// safeFileName убирает символы, недопустимые в именах файлов Windows.
func safeFileName(name string) string {
	replacer := strings.NewReplacer(`\`, "-", "/", "-", ":", "-", "*", "-", "?", "-",
		`"`, "-", "<", "-", ">", "-", "|", "-")
	out := strings.TrimSpace(replacer.Replace(name))
	if out == "" {
		return "freeturn"
	}
	return out
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
