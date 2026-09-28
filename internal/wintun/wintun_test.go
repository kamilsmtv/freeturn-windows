package wintun

import (
	"path/filepath"
	"strings"
	"testing"

	"archive/zip"
	"bytes"
	"os"
)

// Чужой файл не должен становиться wintun.dll: сумма зашита в коде.
func TestInstallFromFileRejectsForeignFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "wintun.dll")
	if err := os.WriteFile(src, []byte("не библиотека"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := InstallFromFile(t.TempDir(), src)
	if err == nil {
		t.Fatal("подложный файл принят")
	}
	if !strings.Contains(err.Error(), "wintun") {
		t.Fatalf("непонятная ошибка: %v", err)
	}
}

// Архив без нужной библиотеки внутри - тоже отказ.
func TestInstallFromFileRejectsArchiveWithoutDLL(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("wintun/readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "wintun.zip")
	if err := os.WriteFile(src, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := InstallFromFile(t.TempDir(), src); err == nil {
		t.Fatal("архив без библиотеки принят")
	}
}

func TestInstallFromFileMissingFile(t *testing.T) {
	if err := InstallFromFile(t.TempDir(), filepath.Join(t.TempDir(), "нет.zip")); err == nil {
		t.Fatal("отсутствующий файл принят")
	}
}

// Выбран каталог вместо файла - понятный отказ, а не попытка чтения.
func TestInstallFromFileRejectsDirectory(t *testing.T) {
	err := InstallFromFile(t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("каталог принят")
	}
	if !strings.Contains(err.Error(), "каталог") {
		t.Fatalf("непонятная ошибка: %v", err)
	}
}

// После отказа в каталоге не должно оставаться обрывков.
func TestInstallFromFileLeavesNoLeftovers(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "wintun.zip")
	if err := os.WriteFile(src, []byte("мусор"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InstallFromFile(dir, src); err == nil {
		t.Fatal("мусор принят")
	}

	left, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("в каталоге что-то осталось: %v", left)
	}
}
