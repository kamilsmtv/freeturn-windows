package profile

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sync"

	"github.com/kamilsmtv/freeturn-windows/internal/secrets"
)

// Snapshot - список профилей и выбранный активный.
type Snapshot struct {
	List     []Profile `json:"list"`
	ActiveID string    `json:"activeId"`
}

// Active возвращает активный профиль.
func (s Snapshot) Active() (Profile, bool) {
	for _, p := range s.List {
		if p.ID == s.ActiveID {
			return p, true
		}
	}
	return Profile{}, false
}

// Store хранит профили в JSON-файле. Секреты на диске зашифрованы DPAPI.
type Store struct {
	mu   sync.RWMutex
	snap Snapshot
	path string
	// warning заполняется, когда файл профилей пришлось отложить.
	warning string
}

// ErrNotFound - профиля с таким ID нет.
var ErrNotFound = errors.New("профиль не найден")

// OpenStore читает профили из файла; отсутствующий файл - пустой список.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.snap); err != nil {
		// Потерять доступ к приложению из-за одного испорченного файла
		// хуже, чем начать с пустого списка: файл откладываем в сторону,
		// чтобы профили можно было достать руками.
		s.snap = Snapshot{}
		broken := path + ".broken"
		if rerr := os.Rename(path, broken); rerr != nil {
			broken = "(сохранить не удалось: " + rerr.Error() + ")"
		}
		s.warning = "файл профилей повреждён, список начат заново; прежний сохранён как " + broken
		return s, nil
	}
	for i := range s.snap.List {
		decryptSecrets(&s.snap.List[i])
	}
	return s, nil
}

// Warning возвращает предупреждение о проблемах при чтении файла.
func (s *Store) Warning() string { return s.warning }

// All возвращает копию текущего снимка.
//
// Список всегда непустой срез, а не nil: encoding/json превращает nil-срез
// в null, и фронтенд получает вместо массива значение, у которого нет ни
// find, ни map.
func (s *Store) All() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{List: s.copyList(), ActiveID: s.snap.ActiveID}
}

// copyList копирует список под уже взятой блокировкой.
func (s *Store) copyList() []Profile {
	out := make([]Profile, len(s.snap.List))
	copy(out, s.snap.List)
	return out
}

// Get возвращает профиль по ID.
func (s *Store) Get(id string) (Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.snap.List {
		if p.ID == id {
			return p, nil
		}
	}
	return Profile{}, ErrNotFound
}

// Save добавляет профиль или обновляет существующий по ID.
func (s *Store) Save(p Profile) (Profile, error) {
	if p.ID == "" {
		p.ID = NewID()
	}
	if p.Name == "" {
		p.Name = FallbackName
	}

	s.mu.Lock()
	replaced := false
	for i := range s.snap.List {
		if s.snap.List[i].ID == p.ID {
			s.snap.List[i], replaced = p, true
			break
		}
	}
	if !replaced {
		s.snap.List = append(s.snap.List, p)
	}
	// Первый добавленный профиль сразу становится активным.
	if s.snap.ActiveID == "" {
		s.snap.ActiveID = p.ID
	}
	s.mu.Unlock()

	return p, s.persist()
}

// Delete удаляет профиль.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	out := s.snap.List[:0]
	found := false
	for _, p := range s.snap.List {
		if p.ID == id {
			found = true
			continue
		}
		out = append(out, p)
	}
	s.snap.List = out
	if s.snap.ActiveID == id {
		s.snap.ActiveID = ""
		if len(s.snap.List) > 0 {
			s.snap.ActiveID = s.snap.List[0].ID
		}
	}
	s.mu.Unlock()

	if !found {
		return ErrNotFound
	}
	return s.persist()
}

// Duplicate клонирует профиль под новым именем.
func (s *Store) Duplicate(id string) (Profile, error) {
	src, err := s.Get(id)
	if err != nil {
		return Profile{}, err
	}
	// Client ID и ключи WireGuard уникальны для клиента: у копии их быть
	// не должно. Иначе сервер увидит два подключения с одной личностью, а
	// два пира с одинаковым публичным ключом он попросту не различит.
	c := src.Clone(src.Name + " (копия)")
	c.Client.ClientID = ""
	c.Client.WireGuardConfig = ""
	return s.Save(c)
}

// SetActive помечает профиль активным.
func (s *Store) SetActive(id string) error {
	if _, err := s.Get(id); err != nil {
		return err
	}
	s.mu.Lock()
	s.snap.ActiveID = id
	s.mu.Unlock()
	return s.persist()
}

// Replace целиком заменяет список (восстановление из бэкапа).
func (s *Store) Replace(snap Snapshot) error {
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
	return s.persist()
}

// ReplaceBySub заменяет профили, пришедшие из подписки subURL, на новые.
// Профили, заведённые руками, не трогаются.
func (s *Store) ReplaceBySub(subURL string, incoming []Profile) error {
	s.mu.Lock()
	kept := make([]Profile, 0, len(s.snap.List))
	// ID существующих узлов переиспользуем по адресу сервера: так у
	// пользователя не разъезжается активный профиль после обновления подписки.
	byPeer := map[string]Profile{}
	for _, p := range s.snap.List {
		if p.SubURL == subURL {
			byPeer[p.Client.ServerAddress] = p
			continue
		}
		kept = append(kept, p)
	}
	for _, p := range incoming {
		p.SubURL = subURL
		if old, ok := byPeer[p.Client.ServerAddress]; ok {
			p.ID = old.ID
			// Ссылка на звонок и client-id принадлежат пользователю,
			// а не подписке: сохраняем их между обновлениями.
			if p.Client.VKLink == "" {
				p.Client.VKLink = old.Client.VKLink
			}
			if p.Client.ClientID == "" {
				p.Client.ClientID = old.Client.ClientID
			}
		}
		kept = append(kept, p)
	}
	s.snap.List = kept
	if _, ok := hasID(kept, s.snap.ActiveID); !ok {
		s.snap.ActiveID = ""
		if len(kept) > 0 {
			s.snap.ActiveID = kept[0].ID
		}
	}
	s.mu.Unlock()
	return s.persist()
}

func hasID(list []Profile, id string) (Profile, bool) {
	for _, p := range list {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

// persist атомарно записывает файл, зашифровав секреты.
func (s *Store) persist() error {
	s.mu.RLock()
	snap := Snapshot{List: s.copyList(), ActiveID: s.snap.ActiveID}
	s.mu.RUnlock()

	for i := range snap.List {
		encryptSecrets(&snap.List[i])
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// encryptSecrets прячет ключи и пароли перед записью на диск.
func encryptSecrets(p *Profile) {
	p.Opts.ObfKey = secrets.Protect(p.Opts.ObfKey)
	p.SSH.Password = secrets.Protect(p.SSH.Password)
	p.SSH.SSHKey = secrets.Protect(p.SSH.SSHKey)
	p.SSH.SudoPassword = secrets.Protect(p.SSH.SudoPassword)
}

// decryptSecrets возвращает секреты в открытый вид после чтения с диска.
func decryptSecrets(p *Profile) {
	p.Opts.ObfKey = secrets.Unprotect(p.Opts.ObfKey)
	p.SSH.Password = secrets.Unprotect(p.SSH.Password)
	p.SSH.SSHKey = secrets.Unprotect(p.SSH.SSHKey)
	p.SSH.SudoPassword = secrets.Unprotect(p.SSH.SudoPassword)
}
