package link

import (
	"errors"
	"strings"
	"testing"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// coreGolden - ссылка, сгенерированная самим ядром (internal/uri.Config.String)
// на наборе полей ниже. Держим как эталон: наш кодировщик обязан давать
// байт в байт то же самое, иначе ссылки разойдутся с ядром и Android-клиентом.
const coreGolden = "freeturn://eyJ2IjoxLCJwcm92aWRlciI6InZrIiwicGVlciI6IjEuMi4zLjQ6NTYwMDAiLCJ0cmFuc3BvcnQiOiJ1ZHAiLCJtb2RlIjoidGNwIiwib2JmIjoicnRwb3B1czMiLCJrZXkiOiJhYWJiIiwibiI6MTIsInNwYyI6NiwiY2lkIjoiY2lkMSIsImxpc3RlbiI6IjEyNy4wLjAuMTo5MTAwIiwiZG5zIjoiZG9oIiwiZG5zcyI6IjEuMS4xLjEiLCJtY2FwIjp0cnVlLCJrY3AiOnsibm9kZWxheSI6MSwiaW50ZXJ2YWwiOjQwLCJyZXNlbmQiOjIsIm5jIjoxLCJzbmR3bmQiOjI1NiwicmN2d25kIjoyNTYsIm10dSI6MTIwMCwiYWNrbm9kZWxheSI6ZmFsc2V9LCJuYW1lIjoi0KHQtdGA0LLQtdGAINCg0KQiLCJ3ZyI6IltJbnRlcmZhY2VdXG5BZGRyZXNzID0gMTAuMTMuMTMuMi8zMiJ9"

func goldenLink() Link {
	return Link{
		V: 1, Provider: "vk", Peer: "1.2.3.4:56000", Transport: "udp", Mode: "tcp",
		Obf: "rtpopus3", Key: "aabb", N: 12, StreamsPerCred: 6, ClientID: "cid1",
		Listen: "127.0.0.1:9100", DNSMode: "doh", DNSServers: "1.1.1.1", ManualCaptcha: true,
		KCP:  &KCP{NoDelay: 1, Interval: 40, Resend: 2, NC: 1, SndWnd: 256, RcvWnd: 256, MTU: 1200, ACKNoDelay: false},
		Name: "Сервер РФ", WGConf: "[Interface]\nAddress = 10.13.13.2/32",
	}
}

func TestEncodeMatchesCore(t *testing.T) {
	if got := goldenLink().Encode(); got != coreGolden {
		t.Fatalf("ссылка разошлась с ядром:\n получено: %s\n эталон:   %s", got, coreGolden)
	}
}

func TestParseCoreGolden(t *testing.T) {
	l, err := Parse(coreGolden)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if l.Peer != "1.2.3.4:56000" || l.Name != "Сервер РФ" || l.Mode != "tcp" {
		t.Errorf("разобрано неверно: %+v", l)
	}
	if l.KCP == nil || l.KCP.Interval != 40 || l.KCP.ACKNoDelay {
		t.Errorf("профиль KCP разобран неверно: %+v", l.KCP)
	}
	if !strings.Contains(l.WGConf, "[Interface]") {
		t.Errorf("конфиг WireGuard потерян: %q", l.WGConf)
	}
}

func TestParseRejects(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want error
	}{
		"чужая схема":     {"vless://abc", ErrScheme},
		"пустая нагрузка": {Scheme, ErrEmpty},
		"не base64":       {Scheme + "!!!", ErrBase64},
		// {"v":2,"provider":"vk","peer":"1.2.3.4:1"}
		"чужая версия": {Scheme + "eyJ2IjoyLCJwcm92aWRlciI6InZrIiwicGVlciI6IjEuMi4zLjQ6MSJ9", ErrVersion},
		// {"v":1,"provider":"vk"}
		"нет peer": {Scheme + "eyJ2IjoxLCJwcm92aWRlciI6InZrIn0", ErrPeer},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(tt.raw); !errors.Is(err, tt.want) {
				t.Errorf("Parse = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestParseAcceptsPadding(t *testing.T) {
	// Некоторые генераторы добавляют padding, которого RawURLEncoding не ждёт.
	if _, err := Parse(coreGolden + "=="); err != nil {
		t.Errorf("padding не должен ломать разбор: %v", err)
	}
}

func TestProfileRoundTrip(t *testing.T) {
	p := profile.New("RU-1")
	p.Client.ServerAddress = "5.6.7.8:56000"
	p.Client.VKLink = "https://vk.ru/call/join/abc"
	p.Client.Threads = 6
	p.Client.UseUDP = true
	p.Client.LocalPort = "127.0.0.1:9100"
	p.Opts.ProxyMode = profile.ModeTCP
	p.Opts.KCP = profile.MobileKCP()
	p.Opts.ObfProfile = profile.ObfRtpOpus2
	p.Opts.ObfKey = strings.Repeat("ab", 32)

	// Без явного согласия ссылка на звонок не уезжает получателю.
	if l := FromProfile(p, false, ""); l.VKLink != "" {
		t.Error("ссылка на звонок не должна попадать в share-ссылку без согласия")
	}

	back, err := Parse(FromProfile(p, true, "").Encode())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := back.ToProfile()

	if got.Client.ServerAddress != p.Client.ServerAddress ||
		got.Client.VKLink != p.Client.VKLink ||
		got.Client.Threads != p.Client.Threads ||
		!got.Client.UseUDP ||
		got.Client.LocalPort != p.Client.LocalPort ||
		got.Opts.ProxyMode != profile.ModeTCP ||
		got.Opts.KCP != profile.MobileKCP() ||
		got.Opts.ObfKey != p.Opts.ObfKey {
		t.Errorf("после круга параметры разошлись:\n было: %+v\n стало: %+v", p, got)
	}
	if got.ID == p.ID {
		t.Error("импортированный профиль должен получать свой ID")
	}
}

// В конфигурации WireGuard лежит приватный ключ владельца: получатель
// подключался бы под его личностью.
func TestFromProfileNeverSharesOwnerWGConf(t *testing.T) {
	p := profile.New("RU-1")
	p.Client.ServerAddress = "1.2.3.4:56000"
	p.Client.WireGuardConfig = "[Interface]\nPrivateKey = QFtZkBQ0d1wY0aWJ8Vd0AGTe0HeNiKJRnEHXbUYXV1c=\n"

	l := FromProfile(p, true, "cid")
	if l.WGConf != "" {
		t.Fatalf("конфигурация владельца попала в ссылку: %q", l.WGConf)
	}
	if strings.Contains(l.Encode(), "QFtZkBQ0d1wY") {
		t.Fatal("приватный ключ владельца оказался в ссылке")
	}

	// Гостевую конфигурацию вкладываем явно.
	guest := l.WithWGConf("[Interface]\nPrivateKey = гостевой\n")
	if guest.WGConf == "" {
		t.Error("конфигурация гостя должна попадать в ссылку")
	}
}

func TestFromProfileOmitsDefaults(t *testing.T) {
	p := profile.New("test")
	p.Client.ServerAddress = "1.2.3.4:56000"
	l := FromProfile(p, false, "")

	if l.Listen != "" || l.DNSMode != "" || l.Mode != "" || l.Transport != "" || l.KCP != nil {
		t.Errorf("дефолтные значения не должны попадать в ссылку: %+v", l)
	}
	if l.Obf != "" || l.Key != "" {
		t.Errorf("выключенная обфускация не должна попадать в ссылку: %+v", l)
	}
}

func TestFromProfileDropsInvalidObfKey(t *testing.T) {
	p := profile.New("test")
	p.Client.ServerAddress = "1.2.3.4:56000"
	p.Opts.ObfProfile = profile.ObfRtpOpus
	p.Opts.ObfKey = "коротко"

	if l := FromProfile(p, false, ""); l.Key != "" {
		t.Error("негодный ключ обфускации в ссылку не пишется")
	}
}

func TestNormalizeWGConf(t *testing.T) {
	got := NormalizeWGConf("# комментарий\n[Interface]\n  Address = 10.0.0.2/32  \nMTU = 1420\n\n; ещё\nPrivateKey = xxx")
	want := "[Interface]\nAddress = 10.0.0.2/32\nPrivateKey = xxx"
	if got != want {
		t.Errorf("NormalizeWGConf = %q, want %q", got, want)
	}
}
