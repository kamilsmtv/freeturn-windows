package profile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "profiles.json"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	return s
}

func TestStoreCRUD(t *testing.T) {
	s := newStore(t)

	p := New("RU-1")
	p.Client.ServerAddress = "1.2.3.4:56000"
	saved, err := s.Save(p)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if s.All().ActiveID != saved.ID {
		t.Error("первый профиль должен становиться активным")
	}

	saved.Name = "RU-1 (переименован)"
	if _, err := s.Save(saved); err != nil {
		t.Fatalf("повторный Save: %v", err)
	}
	if len(s.All().List) != 1 {
		t.Errorf("сохранение по существующему ID не должно плодить записи: %d", len(s.All().List))
	}

	got, err := s.Get(saved.ID)
	if err != nil || got.Name != "RU-1 (переименован)" {
		t.Errorf("Get = %+v, %v", got, err)
	}

	if err := s.Delete(saved.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(saved.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("после удаления ожидался ErrNotFound, получено %v", err)
	}
	if s.All().ActiveID != "" {
		t.Error("активный профиль должен сбрасываться, когда список опустел")
	}
}

func TestStoreDuplicate(t *testing.T) {
	s := newStore(t)
	p := New("RU-1")
	p.Client.ClientID = NewClientID()
	saved, _ := s.Save(p)

	copyOf, err := s.Duplicate(saved.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if copyOf.ID == saved.ID {
		t.Error("у копии должен быть свой ID")
	}
	if copyOf.Client.ClientID != "" {
		t.Error("client-id уникален для клиента и копироваться не должен")
	}
	if copyOf.Name != "RU-1 (копия)" {
		t.Errorf("имя копии = %q", copyOf.Name)
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	s, _ := OpenStore(path)

	p := New("RU-1")
	p.Opts.ObfKey = NewObfKey()
	p.SSH.Password = "секрет"
	saved, _ := s.Save(p)

	again, err := OpenStore(path)
	if err != nil {
		t.Fatalf("повторное открытие: %v", err)
	}
	got, err := again.Get(saved.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// На Windows секреты уезжают через DPAPI и возвращаются расшифрованными;
	// на других ОС - остаются открытыми. В обоих случаях значение то же.
	if got.Opts.ObfKey != saved.Opts.ObfKey || got.SSH.Password != "секрет" {
		t.Errorf("секреты не пережили перезапись: %+v", got.Opts)
	}
}

func TestReplaceBySubKeepsManualProfilesAndUserFields(t *testing.T) {
	s := newStore(t)

	manual := New("свой")
	manual.Client.ServerAddress = "9.9.9.9:56000"
	manualSaved, _ := s.Save(manual)

	fromSub := New("RU-1")
	fromSub.Client.ServerAddress = "1.2.3.4:56000"
	fromSub.Client.VKLink = "https://vk.ru/call/join/mine"
	fromSub.Client.ClientID = NewClientID()
	fromSub.SubURL = "https://example.invalid/sub.md"
	subSaved, _ := s.Save(fromSub)

	// Подписка обновилась: тот же адрес сервера, но без пользовательских полей.
	incoming := New("RU-1 обновлён")
	incoming.Client.ServerAddress = "1.2.3.4:56000"
	if err := s.ReplaceBySub("https://example.invalid/sub.md", []Profile{incoming}); err != nil {
		t.Fatalf("ReplaceBySub: %v", err)
	}

	list := s.All().List
	if len(list) != 2 {
		t.Fatalf("профилей = %d, want 2 (свой + из подписки)", len(list))
	}
	if _, err := s.Get(manualSaved.ID); err != nil {
		t.Error("профиль, заведённый руками, обновление подписки трогать не должно")
	}

	updated, err := s.Get(subSaved.ID)
	if err != nil {
		t.Fatalf("узел подписки должен сохранить свой ID: %v", err)
	}
	if updated.Name != "RU-1 обновлён" {
		t.Errorf("имя не обновилось: %q", updated.Name)
	}
	if updated.Client.VKLink != "https://vk.ru/call/join/mine" {
		t.Error("ссылка на звонок принадлежит пользователю и должна пережить обновление")
	}
	if updated.Client.ClientID != fromSub.Client.ClientID {
		t.Error("client-id должен пережить обновление подписки")
	}
}

// Пустой список обязан уезжать во фронтенд как [], а не null: на null
// у массива нет ни find, ни map, и интерфейс падает при первой же отрисовке.
func TestSnapshotMarshalsEmptyListAsArray(t *testing.T) {
	s := newStore(t)

	data, err := json.Marshal(s.All())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), `"list":null`) {
		t.Fatalf("пустой список сериализован как null: %s", data)
	}
	if !strings.Contains(string(data), `"list":[]`) {
		t.Fatalf("ожидался пустой массив, получено: %s", data)
	}
}

// Испорченный файл профилей не должен мешать запуску приложения.
func TestOpenStoreSurvivesBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte("[не json"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("испорченный файл не должен давать ошибку запуска: %v", err)
	}
	if len(s.All().List) != 0 {
		t.Error("список должен начинаться пустым")
	}
	if !strings.Contains(s.Warning(), "повреждён") {
		t.Errorf("пользователь должен узнать о проблеме: %q", s.Warning())
	}
	if _, err := os.Stat(path + ".broken"); err != nil {
		t.Error("прежний файл должен сохраняться: профили можно достать руками")
	}

	// Хранилище остаётся рабочим.
	if _, err := s.Save(New("после сбоя")); err != nil {
		t.Fatalf("Save: %v", err)
	}
}
