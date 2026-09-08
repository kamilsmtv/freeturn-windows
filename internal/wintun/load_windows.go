package wintun

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Preload загружает библиотеку из нашего каталога.
//
// Обёртка wintun ищет DLL только рядом с .exe и в System32, а писать рядом
// с .exe мы не имеем права. Но загрузчик Windows сопоставляет модули по
// имени файла: если wintun.dll уже загружена в процесс, повторный
// LoadLibraryEx вернёт её же, откуда бы она ни пришла.
func Preload(dir string) error {
	if !Installed(dir) {
		return fmt.Errorf("библиотека %s не найдена в %s", FileName, dir)
	}
	path, err := windows.UTF16PtrFromString(Path(dir))
	if err != nil {
		return err
	}
	// LOAD_WITH_ALTERED_SEARCH_PATH разрешает загрузку по полному пути.
	if _, err := windows.LoadLibraryEx(windows.UTF16PtrToString(path), 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH); err != nil {
		return fmt.Errorf("не удалось загрузить %s: %w", FileName, err)
	}
	return nil
}
