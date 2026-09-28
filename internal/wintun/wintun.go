// Package wintun добывает библиотеку wintun.dll, без которой в Windows не
// создать виртуальный сетевой адаптер.
//
// Библиотека не вкомпилирована в приложение намеренно: её лицензия
// (Prebuilt Binaries License от WireGuard LLC) разрешает распространение
// лишь вместе с ПО, использующим её через документированный API, а чужой
// проприетарный бинарь внутри GPL-приложения - лишний повод для спора.
// Поэтому файл скачивается с официального сайта и проверяется по SHA256,
// ровно как бинарь ядра.
package wintun

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Версия и контрольные суммы официальной сборки. Подставлять сюда чужие
// значения нельзя: библиотека получает права на создание сетевых адаптеров.
const (
	Version    = "0.14.1"
	ArchiveURL = "https://www.wintun.net/builds/wintun-" + Version + ".zip"

	archiveSHA256 = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51"
	dllSHA256     = "e5da8447dc2c320edc0fc52fa01885c103de8c118481f683643cacc3220dafce"

	dllPathInZip = "wintun/bin/amd64/wintun.dll"
)

// FileName - имя библиотеки; менять нельзя, загрузчик Windows ищет его по имени.
const FileName = "wintun.dll"

// Path возвращает путь к библиотеке внутри каталога dir.
func Path(dir string) string { return filepath.Join(dir, FileName) }

// Installed сообщает, лежит ли проверенная библиотека на месте.
func Installed(dir string) bool {
	data, err := os.ReadFile(Path(dir))
	if err != nil {
		return false
	}
	return sum(data) == dllSHA256
}

// Ensure докачивает библиотеку, если её нет. onProgress получает
// скачанные и общие байты (общие известны не всегда).
func Ensure(ctx context.Context, dir string, onProgress func(done, total int64)) error {
	if Installed(dir) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	archive, err := download(ctx, onProgress)
	if err != nil {
		return err
	}
	if got := sum(archive); got != archiveSHA256 {
		return fmt.Errorf("контрольная сумма архива wintun не совпала: ожидалась %s, получена %s", archiveSHA256, got)
	}

	dll, err := extract(archive)
	if err != nil {
		return err
	}
	if got := sum(dll); got != dllSHA256 {
		return fmt.Errorf("контрольная сумма wintun.dll не совпала: ожидалась %s, получена %s", dllSHA256, got)
	}

	tmp := Path(dir) + ".download"
	defer func() { _ = os.Remove(tmp) }()
	if err := os.WriteFile(tmp, dll, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, Path(dir))
}

func download(ctx context.Context, onProgress func(done, total int64)) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ArchiveURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "FreeTurn-Windows")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("не удалось скачать wintun: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("не удалось скачать wintun: %s", resp.Status)
	}

	var buf bytes.Buffer
	// Архив весит около мегабайта; ограничение защищает от подмены ответа.
	if _, err := io.Copy(&progressWriter{w: &buf, total: resp.ContentLength, on: onProgress},
		io.LimitReader(resp.Body, 16<<20)); err != nil {
		return nil, fmt.Errorf("скачивание wintun прервано: %w", err)
	}
	return buf.Bytes(), nil
}

func extract(archive []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("архив wintun повреждён: %w", err)
	}
	for _, f := range r.File {
		if f.Name != dllPathInZip {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer func() { _ = rc.Close() }()
		return io.ReadAll(io.LimitReader(rc, 16<<20))
	}
	return nil, errors.New("в архиве wintun нет библиотеки для amd64")
}

func sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

type progressWriter struct {
	w     io.Writer
	done  int64
	total int64
	on    func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.on != nil {
		p.on(p.done, p.total)
	}
	return n, err
}

// InstallFromFile ставит библиотеку из файла, принесённого руками.
//
// Нужен там, где wintun.net недоступен без VPN, а VPN не поднять без
// библиотеки. Принимается и официальный архив, и распакованная dll:
// проверка та же самая - SHA256 должен совпасть с зашитым, поэтому
// подсунуть чужой бинарь этим путём не выйдет.
func InstallFromFile(dir, path string) error {
	data, err := readLimited(path)
	if err != nil {
		return err
	}

	dll := data
	if sum(data) != dllSHA256 {
		// Не библиотека - значит, должен быть архив с ней внутри.
		extracted, err := extract(data)
		if err != nil {
			return fmt.Errorf("это не wintun %s: ни библиотека, ни её архив (%w)", Version, err)
		}
		dll = extracted
	}
	if got := sum(dll); got != dllSHA256 {
		return fmt.Errorf("контрольная сумма wintun.dll не совпала: ожидалась %s, получена %s", dllSHA256, got)
	}

	// Библиотека версии Version всегда одна и та же, так что переписывать
	// уже лежащую нечем. А если туннель поднят, файл открыт системой, и
	// замена сорвалась бы на ровном месте.
	if Installed(dir) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, FileName+".*.part")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(dll); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), Path(dir))
}

// maxFileSize - потолок для файла, выбранного руками. Официальный архив
// весит около мегабайта; без ограничения случайно выбранный образ диска
// уехал бы в память целиком и подвесил окно.
const maxFileSize = 64 << 20

// readLimited читает файл целиком, отказываясь от заведомо чужих размеров.
func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать файл: %w", err)
	}
	defer func() { _ = f.Close() }()

	if st, err := f.Stat(); err == nil {
		if st.IsDir() {
			return nil, errors.New("выбран каталог, а нужен архив или библиотека")
		}
		if st.Size() > maxFileSize {
			return nil, fmt.Errorf("файл великоват для wintun: %d МиБ - это точно не тот архив", st.Size()>>20)
		}
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileSize))
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать файл: %w", err)
	}
	return data, nil
}
