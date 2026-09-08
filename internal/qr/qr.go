// Package qr рисует QR-коды ссылок для передачи конфигурации с экрана.
package qr

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// MaxChars - предел длины строки, которую имеет смысл показывать кодом.
//
// Ограничение взято у Android-клиента: длинная ссылка укладывается в такой
// плотный код, что телефон его уже не читает, и вместо бесполезной картинки
// честнее показать, что кода не будет.
const MaxChars = 1200

// PixelsPerModule - размер модуля в сохраняемой картинке.
//
// Библиотека понимает отрицательный размер как «столько пикселей на модуль»:
// так сторона всегда кратна матрице и модули остаются ровными квадратами.
// Восьми пикселей хватает и для печати, и для пересылки картинкой.
const PixelsPerModule = 8

// ErrTooLong - строка не помещается в читаемый код.
var ErrTooLong = errors.New("ссылка слишком длинная для QR-кода")

// PNG рисует код и возвращает PNG.
//
// Уровень коррекции - низкий, как в Android-клиенте: избыточность крадёт
// ёмкость, а ссылка и так у предела; экран не мнётся и не пачкается.
func PNG(text string) ([]byte, error) {
	if err := check(text); err != nil {
		return nil, err
	}
	return qrcode.Encode(text, qrcode.Low, -PixelsPerModule)
}

func check(text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("нечего кодировать")
	}
	if len([]rune(text)) > MaxChars {
		return ErrTooLong
	}
	return nil
}

// SVG рисует тот же код вектором.
//
// Для экрана он лучше растра: гостевая ссылка с конфигурацией WireGuard даёт
// матрицу за сотню модулей, и уменьшённый PNG теряет их границы - код
// перестаёт читаться камерой. Вектор остаётся резким при любом размере.
func SVG(text string) (string, error) {
	svg, _, err := build(text)
	return svg, err
}

// build рисует код и заодно сообщает сторону матрицы: интерфейсу она нужна
// для выбора размера, а второй раз считать её разбором разметки - глупо.
func build(text string) (string, int, error) {
	if err := check(text); err != nil {
		return "", 0, err
	}
	code, err := qrcode.New(text, qrcode.Low)
	if err != nil {
		return "", 0, err
	}

	// Bitmap уже включает поле тишины вокруг матрицы.
	bitmap := code.Bitmap()
	n := len(bitmap)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, n, n)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="`, n, n)
	for y, row := range bitmap {
		// Соседние модули склеиваем в один прямоугольник: путь короче в разы.
		for x := 0; x < len(row); x++ {
			if !row[x] {
				continue
			}
			run := 1
			for x+run < len(row) && row[x+run] {
				run++
			}
			fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", x, y, run, run)
			x += run - 1
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String(), n, nil
}

// Image - готовая к показу картинка кода вместе со стороной матрицы.
type Image struct {
	// URI - data:image/svg+xml с самим кодом.
	URI string `json:"uri"`
	// Modules - сторона матрицы в модулях, вместе с полем тишины. По ней
	// интерфейс подбирает размер: модуль должен занимать целое число
	// пикселей, иначе границы плывут и камера код не разбирает.
	Modules int `json:"modules"`
}

// DataURI отдаёт код как data:image/svg+xml - так его показывает интерфейс,
// не сохраняя ничего на диск.
func DataURI(text string) (Image, error) {
	svg, n, err := build(text)
	if err != nil {
		return Image{}, err
	}
	return Image{
		URI:     "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg)),
		Modules: n,
	}, nil
}
