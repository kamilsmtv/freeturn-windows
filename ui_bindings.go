package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/netstat"
	"github.com/kamilsmtv/freeturn-windows/internal/qr"
	"github.com/kamilsmtv/freeturn-windows/internal/wg"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ExportLog сохраняет журнал ядра в текстовый файл.
func (a *App) ExportLog() (string, error) {
	lines := a.core.Log()
	if len(lines) == 0 {
		return "", errors.New("журнал пуст")
	}

	path, err := a.saveFileDialog("Сохранить журнал",
		"freeturn-log-"+time.Now().Format("2006-01-02-1504")+".txt",
		[]wailsruntime.FileFilter{{DisplayName: "Текстовый файл (*.txt)", Pattern: "*.txt"}})
	if err != nil || path == "" {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "FreeTurn %s, журнал от %s\r\n\r\n", version, time.Now().Format(time.RFC3339))
	for _, l := range lines {
		// CRLF: файл откроют блокнотом.
		fmt.Fprintf(&b, "%s %s\r\n", l.Time, l.Text)
	}
	return path, os.WriteFile(path, []byte(b.String()), 0o600)
}

// QRCode рисует ссылку QR-кодом и отдаёт картинку как data URI.
//
// Так же делится ссылками Android-клиент: сосед наводит камеру и получает
// профиль целиком, не пересылая длинную строку через мессенджер.
func (a *App) QRCode(text string) (qr.Image, error) {
	return qr.DataURI(text)
}

// SaveQRCode сохраняет QR-код ссылки в PNG-файл.
func (a *App) SaveQRCode(text, name string) (string, error) {
	png, err := qr.PNG(text)
	if err != nil {
		return "", err
	}

	path, err := a.saveFileDialog("Сохранить QR-код", safeFileName(name)+"-qr.png",
		[]wailsruntime.FileFilter{{DisplayName: "Картинка PNG (*.png)", Pattern: "*.png"}})
	if err != nil || path == "" {
		return "", err
	}
	// Тот же режим, что у ссылки в файле: внутри ключи, читать её посторонним незачем.
	return path, os.WriteFile(path, png, 0o600)
}

// PrepareWGConfig подставляет в конфиг WireGuard параметры работы через ядро.
func (a *App) PrepareWGConfig(profileID string) (string, error) {
	p, err := a.profiles.Get(profileID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(p.Client.WireGuardConfig) == "" {
		return "", errors.New("в профиле нет конфигурации WireGuard - вставьте её в разделе «Дополнительно»")
	}
	return wg.Prepare(p.Client.WireGuardConfig, wg.Options{
		Listen:     p.Client.LocalPort,
		SplitMode:  p.Client.SplitTunnelMode,
		Subnets:    p.Client.SplitTunnelSubnets,
		DNSServers: p.Client.CustomDNS,
	})
}

// SaveWGConfig сохраняет подготовленный конфиг в .conf-файл.
func (a *App) SaveWGConfig(profileID string) (string, error) {
	conf, err := a.PrepareWGConfig(profileID)
	if err != nil {
		return "", err
	}
	p, _ := a.profiles.Get(profileID)
	name := p.Client.WireGuardTunnelName
	if name == "" {
		name = "freeturn-wg"
	}

	path, err := a.saveFileDialog("Сохранить конфигурацию WireGuard", safeFileName(name)+".conf",
		[]wailsruntime.FileFilter{{DisplayName: "Конфигурация WireGuard (*.conf)", Pattern: "*.conf"}})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, []byte(conf), 0o600)
}

// WGTemplate - заготовка конфигурации клиента и её ключи.
type WGTemplate struct {
	Config string `json:"config"`
	// PublicKey нужно передать владельцу сервера, чтобы он добавил клиента.
	PublicKey string `json:"publicKey"`
	// NeedsServerKey сообщает, что публичный ключ сервера ещё не вписан.
	NeedsServerKey bool `json:"needsServerKey"`
}

// GenerateWGConfig создаёт конфигурацию клиента со свежими ключами.
//
// Пригодится, когда сервер поднят не через это приложение и забрать готовый
// конфиг по SSH нельзя: остаётся вписать только публичный ключ сервера.
func (a *App) GenerateWGConfig(profileID string) (WGTemplate, error) {
	p, err := a.profiles.Get(profileID)
	if err != nil {
		return WGTemplate{}, err
	}

	conf, keys, err := wg.Template(wg.TemplateOptions{Endpoint: endpointFor(p)})
	if err != nil {
		return WGTemplate{}, err
	}
	return WGTemplate{Config: conf, PublicKey: keys.Public, NeedsServerKey: true}, nil
}

// Traffic - показания счётчиков вместе со скоростью.
type Traffic struct {
	netstat.Counters
	RxRate int64 `json:"rxRate"`
	TxRate int64 `json:"txRate"`
}

// trafficSampler считает скорость по разнице соседних показаний.
type trafficSampler struct {
	mu     sync.Mutex
	rx, tx uint64
	at     time.Time
}

func (s *trafficSampler) rates(rx, tx uint64) (rxRate, txRate int64) {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	prevRx, prevTx, prevAt := s.rx, s.tx, s.at
	s.rx, s.tx, s.at = rx, tx, now

	elapsed := now.Sub(prevAt).Seconds()
	// Первый замер и перезапуск туннеля (счётчик уехал назад) скорости не дают.
	if prevAt.IsZero() || elapsed <= 0 || rx < prevRx || tx < prevTx {
		return 0, 0
	}
	return int64(float64(rx-prevRx) / elapsed), int64(float64(tx-prevTx) / elapsed)
}

// TrafficStats отдаёт счётчики адаптера активного профиля.
//
// Цифры есть только в VPN-режиме: в proxy-режиме адаптера нет, и брать
// объём трафика неоткуда - ядро его не публикует.
func (a *App) TrafficStats() Traffic {
	snap := a.profiles.All()
	p, ok := snap.Active()
	if !ok {
		return Traffic{Counters: netstat.Counters{Reason: "профиль не выбран"}}
	}
	if p.Client.TunnelTransport != "wireguard" {
		return Traffic{Counters: netstat.Counters{
			Reason: "счётчики доступны только в режиме VPN: в режиме прокси ядро объём трафика не публикует",
		}}
	}

	// У встроенного туннеля счётчики точнее адаптерных: они считают именно
	// трафик WireGuard, без служебных пакетов интерфейса.
	if st, ok := a.tunnel.Stats(); ok {
		rxRate, txRate := a.traffic.rates(st.RxBytes, st.TxBytes)
		return Traffic{
			Counters: netstat.Counters{
				Available: true,
				Adapter:   a.tunnel.Name(),
				RxBytes:   st.RxBytes,
				TxBytes:   st.TxBytes,
			},
			RxRate: rxRate,
			TxRate: txRate,
		}
	}

	c := netstat.Read(p.Client.WireGuardTunnelName)
	if !c.Available {
		return Traffic{Counters: c}
	}
	rxRate, txRate := a.traffic.rates(c.RxBytes, c.TxBytes)
	return Traffic{Counters: c, RxRate: rxRate, TxRate: txRate}
}
