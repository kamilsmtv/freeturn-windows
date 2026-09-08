// Package netstat читает счётчики сетевых адаптеров.
//
// Ядро free-turn-proxy не печатает объём трафика (счётчики internal/stats
// доступны только gomobile-сборке, см. docs/analysis.md), поэтому в
// VPN-режиме цифры берём у адаптера WireGuard: это ровно тот трафик,
// который прошёл через туннель. В proxy-режиме адаптера нет и честного
// источника тоже - там счётчики недоступны.
package netstat

// Counters - показания адаптера.
type Counters struct {
	// Available сообщает, удалось ли найти адаптер и снять показания.
	Available bool   `json:"available"`
	Adapter   string `json:"adapter"`
	RxBytes   uint64 `json:"rxBytes"`
	TxBytes   uint64 `json:"txBytes"`
	Reason    string `json:"reason"`
}

// Read снимает показания адаптера по его имени в системе (FriendlyName).
// Имя пустое или адаптер не найден - Available=false с пояснением.
func Read(adapter string) Counters {
	if adapter == "" {
		return Counters{Reason: "адаптер не задан"}
	}
	return read(adapter)
}
