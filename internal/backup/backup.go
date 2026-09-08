// Package backup читает и пишет бэкапы настроек в формате Android-клиента.
//
// Совместимость: конверт повторяет data/backup/BackupCrypto.kt (PBKDF2-HMAC-SHA256,
// 210 000 итераций, AES-256-GCM), содержимое - SettingsBackup версии 4 с
// профилями в раскладке ServerJson.kt. Расхождения описаны в README.
package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"crypto/sha256"
	"golang.org/x/crypto/pbkdf2"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// Параметры конверта - контракт с Android-клиентом, менять нельзя.
const (
	magic      = "freeturn-backup"
	envVersion = 1
	iterations = 210_000
	keyBytes   = 32
	saltLen    = 16
	ivLen      = 12

	// FormatVersion - версия полезной нагрузки (SettingsBackup.FORMAT_VERSION).
	FormatVersion = 4
)

// Ошибки восстановления.
var (
	ErrBadPassword = errors.New("неверный пароль от бэкапа")
	ErrFormat      = errors.New("это не бэкап FreeTurn или файл повреждён")
	ErrClientID    = errors.New("в бэкапе испорчен client-id устройства")
)

// Data - содержимое бэкапа.
type Data struct {
	Profiles []profile.Profile
	ActiveID string
	// OwnClientID - постоянная личность устройства. Android требует её
	// обязательно: без неё восстановление на новом устройстве даёт свежий
	// cid, и сервер отклоняет подключение по allowlist.
	OwnClientID string
	// Toggles - настройки интерфейса из бэкапа. Часть из них относится
	// только к Android; мы их сохраняем ради round-trip, но не применяем.
	Toggles map[string]bool
}

type envelope struct {
	Magic string `json:"magic"`
	V     int    `json:"v"`
	KDF   string `json:"kdf"`
	Iter  int    `json:"iter"`
	Salt  string `json:"salt"`
	IV    string `json:"iv"`
	CT    string `json:"ct"`
}

// Encrypt шифрует полезную нагрузку паролем и возвращает конверт-JSON.
func Encrypt(plaintext []byte, password string) ([]byte, error) {
	salt := make([]byte, saltLen)
	iv := make([]byte, ivLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	gcm, err := newGCM(password, salt, iterations)
	if err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, iv, plaintext, nil)

	return json.Marshal(envelope{
		Magic: magic,
		V:     envVersion,
		KDF:   "pbkdf2-sha256",
		Iter:  iterations,
		Salt:  base64.StdEncoding.EncodeToString(salt),
		IV:    base64.StdEncoding.EncodeToString(iv),
		CT:    base64.StdEncoding.EncodeToString(ct),
	})
}

// Decrypt разбирает конверт и расшифровывает полезную нагрузку.
func Decrypt(data []byte, password string) ([]byte, error) {
	var e envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, ErrFormat
	}
	if e.Magic != magic {
		return nil, ErrFormat
	}
	iter := e.Iter
	if iter <= 0 {
		iter = iterations
	}

	salt, err1 := base64.StdEncoding.DecodeString(e.Salt)
	iv, err2 := base64.StdEncoding.DecodeString(e.IV)
	ct, err3 := base64.StdEncoding.DecodeString(e.CT)
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, ErrFormat
	}

	gcm, err := newGCM(password, salt, iter)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, iv, ct, nil)
	if err != nil {
		// GCM-тег не сошёлся: пароль неверный либо файл побит.
		return nil, ErrBadPassword
	}
	return plain, nil
}

func newGCM(password string, salt []byte, iter int) (cipher.AEAD, error) {
	key := pbkdf2.Key([]byte(password), salt, iter, keyBytes, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Export собирает зашифрованный бэкап.
func Export(d Data, password string) ([]byte, error) {
	if !profile.ValidClientID(d.OwnClientID) {
		return nil, ErrClientID
	}
	payload, err := encodePayload(d)
	if err != nil {
		return nil, err
	}
	return Encrypt(payload, password)
}

// Import расшифровывает и разбирает бэкап.
func Import(data []byte, password string) (Data, error) {
	payload, err := Decrypt(data, password)
	if err != nil {
		return Data{}, err
	}
	return decodePayload(payload)
}

// Toggles, которые Android пишет в бэкап. Наши значения по умолчанию
// совпадают с его дефолтами, чтобы round-trip не менял файл.
var toggleDefaults = map[string]bool{
	"dynamicTheme":          true,
	"nerdMode":              true,
	"privacyMode":           false,
	"seasonalDecor":         true,
	"restartServerOnSwitch": false,
	"hotspotProxy":          false,
	"suppressUpdatePrompt":  false,
	"suppressTgPrompt":      false,
}

func encodePayload(d Data) ([]byte, error) {
	out := map[string]any{
		"v":           FormatVersion,
		"servers":     encodeServers(d.Profiles),
		"ownClientId": d.OwnClientID,
	}
	if d.ActiveID != "" {
		out["activeId"] = d.ActiveID
	}
	for key, def := range toggleDefaults {
		if v, ok := d.Toggles[key]; ok {
			out[key] = v
			continue
		}
		out[key] = def
	}
	return json.Marshal(out)
}

func decodePayload(payload []byte) (Data, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return Data{}, ErrFormat
	}

	d := Data{Toggles: map[string]bool{}}
	_ = json.Unmarshal(raw["ownClientId"], &d.OwnClientID)
	// Android валидирует cid до перезаписи профилей: применить бэкап с
	// чужой личностью - значит получить обрыв на allowlist сервера.
	if !profile.ValidClientID(d.OwnClientID) {
		return Data{}, ErrClientID
	}
	_ = json.Unmarshal(raw["activeId"], &d.ActiveID)

	for key := range toggleDefaults {
		var v bool
		if err := json.Unmarshal(raw[key], &v); err == nil {
			d.Toggles[key] = v
		}
	}

	var servers []json.RawMessage
	if err := json.Unmarshal(raw["servers"], &servers); err != nil {
		return Data{}, fmt.Errorf("%w: список серверов не читается", ErrFormat)
	}
	d.Profiles = decodeServers(servers)
	return d, nil
}
