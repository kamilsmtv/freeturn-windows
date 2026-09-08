package qr

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"strconv"
	"strings"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
)

func TestPNG(t *testing.T) {
	data, err := PNG("freeturn://eyJ2IjoxfQ")
	if err != nil {
		t.Fatalf("не удалось нарисовать код: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("получен не PNG: %v", err)
	}
	// Сторона обязана быть кратна модулю: иначе при сохранении границы
	// модулей размывались бы и код терял читаемость.
	side := img.Bounds().Dx()
	if side%PixelsPerModule != 0 {
		t.Errorf("сторона %d не кратна %d пикселям модуля", side, PixelsPerModule)
	}
	uri, err := DataURI("freeturn://eyJ2IjoxfQ")
	if err != nil {
		t.Fatalf("не удалось собрать data URI: %v", err)
	}
	if want := uri.Modules * PixelsPerModule; side != want {
		t.Errorf("сторона картинки %d, ожидалась %d", side, want)
	}
}

// Длинную ссылку телефон уже не прочитает: лучше отказ, чем картинка,
// которую невозможно отсканировать.
func TestPNGTooLong(t *testing.T) {
	if _, err := PNG(strings.Repeat("a", MaxChars+1)); !errors.Is(err, ErrTooLong) {
		t.Errorf("ожидалась ErrTooLong, получено %v", err)
	}
	if _, err := PNG(strings.Repeat("a", MaxChars)); err != nil {
		t.Errorf("ссылка предельной длины должна кодироваться: %v", err)
	}
}

func TestPNGEmpty(t *testing.T) {
	if _, err := PNG("   "); err == nil {
		t.Error("пустая строка не должна кодироваться")
	}
}

func TestSVG(t *testing.T) {
	svg, err := SVG("freeturn://eyJ2IjoxfQ")
	if err != nil {
		t.Fatalf("не удалось нарисовать вектор: %v", err)
	}
	for _, want := range []string{"<svg ", "viewBox=\"0 0 ", "fill=\"#fff\"", "fill=\"#000\"", "</svg>"} {
		if !strings.Contains(svg, want) {
			t.Errorf("в разметке нет %q", want)
		}
	}
	// Матрица версии 1 с полем тишины - 21+8 модулей; больше кода - больше сторона.
	long, err := SVG(strings.Repeat("a", 800))
	if err != nil {
		t.Fatalf("длинная строка должна кодироваться: %v", err)
	}
	if side(t, long) <= side(t, svg) {
		t.Error("матрица длинной ссылки должна быть крупнее")
	}
}

// side достаёт сторону матрицы из viewBox.
func side(t *testing.T, svg string) int {
	t.Helper()
	const marker = `viewBox="0 0 `
	i := strings.Index(svg, marker)
	if i < 0 {
		t.Fatal("нет viewBox")
	}
	rest := svg[i+len(marker):]
	n, _, ok := strings.Cut(rest, " ")
	if !ok {
		t.Fatal("не разобран viewBox")
	}
	v, err := strconv.Atoi(n)
	if err != nil {
		t.Fatalf("сторона не число: %v", err)
	}
	return v
}

func TestSVGTooLong(t *testing.T) {
	if _, err := SVG(strings.Repeat("a", MaxChars+1)); !errors.Is(err, ErrTooLong) {
		t.Errorf("ожидалась ErrTooLong, получено %v", err)
	}
}

func TestDataURIIsSVG(t *testing.T) {
	img, err := DataURI("freeturn://eyJ2IjoxfQ")
	if err != nil {
		t.Fatalf("не удалось собрать data URI: %v", err)
	}
	uri := img.URI
	if !strings.HasPrefix(uri, "data:image/svg+xml;base64,") {
		t.Errorf("неожиданный префикс: %.40s", uri)
	}
	// Сторона матрицы нужна интерфейсу для выбора размера картинки.
	if img.Modules < 21 {
		t.Errorf("сторона матрицы %d - меньше минимальной версии кода", img.Modules)
	}
	long, err := DataURI(strings.Repeat("a", 800))
	if err != nil {
		t.Fatalf("длинная строка должна кодироваться: %v", err)
	}
	if long.Modules <= img.Modules {
		t.Errorf("матрица длинной ссылки (%d) должна быть крупнее короткой (%d)", long.Modules, img.Modules)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "data:image/svg+xml;base64,"))
	if err != nil {
		t.Fatalf("тело не base64: %v", err)
	}
	if !strings.HasPrefix(string(raw), "<svg ") {
		t.Error("в data URI не SVG")
	}
}

// Вектор и растр обязаны совпадать модуль в модуль: перепутанные оси дают
// зеркальный код, который на глаз почти не отличить, а камера не читает.
func TestSVGMatchesPNG(t *testing.T) {
	const text = "freeturn://eyJ2IjoxLCJuYW1lIjoi0YHQtdGA0LLQtdGAIn0"

	svg, side, err := build(text)
	if err != nil {
		t.Fatalf("не удалось нарисовать код: %v", err)
	}

	// Растр в один пиксель на модуль - прямой слепок матрицы.
	raw, err := qrcodeEncodeOnePixel(text)
	if err != nil {
		t.Fatalf("не удалось нарисовать растр: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("получен не PNG: %v", err)
	}
	if got := img.Bounds().Dx(); got != side {
		t.Fatalf("сторона растра %d, вектора %d", got, side)
	}

	dark := parsePath(t, svg, side)
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			r, g, b, _ := img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y).RGBA()
			black := r == 0 && g == 0 && b == 0
			if black != dark[y][x] {
				t.Fatalf("модуль (%d,%d): растр %v, вектор %v", x, y, black, dark[y][x])
			}
		}
	}
}

// parsePath собирает матрицу обратно из пути SVG: M<x> <y>h<run>v1h-<run>z.
func parsePath(t *testing.T, svg string, side int) [][]bool {
	t.Helper()

	out := make([][]bool, side)
	for i := range out {
		out[i] = make([]bool, side)
	}

	_, path, ok := strings.Cut(svg, `<path fill="#000" d="`)
	if !ok {
		t.Fatal("в разметке нет пути")
	}
	path, _, ok = strings.Cut(path, `"`)
	if !ok {
		t.Fatal("путь не закрыт")
	}

	for _, cmd := range strings.Split(path, "z") {
		if cmd == "" {
			continue
		}
		var x, y, run int
		if _, err := fmt.Sscanf(cmd, "M%d %dh%dv1h-%d", &x, &y, &run, new(int)); err != nil {
			t.Fatalf("не разобран участок %q: %v", cmd, err)
		}
		for i := 0; i < run; i++ {
			out[y][x+i] = true
		}
	}
	return out
}

// qrcodeEncodeOnePixel рисует растр ровно по пикселю на модуль.
func qrcodeEncodeOnePixel(text string) ([]byte, error) {
	return qrcode.Encode(text, qrcode.Low, -1)
}
