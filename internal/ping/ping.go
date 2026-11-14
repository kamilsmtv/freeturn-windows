// Package ping меряет задержку через туннель так же, как Android-клиент:
// временем HTTP-запроса к generate_204.
//
// ICMP здесь не годится: на Windows он требует прав или отдельной службы,
// а главное, через TURN-релей ходит только UDP WireGuard, и «пинг до
// сервера» показал бы путь до VK, а не то, что чувствует пользователь.
// Запрос же уходит обычным сокетом, и маршруты системы ведут его в туннель.
package ping

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

// Targets - проверочные адреса по очереди: второй выручает, если первый
// недоступен из сети сервера.
var Targets = []string{
	"https://www.google.com/generate_204",
	"https://cp.cloudflare.com/generate_204",
}

// timeout - сколько ждём один ответ.
const timeout = 3 * time.Second

// staleAfter - после такого простоя соединение считаем остывшим и сначала
// прогреваем: DNS, TCP и TLS к задержке туннеля отношения не имеют.
const staleAfter = time.Minute

// ErrNoAnswer - ни один проверочный адрес не ответил.
var ErrNoAnswer = errors.New("проверочные узлы не ответили")

// Meter переиспользует соединение между замерами.
type Meter struct {
	client  *http.Client
	targets []string

	mu     sync.Mutex
	lastOK time.Time
}

// New создаёт мерило. Системный прокси не используем: он увёл бы запрос
// мимо туннеля.
func New() *Meter {
	tr := &http.Transport{
		Proxy:               nil,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        2,
		IdleConnTimeout:     2 * time.Minute,
		TLSHandshakeTimeout: timeout,
	}
	return &Meter{
		client: &http.Client{
			Transport: tr,
			Timeout:   timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		targets: Targets,
	}
}

// Measure возвращает задержку одного запроса. Замеры не идут параллельно:
// второй клик во время первого дождётся его и сделает свой.
func (m *Meter) Measure(ctx context.Context) (time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, url := range m.targets {
		if time.Since(m.lastOK) > staleAfter {
			if _, err := m.probe(ctx, url); err != nil {
				m.reset()
				continue
			}
		}
		d, err := m.probe(ctx, url)
		if err != nil {
			m.reset()
			continue
		}
		m.lastOK = time.Now()
		return d, nil
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return 0, ErrNoAnswer
}

// Reset забывает соединение: после переподключения туннеля старое
// соединение мертво, и первый замер на нём только потратил бы таймаут.
func (m *Meter) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reset()
}

func (m *Meter) reset() {
	m.lastOK = time.Time{}
	m.client.CloseIdleConnections()
}

func (m *Meter) probe(ctx context.Context, url string) (time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	resp, err := m.client.Do(req)
	if err != nil {
		return 0, err
	}
	d := time.Since(start)
	_ = resp.Body.Close()
	return d, nil
}
