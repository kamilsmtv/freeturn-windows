// Package profile описывает модель профиля подключения.
//
// Структура повторяет модель Android-клиента (data/server/Server.kt и
// ServerJson.kt): имена JSON-ключей - контракт совместимости бэкапов,
// менять их можно только вместе с миграцией.
package profile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
)

// Значения-перечисления ядра (internal/config ядра, data/config/CoreFlags.kt).
const (
	ProviderVK = "vk"

	TransportTCP = "tcp"
	TransportUDP = "udp"

	ModeUDP = "udp"
	ModeTCP = "tcp"

	ObfNone     = "none"
	ObfRtpOpus  = "rtpopus"
	ObfRtpOpus2 = "rtpopus2"
	ObfRtpOpus3 = "rtpopus3"

	DNSAuto  = "auto"
	DNSPlain = "plain"
	DNSDoH   = "doh"

	PlatformDesktop = "desktop"
	PlatformMobile  = "mobile"

	// Максимальный пейсинг мимикрии, мс (ObfProfile.TIMING_MAX в Android).
	ObfTimingMax = 60

	// DefaultListen - дефолт ядра для локального сокета.
	DefaultListen = "127.0.0.1:9000"

	// WGMTU - константа туннеля: WG идёт поверх TURN, 1420 фрагментируется.
	// Совпадает с WG_MTU сервера (scripts/install.sh) и Android-клиентом.
	WGMTU = 1280
)

// Дефолты ядра (internal/config/defaults.go). Флаг, равный дефолту, в
// командную строку не пишется - так же поступает ClientArgs в ядре.
const (
	DefaultN              = 10
	DefaultStreamsPerCred = 10
)

// Client - параметры клиента ядра.
type Client struct {
	ServerAddress  string `json:"serverAddress"`
	VKLink         string `json:"vkLink"`
	Provider       string `json:"provider"`
	Threads        int    `json:"threads"`
	StreamsPerCred int    `json:"streamsPerCred"`
	UseUDP         bool   `json:"useUdp"`
	ManualCaptcha  bool   `json:"manualCaptcha"`
	LocalPort      string `json:"localPort"`
	DebugMode      bool   `json:"debugMode"`
	DNSMode        string `json:"dnsMode"`
	CustomDNS      string `json:"customDns"`
	Platform       string `json:"platform"`
	// MagicSwitch включает ручной TURN-сервер (-turn/-port) из MagicTurn.
	MagicSwitch bool   `json:"magicSwitch"`
	MagicTurn   string `json:"magicTurn"`
	MagicPort   string `json:"magicPort"`
	// Routes включает -routes: ядро само добавляет маршруты к TURN.
	Routes bool `json:"routes"`

	TunnelTransport     string `json:"tunnelTransport"`
	WireGuardConfig     string `json:"wireGuardConfig"`
	WireGuardTunnelName string `json:"wireGuardTunnelName"`
	// SplitTunnelMode/Subnets - раздельное туннелирование по подсетям
	// (на Windows аналога Android per-app нет, см. docs/analysis.md).
	SplitTunnelMode    string `json:"splitTunnelMode"`
	SplitTunnelSubnets string `json:"splitTunnelSubnets"`

	LogsEnabled bool   `json:"logsEnabled"`
	ClientID    string `json:"clientId"`
}

// Opts - параметры, которые должны совпадать с серверными.
type Opts struct {
	ObfProfile  string `json:"obfProfile"`
	ObfKey      string `json:"obfKey"`
	ObfTimingMs int    `json:"obfTimingMs"`
	ProxyMode   string `json:"proxyMode"`
	KCP         KCP    `json:"kcp"`
}

// ObfEnabled - обфускация включена, когда выбран реальный профиль.
func (o Opts) ObfEnabled() bool { return o.ObfProfile != "" && o.ObfProfile != ObfNone }

// TCPMode - режим проброса tcp (Xray/sing-box).
func (o Opts) TCPMode() bool { return o.ProxyMode == ModeTCP }

// KCP - профиль ARQ для -mode tcp. В udp любое отличие от дефолта ядро
// считает фатальной ошибкой старта, поэтому флаги пишутся только в tcp.
type KCP struct {
	NoDelay    int  `json:"noDelay"`
	Interval   int  `json:"interval"`
	Resend     int  `json:"resend"`
	NC         int  `json:"nc"`
	SndWnd     int  `json:"sndWnd"`
	RcvWnd     int  `json:"rcvWnd"`
	MTU        int  `json:"mtu"`
	ACKNoDelay bool `json:"ackNoDelay"`
}

// DefaultKCP повторяет дефолт ядра (internal/config/kcp.go).
func DefaultKCP() KCP {
	return KCP{NoDelay: 1, Interval: 20, Resend: 2, NC: 1, SndWnd: 512, RcvWnd: 512, MTU: 1200, ACKNoDelay: true}
}

// MobileKCP - щадящий профиль для мобильной сети (docs/flags.md ядра).
func MobileKCP() KCP {
	k := DefaultKCP()
	k.Interval, k.SndWnd, k.RcvWnd, k.ACKNoDelay = 40, 256, 256, false
	return k
}

// Valid проверяет диапазоны, которые ядро отвергает на старте.
func (k KCP) Valid() bool {
	return k.NoDelay >= 0 && k.NoDelay <= 1 && k.NC >= 0 && k.NC <= 1 &&
		k.Interval > 0 && k.Resend >= 0 && k.SndWnd > 0 && k.RcvWnd > 0 &&
		k.MTU >= 300 && k.MTU <= 1350
}

// SSH - доступ к VPS для установки ядра.
type SSH struct {
	IP              string `json:"ip"`
	Port            int    `json:"port"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	AuthType        string `json:"authType"`
	SSHKey          string `json:"sshKey"`
	HostFingerprint string `json:"hostFingerprint"`
	RootMode        string `json:"rootMode"`
	SudoPassword    string `json:"sudoPassword"`
}

// Значения authType и rootMode - контракт хранения Android-клиента.
const (
	AuthPassword = "PASSWORD"
	AuthSSHKey   = "SSH_KEY"

	RootDirect     = "ROOT"
	RootSudoNoPass = "SUDO_NOPASS"
	RootSudoPass   = "SUDO_PASS"
)

// Profile - именованный профиль: SSH-доступ, клиентские параметры, серверные опции.
type Profile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	SSH          SSH    `json:"ssh"`
	Client       Client `json:"client"`
	ProxyListen  string `json:"proxyListen"`
	ProxyConnect string `json:"proxyConnect"`
	Opts         Opts   `json:"opts"`
	// SubURL заполняется, когда профиль пришёл из подписки.
	SubURL string `json:"subUrl,omitempty"`
	// Extra хранит поля Android-клиента, которых нет в модели Windows
	// (например список приложений раздельного туннелирования). Мы их не
	// применяем, но переносим без потерь через бэкап.
	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// FallbackName - имя для записей без имени (как в Android-клиенте).
const FallbackName = "Без названия"

// New создаёт профиль с дефолтами.
func New(name string) Profile {
	if name == "" {
		name = FallbackName
	}
	return Profile{
		ID:   NewID(),
		Name: name,
		SSH:  SSH{Port: 22, Username: "root", AuthType: AuthPassword, RootMode: RootDirect},
		Client: Client{
			Provider:            ProviderVK,
			Threads:             DefaultN,
			StreamsPerCred:      DefaultStreamsPerCred,
			LocalPort:           DefaultListen,
			DNSMode:             DNSAuto,
			Platform:            PlatformDesktop,
			Routes:              true,
			TunnelTransport:     "none",
			WireGuardTunnelName: "freeturn-wg",
			SplitTunnelMode:     "all",
			LogsEnabled:         true,
		},
		ProxyListen:  "0.0.0.0:56000",
		ProxyConnect: "127.0.0.1:40537",
		Opts:         Opts{ObfProfile: ObfNone, ProxyMode: ModeUDP, KCP: DefaultKCP()},
	}
}

// Clone возвращает копию профиля с новым ID и именем.
func (p Profile) Clone(name string) Profile {
	c := p
	c.ID = NewID()
	if name != "" {
		c.Name = name
	}
	return c
}

// NewID генерирует идентификатор профиля.
func NewID() string { return randHex(16) }

// NewClientID генерирует client-id в формате ядра: 32 hex-символа.
func NewClientID() string { return randHex(16) }

var clientIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ValidClientID проверяет формат client-id (совпадает с ClientId.isValid Android).
func ValidClientID(id string) bool { return clientIDRe.MatchString(id) }

var obfKeyRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// ValidObfKey проверяет ключ обфускации: 32 байта hex.
func ValidObfKey(key string) bool { return obfKeyRe.MatchString(key) }

// NewObfKey генерирует 32-байтный ключ обфускации в hex.
func NewObfKey() string { return randHex(32) }

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand на Windows не отказывает; молчать здесь опаснее, чем упасть.
		panic("profile: нет источника случайных чисел: " + err.Error())
	}
	return hex.EncodeToString(b)
}
