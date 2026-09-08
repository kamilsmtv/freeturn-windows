// Package secrets шифрует чувствительные поля профилей (ключи обфускации,
// пароли SSH) через DPAPI, чтобы они не лежали в JSON открытым текстом.
//
// Формат хранения: строка "dpapi:<base64(blob)>". Значение без этого
// префикса считается открытым - так читаются файлы, созданные до включения
// шифрования, и бэкапы, пришедшие с другой машины.
package secrets

import (
	"encoding/base64"
	"strings"
)

const prefix = "dpapi:"

// Protect шифрует значение для хранения на диске. Пустая строка остаётся пустой.
// Если DPAPI недоступен, значение сохраняется как есть: потерять профиль
// хуже, чем сохранить его без дополнительной защиты.
func Protect(value string) string {
	if value == "" || strings.HasPrefix(value, prefix) {
		return value
	}
	blob, err := protect([]byte(value))
	if err != nil {
		return value
	}
	return prefix + base64.StdEncoding.EncodeToString(blob)
}

// Unprotect расшифровывает значение, сохранённое Protect.
func Unprotect(value string) string {
	if !strings.HasPrefix(value, prefix) {
		return value
	}
	blob, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return ""
	}
	plain, err := unprotect(blob)
	if err != nil {
		// Блоб с чужой машины или из чужого профиля Windows расшифровать
		// нельзя - возвращаем пустую строку, чтобы UI попросил ввести заново.
		return ""
	}
	return string(plain)
}

// IsProtected сообщает, зашифровано ли значение.
func IsProtected(value string) bool { return strings.HasPrefix(value, prefix) }
