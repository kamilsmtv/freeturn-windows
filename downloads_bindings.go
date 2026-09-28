package main

import (
	"github.com/kamilsmtv/freeturn-windows/internal/paths"
	"github.com/kamilsmtv/freeturn-windows/internal/updater"
	"github.com/kamilsmtv/freeturn-windows/internal/wintun"
)

// DownloadLink - один файл, который можно принести руками.
type DownloadLink struct {
	Title string `json:"title"`
	// Hint объясняет, зачем файл нужен и что с ним делать дальше.
	Hint string `json:"hint"`
	URL  string `json:"url"`
	// Dir - куда приложение положит файл, когда его укажут кнопкой
	// «Выбрать файл». Показываем, чтобы было видно: руками раскладывать
	// ничего не нужно.
	Dir string `json:"dir"`
	// Optional - без файла можно обойтись (например checksums.txt).
	Optional bool `json:"optional"`
}

// Downloads - адреса всего, что приложение качает само.
//
// Нужны они в единственном случае, зато частом: сайты недоступны, а
// поднять VPN без этих же файлов нельзя. Замкнутый круг размыкается
// так - человек открывает ссылку там, где сеть есть (с телефона, с
// другой машины), приносит файл и указывает его кнопкой «Выбрать файл».
//
// Прямые ссылки на GitHub строим через releases/latest/download: версия
// в них не зашита, и адрес не протухает с выходом нового релиза.
func (a *App) Downloads() []DownloadLink {
	coreDir, _ := paths.Core()
	binDir, _ := paths.Bin()

	coreBase := "https://github.com/" + CoreRepo + "/releases/latest/download/"
	return []DownloadLink{
		{
			Title: "Ядро free-turn-proxy",
			Hint:  "Без него приложение не подключится. Укажите кнопкой «Выбрать файл» в карточке ядра.",
			URL:   coreBase + updater.AssetName,
			Dir:   coreDir,
		},
		{
			Title:    "Контрольные суммы ядра",
			Hint:     "Не обязателен. Положите рядом со скачанным ядром - приложение сверит файл по сумме.",
			URL:      coreBase + updater.ChecksumsName,
			Dir:      coreDir,
			Optional: true,
		},
		{
			Title: "Компонент сетевого адаптера (wintun " + wintun.Version + ")",
			Hint:  "Нужен только в режиме VPN. Подойдёт и архив, и распакованная wintun.dll.",
			URL:   wintun.ArchiveURL,
			Dir:   binDir,
		},
		{
			Title: "Само приложение FreeTurn",
			Hint:  "Страница релизов: оттуда берутся обновления окна.",
			URL:   "https://github.com/" + GUIRepo + "/releases/latest",
		},
	}
}
