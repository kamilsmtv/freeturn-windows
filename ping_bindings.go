package main

import (
	"context"
	"time"
)

// PingResult - один замер задержки для карточки статистики.
type PingResult struct {
	OK bool `json:"ok"`
	// Ms - задержка в миллисекундах; 0, если ответа нет.
	Ms int64 `json:"ms"`
	// Error - причина, если замер не удался.
	Error string `json:"error"`
}

// Ping меряет задержку HTTP-запроса через туннель.
//
// Смысл замер имеет только в режиме VPN: в режиме прокси запрос приложения
// пошёл бы мимо туннеля и показал бы задержку обычной сети. Это решает
// интерфейс - он знает режим и просто не зовёт Ping.
func (a *App) Ping() PingResult {
	if gen := a.connectGen.Load(); a.pingGen.Swap(gen) != gen {
		a.ping.Reset()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := a.ping.Measure(ctx)
	if err != nil {
		return PingResult{Error: err.Error()}
	}
	ms := d.Milliseconds()
	if ms < 1 {
		ms = 1
	}
	return PingResult{OK: true, Ms: ms}
}
