package ping

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func meter(urls ...string) *Meter {
	m := New()
	m.targets = urls
	return m
}

func TestWarmUpOnFirstMeasure(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	m := meter(srv.URL)
	if _, _, err := m.Measure(context.Background()); err != nil {
		t.Fatalf("Measure: %v", err)
	}
	if hits.Load() != 2 {
		t.Errorf("запросов %d, ожидался прогрев и замер", hits.Load())
	}
	if _, _, err := m.Measure(context.Background()); err != nil {
		t.Fatalf("Measure: %v", err)
	}
	if hits.Load() != 3 {
		t.Errorf("тёплый замер прогрелся заново: запросов %d", hits.Load())
	}
	if _, remote, _ := m.Measure(context.Background()); !remote.IsLoopback() {
		t.Errorf("адрес узла не определён: %v", remote)
	}
}

func TestFallsBackToSecondTarget(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	alive := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer alive.Close()

	if _, _, err := meter(deadURL, alive.URL).Measure(context.Background()); err != nil {
		t.Fatalf("второй адрес не выручил: %v", err)
	}
}

func TestNoAnswer(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	defer slow.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, _, err := meter(slow.URL).Measure(ctx); err == nil {
		t.Fatal("молчащий узел дал результат")
	}
}
