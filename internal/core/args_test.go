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
	p.Client.StreamsPerCred = 12
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
		"-turn 9.9.9.9", "-port 443", "-n 6", "-streams-per-cred 12",
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
