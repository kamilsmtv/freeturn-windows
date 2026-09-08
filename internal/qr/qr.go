// Package qr рисует QR-коды ссылок для передачи конфигурации с экрана.
package qr

import (
	"encoding/base64"
	"errors"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// MaxChars - предел длины строки, которую имеет смысл показывать кодом.
//
// Ограничение взято у Android-клиента: длинная ссылка укладывается в такой
// плотный код, что телефон его уже не читает, и вместо бесполезной картинки
// честнее показать, что кода не будет.
const MaxChars = 1200

// Size - сторона картинки в пикселях. Кратна типичному размеру матрицы,
// поэтому модули остаются ровными квадратами без размытия.
const Size = 512

// ErrTooLong - строка не помещается в читаемый код.
var ErrTooLong = errors.New("ссылка слишком длинная для QR-кода")

// PNG рисует код и возвращает PNG.
//
// Уровень коррекции - низкий, как в Android-клиенте: избыточность крадёт
// ёмкость, а ссылка и так у предела; экран не мнётся и не пачкается.
func PNG(text string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("нечего кодировать")
	}
	if len([]rune(text)) > MaxChars {
		return nil, ErrTooLong
	}
	return qrcode.Encode(text, qrcode.Low, Size)
}

// DataURI отдаёт код в виде data:image/png;base64 - так его показывает
// интерфейс, не сохраняя ничего на диск.
func DataURI(text string) (string, error) {
	png, err := PNG(text)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}
