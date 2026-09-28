package core

import (
	"strings"
	"testing"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

func base() profile.Profile {
	p := profile.New("test")
	p.Client.ServerAddress = "1.2.3.4:56000"
	p.Client.VKLink = "https://vk.ru/call/join/abc"
	p.Client.Routes = false
	return p
}

func joined(args []string) string { return strings.Join(args, " ") }

// hasFlag ищет флаг как отдельный аргумент: подстрока "-mode" встречается
// и внутри "-dns-mode".
func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func TestBuildArgsMinimal(t *testing.T) {
	got := joined(BuildArgs(base()))
	want := "-peer 1.2.3.4:56000 -links https://vk.ru/call/join/abc"
	if got != want {
		t.Fatalf("BuildArgs = %q, want %q", got, want)
	}
}

func TestBuildArgsSkipsDefaults(t *testing.T) {
	p := base()
	p.Client.Threads = profile.DefaultN
	p.Client.StreamsPerCred = profile.DefaultStreamsPerCred
	p.Client.LocalPort = profile.DefaultListen
	p.Client.DNSMode = profile.DNSAuto
	p.Client.Platform = profile.PlatformDesktop
	p.Client.Provider = profile.ProviderVK

	for _, flag := range []string{"-n", "-streams-per-cred", "-listen", "-dns-mode", "-platform", "-provider"} {
		if strings.Contains(joined(BuildArgs(p)), flag) {
			t.Errorf("дефолтное значение не должно давать флаг %s", flag)
		}
	}
}

func TestBuildArgsFull(t *testing.T) {
	p := base()
	p.Client.Threads = 6
	p.Client.StreamsPerCred = 6
	p.Client.LocalPort = "127.0.0.1:9100"
	p.Client.UseUDP = true
	p.Client.ManualCaptcha = true
	p.Client.DNSMode = profile.DNSDoH
	p.Client.CustomDNS = " 1.1.1.1 , 8.8.8.8 "
	p.Client.ClientID = strings.Repeat("a", 32)
	p.Client.Routes = true
	p.Client.DebugMode = true
	p.Client.MagicSwitch = true
	p.Client.MagicTurn = "9.9.9.9"
	p.Client.MagicPort = "443"
	p.Opts.ObfProfile = profile.ObfRtpOpus3
	p.Opts.ObfKey = strings.Repeat("ab", 32)
	p.Opts.ObfTimingMs = 20

	got := joined(BuildArgs(p))
	for _, want := range []string{
		"-turn 9.9.9.9", "-port 443", "-n 6", "-streams-per-cred 6",
		"-listen 127.0.0.1:9100", "-transport udp", "-obf-profile rtpopus3",
		"-obf-timing 20ms", "-manual-captcha", "-dns-mode doh",
		"-dns-servers 1.1.1.1,8.8.8.8", "-routes", "-debug",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("в %q нет %q", got, want)
		}
	}
	if hasFlag(BuildArgs(p), "-mode") {
		t.Error("режим udp - дефолт ядра, флаг -mode писать не нужно")
	}
}

func TestBuildArgsObfTimingRequiresProfile(t *testing.T) {
	p := base()
	p.Opts.ObfProfile = profile.ObfNone
	p.Opts.ObfTimingMs = 20
	if strings.Contains(joined(BuildArgs(p)), "-obf-timing") {
		t.Error("без профиля обфускации ядро отвергает -obf-timing")
	}
}

func TestBuildArgsKCPOnlyInTCP(t *testing.T) {
	p := base()
	p.Opts.KCP = profile.MobileKCP()

	if strings.Contains(joined(BuildArgs(p)), "-kcp-") {
		t.Fatal("в режиме udp флаги -kcp-* запрещены ядром")
	}

	p.Opts.ProxyMode = profile.ModeTCP
	got := joined(BuildArgs(p))
	for _, want := range []string{"-mode tcp", "-kcp-interval 40", "-kcp-sndwnd 256", "-kcp-rcvwnd 256", "-kcp-acknodelay=false"} {
		if !strings.Contains(got, want) {
			t.Errorf("в %q нет %q", got, want)
		}
	}
	if strings.Contains(got, "-kcp-mtu") {
		t.Error("MTU в мобильном профиле дефолтный, флаг лишний")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*profile.Profile)
		wantErr bool
	}{
		{"валидный", func(*profile.Profile) {}, false},
		{"нет peer", func(p *profile.Profile) { p.Client.ServerAddress = "" }, true},
		{"кривой peer", func(p *profile.Profile) { p.Client.ServerAddress = "1.2.3.4" }, true},
		{"нет ссылки VK", func(p *profile.Profile) { p.Client.VKLink = "" }, true},
		{"кривой listen", func(p *profile.Profile) { p.Client.LocalPort = "9000" }, true},
		{"обфускация без ключа", func(p *profile.Profile) { p.Opts.ObfProfile = profile.ObfRtpOpus }, true},
		{"короткий client-id", func(p *profile.Profile) { p.Client.ClientID = "abc" }, true},
		{"пейсинг вне диапазона", func(p *profile.Profile) {
			p.Opts.ObfProfile, p.Opts.ObfKey = profile.ObfRtpOpus, strings.Repeat("ab", 32)
			p.Opts.ObfTimingMs = 100
		}, true},
		{"кривой KCP в tcp", func(p *profile.Profile) {
			p.Opts.ProxyMode = profile.ModeTCP
			p.Opts.KCP.MTU = 5000
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base()
			tt.mutate(&p)
			if err := Validate(p); (err != nil) != tt.wantErr {
				t.Fatalf("Validate = %v, ожидалась ошибка: %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseVersionLine(t *testing.T) {
	tests := map[string]string{
		"2026/09/07 12:00:00 [INFO] Free Turn Proxy client version=1.4.2": "1.4.2",
		"[INFO] Free Turn Proxy client version=dev":                       "dev",
		"[INFO] provider=vk": "",
	}
	for line, want := range tests {
		if got := ParseVersionLine(line); got != want {
			t.Errorf("ParseVersionLine(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestRedactArgsHidesObfKey(t *testing.T) {
	key := strings.Repeat("ab", 32)
	got := redactArgs([]string{"-obf-profile", "rtpopus3", "-obf-key", key})
	if strings.Contains(got, key) {
		t.Fatalf("ключ обфускации попал в журнал: %q", got)
	}
}

// socksProfile - профиль с включённой раздачей и валидным окружением.
func socksProfile() profile.Profile {
	p := base()
	p.Client.TunnelTransport = "wireguard"
	p.Client.SocksHotspot = true
	p.Client.SocksListen = profile.DefaultSocksListen
	p.Client.SocksUser = "kamil"
	p.Client.SocksPassword = "секрет"
	return p
}

func TestValidateSocksOK(t *testing.T) {
	if err := Validate(socksProfile()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// Забытая галка раздачи в прокси-профиле не должна мешать подключению:
// раздачу приложение просто не поднимет.
func TestValidateSocksIgnoredWithoutTunnel(t *testing.T) {
	p := socksProfile()
	p.Client.TunnelTransport = "none"
	p.Client.SocksUser, p.Client.SocksPassword = "", ""
	if err := Validate(p); err != nil {
		t.Errorf("прокси-профиль с раздачей отвергнут: %v", err)
	}
}

func TestValidateSocksNeedsPassword(t *testing.T) {
	p := socksProfile()
	p.Client.SocksUser = ""
	if err := Validate(p); err == nil {
		t.Error("раздача принята без логина")
	}

	p = socksProfile()
	p.Client.SocksPassword = ""
	if err := Validate(p); err == nil {
		t.Error("раздача принята без пароля")
	}
}

// Одинаковый номер порта у ядра и раздачи - не ошибка: ядро слушает UDP,
// раздача TCP. Запрещать такой профиль было бы выдумкой.
func TestValidateSocksSamePortAllowed(t *testing.T) {
	p := socksProfile()
	p.Client.LocalPort = "127.0.0.1:1080"
	p.Client.SocksListen = "0.0.0.0:1080"
	if err := Validate(p); err != nil {
		t.Errorf("профиль отвергнут: %v", err)
	}
}

func TestValidateSocksBadListen(t *testing.T) {
	p := socksProfile()
	p.Client.SocksListen = "1080"
	if err := Validate(p); err == nil {
		t.Error("адрес раздачи без хоста принят")
	}
}

// Профиль, созданный до появления раздачи, поля адреса не имеет вовсе -
// приложение подставит дефолт, а не заведёт человека в тупик.
func TestValidateSocksEmptyListen(t *testing.T) {
	p := socksProfile()
	p.Client.SocksListen = ""
	if err := Validate(p); err != nil {
		t.Errorf("пустой адрес раздачи отвергнут: %v", err)
	}
}

// Выключенная раздача не должна мешать: её поля могут остаться пустыми
// от старого профиля.
func TestValidateSocksDisabled(t *testing.T) {
	p := base()
	p.Client.SocksHotspot = false
	p.Client.SocksListen, p.Client.SocksUser, p.Client.SocksPassword = "", "", ""
	if err := Validate(p); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// Провайдер direct ходит прямо на -peer, и флаги реле ядро игнорирует.
// Писать их всё равно - значит путать того, кто читает журнал запуска.
func TestBuildArgsDirectDropsRelayFlags(t *testing.T) {
	p := base()
	p.Client.Provider = profile.ProviderDirect
	p.Client.UseUDP = true
	p.Client.ManualCaptcha = true
	p.Client.Platform = profile.PlatformMobile
	p.Client.StreamsPerCred = 6
	p.Client.MagicSwitch = true
	p.Client.MagicTurn = "9.9.9.9"
	p.Client.MagicPort = "443"

	args := BuildArgs(p)
	got := joined(args)
	if !strings.Contains(got, "-provider direct") {
		t.Errorf("в %q нет провайдера", got)
	}
	for _, unwanted := range []string{"-links", "-transport", "-manual-captcha", "-platform", "-streams-per-cred", "-turn", "-port"} {
		if hasFlag(args, unwanted) {
			t.Errorf("в режиме direct не нужен %q: %s", unwanted, got)
		}
	}
}

// У direct свой дефолт потоков - единица, и писать её незачем.
func TestBuildArgsDirectThreadsDefault(t *testing.T) {
	p := base()
	p.Client.Provider = profile.ProviderDirect
	p.Client.Threads = profile.DefaultNDirect
	if hasFlag(BuildArgs(p), "-n") {
		t.Error("-n равен дефолту direct, писать его не нужно")
	}

	p.Client.Threads = profile.DefaultN
	if !hasFlag(BuildArgs(p), "-n") {
		t.Error("12 потоков в режиме direct - не дефолт, флаг нужен")
	}
}

// bond включается только в режиме tcp: в udp ядро его не знает.
func TestBuildArgsBond(t *testing.T) {
	p := base()
	p.Client.Bond = true
	if hasFlag(BuildArgs(p), "-bond") {
		t.Error("-bond в режиме udp писать нельзя")
	}

	p.Opts.ProxyMode = profile.ModeTCP
	if !hasFlag(BuildArgs(p), "-bond") {
		t.Error("-bond в режиме tcp не выставлен")
	}
}

// В режиме direct ссылка на звонок не нужна: реле не используется.
func TestValidateDirectWithoutLink(t *testing.T) {
	p := base()
	p.Client.Provider = profile.ProviderDirect
	p.Client.VKLink = ""
	if err := Validate(p); err != nil {
		t.Errorf("профиль direct отвергнут: %v", err)
	}

	p.Client.Provider = profile.ProviderVK
	if err := Validate(p); err == nil {
		t.Error("профиль vk без ссылки на звонок принят")
	}
}

func TestValidateBondNeedsTCP(t *testing.T) {
	p := base()
	p.Client.Bond = true
	if err := Validate(p); err == nil {
		t.Error("bond принят в режиме udp")
	}
	p.Opts.ProxyMode = profile.ModeTCP
	if err := Validate(p); err != nil {
		t.Errorf("bond отвергнут в режиме tcp: %v", err)
	}
}

// Выбор бэкенда живёт в профиле, но режим проброса tcp решает за него:
// свой WireGuard скрипт поднимает только в udp.
func TestServerBackendDefault(t *testing.T) {
	p := profile.New("сервер")
	if p.ServerBackend != profile.ServerBackendNew {
		t.Errorf("по умолчанию ожидался свой WireGuard, получено %q", p.ServerBackend)
	}
}
