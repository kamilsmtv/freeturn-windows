package qr

import (
	"bytes"
	"errors"
	"image/png"
	"strings"
	"testing"
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
	if got := img.Bounds().Dx(); got != Size {
		t.Errorf("сторона картинки %d, ожидалась %d", got, Size)
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

func TestDataURI(t *testing.T) {
	uri, err := DataURI("freeturn://eyJ2IjoxfQ")
	if err != nil {
		t.Fatalf("не удалось собрать data URI: %v", err)
	}
	if !strings.HasPrefix(uri, "data:image/png;base64,") {
		t.Errorf("неожиданный префикс: %.40s", uri)
	}
}
