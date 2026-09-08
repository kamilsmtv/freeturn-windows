package main

import (
	"testing"

	"github.com/kamilsmtv/freeturn-windows/internal/core"
)

// Значок в трее обязан говорить то же, что и окно: в режиме VPN запущенное
// ядро - это ещё не связь, пока не поднят туннель.
func TestConnected(t *testing.T) {
	tests := []struct {
		name  string
		state core.State
		tun   TunnelStatus
		want  bool
	}{
		{"ядро остановлено", core.StateStopped, TunnelStatus{}, false},
		{"ядро запускается", core.StateStarting, TunnelStatus{Enabled: true}, false},
		{"туннель ещё не поднят", core.StateRunning, TunnelStatus{Enabled: true}, false},
		{"туннель поднят", core.StateRunning, TunnelStatus{Enabled: true, Up: true}, true},
		{"режим прокси - связь сразу", core.StateRunning, TunnelStatus{}, true},
		{"остановка", core.StateStopping, TunnelStatus{Enabled: true, Up: true}, false},
		{"ядро упало", core.StateFailed, TunnelStatus{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := connected(tt.state, tt.tun); got != tt.want {
				t.Errorf("connected = %v, ожидалось %v", got, tt.want)
			}
		})
	}
}

func TestTrayDetail(t *testing.T) {
	tests := []struct {
		name string
		st   core.Status
		tun  TunnelStatus
		want string
	}{
		{"ошибка ядра важнее всего", core.Status{State: core.StateFailed, Error: "не запустилось"}, TunnelStatus{}, "не запустилось"},
		{"прокси - без подробностей", core.Status{State: core.StateRunning}, TunnelStatus{}, ""},
		{"туннель поднимается", core.Status{State: core.StateRunning}, TunnelStatus{Enabled: true}, "поднимаю туннель"},
		{"туннель поднят", core.Status{State: core.StateRunning}, TunnelStatus{Enabled: true, Up: true}, "туннель поднят"},
		{"туннель сорвался", core.Status{State: core.StateRunning}, TunnelStatus{Enabled: true, Error: "нет рукопожатия"}, "нет рукопожатия"},
		{"остановлено", core.Status{State: core.StateStopped}, TunnelStatus{Enabled: true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trayDetail(tt.st, tt.tun); got != tt.want {
				t.Errorf("trayDetail = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

// Подписи в трее и в окне должны совпадать по смыслу: иначе человек решит,
// что это разные состояния.
func TestTrayTitle(t *testing.T) {
	tests := []struct {
		name string
		st   core.Status
		tun  TunnelStatus
		want string
	}{
		{"остановлено", core.Status{State: core.StateStopped}, TunnelStatus{}, "Отключено"},
		{"запуск", core.Status{State: core.StateStarting}, TunnelStatus{Enabled: true}, "Запуск ядра…"},
		{"поднимаю туннель", core.Status{State: core.StateRunning}, TunnelStatus{Enabled: true}, "Поднимаю туннель…"},
		{"подключено", core.Status{State: core.StateRunning}, TunnelStatus{Enabled: true, Up: true}, "Подключено"},
		{"прокси", core.Status{State: core.StateRunning}, TunnelStatus{}, "Подключено"},
		{"туннель сорвался", core.Status{State: core.StateRunning}, TunnelStatus{Enabled: true, Error: "нет рукопожатия"}, "Туннель не поднялся"},
		{"остановка", core.Status{State: core.StateStopping}, TunnelStatus{Enabled: true, Up: true}, "Отключение…"},
		{"упало", core.Status{State: core.StateFailed, Error: "нет ядра"}, TunnelStatus{}, "Ошибка"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trayTitle(tt.st, tt.tun); got != tt.want {
				t.Errorf("trayTitle = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}
