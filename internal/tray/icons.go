package tray

import (
	_ "embed"
	"encoding/binary"
)

// Значок в трее несёт состояние цветом: зелёный - подключено, серый -
// отключено, красный - ядро упало. Иконки вшиты отдельными файлами, а не
// берутся из ресурсов .exe: там лежит одна, цветная и с подложкой.
//
//go:embed tray.ico
var iconConnected []byte

//go:embed tray-off.ico
var iconIdle []byte

// На тёмной панели задач тёмно-серый глиф почти не виден, поэтому у
// состояния «отключено» два варианта, и нужный выбирается по теме панели.
//
//go:embed tray-off-light.ico
var iconIdleLight []byte

//go:embed tray-error.ico
var iconFailed []byte

// Переходное состояние: янтарный читается на панели любой темы.
//
//go:embed tray-busy.ico
var iconBusy []byte

// Значок реализован напрямую через Shell_NotifyIcon, а не библиотекой:
// готовые обёртки открывают меню по любой кнопке мыши, а нам нужно левой
// показывать окно и только правой - меню. Заодно мы полностью управляем
// потоком, на котором крутится цикл сообщений.

// iconForState выбирает иконку под состояние: цвет значка и есть индикатор.
//
// lightGlyph - панель задач тёмная и глиф должен быть светлым. Зелёный и
// красный читаются на любой панели, а серый - нет.
func iconForState(s State, lightGlyph bool) []byte {
	switch {
	case s.Failed:
		return iconFailed
	case s.Connected:
		return iconConnected
	case s.Busy:
		return iconBusy
	case lightGlyph:
		return iconIdleLight
	default:
		return iconIdle
	}
}

// pickIconEntry возвращает изображение из .ico, ближайшее к нужному размеру.
func pickIconEntry(ico []byte, want int32) ([]byte, bool) {
	const dirSize, entrySize = 6, 16
	if len(ico) < dirSize {
		return nil, false
	}
	if binary.LittleEndian.Uint16(ico) != 0 || binary.LittleEndian.Uint16(ico[2:]) != 1 {
		return nil, false
	}
	count := int(binary.LittleEndian.Uint16(ico[4:]))
	if count == 0 {
		return nil, false
	}

	best, bestDiff := -1, int32(1<<30)
	for i := 0; i < count; i++ {
		off := dirSize + i*entrySize
		if off+entrySize > len(ico) {
			break
		}
		width := int32(ico[off])
		if width == 0 {
			width = 256 // нулём в каталоге кодируется 256
		}
		diff := width - want
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			best, bestDiff = i, diff
		}
	}
	if best < 0 {
		return nil, false
	}

	off := dirSize + best*entrySize
	size := binary.LittleEndian.Uint32(ico[off+8:])
	start := binary.LittleEndian.Uint32(ico[off+12:])
	if size == 0 || uint64(start)+uint64(size) > uint64(len(ico)) {
		return nil, false
	}
	return ico[start : start+size], true
}
