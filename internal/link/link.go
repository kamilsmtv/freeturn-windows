// Package link разбирает и собирает share-ссылки freeturn://.
//
// Схема повторяет internal/uri ядра (docs/uri.md): freeturn:// +
// base64url-без-padding от JSON с версией v=1. Поле "vk" - расширение
// Android-клиента (data/share/FreeturnLink.kt): ядро его игнорирует,
// а нам оно экономит ручной ввод ссылки на звонок.
package link

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// Scheme - префикс share-ссылки.
const Scheme = "freeturn://"

// Version - текущая версия формата. Чужую версию парсер отвергает.
const Version = 1

// Link - полезная нагрузка ссылки. Порядок полей повторяет структуру wire
// ядра: json.Marshal пишет поля в порядке объявления, и ссылки, собранные
// нами и ядром, совпадают байт в байт.
type Link struct {
	V              int    `json:"v"`
	Provider       string `json:"provider"`
	Peer           string `json:"peer"`
	Transport      string `json:"transport,omitempty"`
	Mode           string `json:"mode,omitempty"`
	Obf            string `json:"obf,omitempty"`
	Key            string `json:"key,omitempty"`
	N              int    `json:"n,omitempty"`
	StreamsPerCred int    `json:"spc,omitempty"`
	ClientID       string `json:"cid,omitempty"`
	Listen         string `json:"listen,omitempty"`
	DNSMode        string `json:"dns,omitempty"`
	DNSServers     string `json:"dnss,omitempty"`
	ManualCaptcha  bool   `json:"mcap,omitempty"`
	KCP            *KCP   `json:"kcp,omitempty"`
	Name           string `json:"name,omitempty"`
	// VKLink - расширение Android-клиента, не входит в схему ядра.
	VKLink string `json:"vk,omitempty"`
	WGConf string `json:"wg,omitempty"`
}

// KCP - профиль ARQ в ссылке. Ключи строчные, как в json-тегах uri.KCP ядра,
// а не camelCase, как в конфиге Android.
type KCP struct {
	NoDelay    int  `json:"nodelay"`
	Interval   int  `json:"interval"`
	Resend     int  `json:"resend"`
	NC         int  `json:"nc"`
	SndWnd     int  `json:"sndwnd"`
	RcvWnd     int  `json:"rcvwnd"`
	MTU        int  `json:"mtu"`
	ACKNoDelay bool `json:"acknodelay"`
}

// Ошибки разбора ссылки.
var (
	ErrScheme   = errors.New("это не ссылка freeturn:// - проверьте, что скопирована вся строка")
	ErrEmpty    = errors.New("ссылка пустая")
	ErrBase64   = errors.New("ссылка повреждена: не удалось декодировать содержимое")
	ErrJSON     = errors.New("ссылка повреждена: неверный формат данных")
	ErrVersion  = errors.New("версия ссылки не поддерживается - обновите приложение")
	ErrProvider = errors.New("в ссылке не указан провайдер")
	ErrPeer     = errors.New("в ссылке не указан адрес сервера")
)

// LooksLike сообщает, похожа ли строка на share-ссылку.
func LooksLike(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(raw)), Scheme)
}

// Parse разбирает freeturn://-ссылку.
func Parse(raw string) (Link, error) {
	s := strings.TrimSpace(raw)
	if !LooksLike(s) {
		return Link{}, ErrScheme
	}
	payload := s[len(Scheme):]
	if payload == "" {
		return Link{}, ErrEmpty
	}

	// Ядро кодирует base64url без padding; лишний padding из чужих
	// генераторов принимаем молча.
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(payload, "="))
	if err != nil {
		return Link{}, ErrBase64
	}

	var l Link
	if err := json.Unmarshal(data, &l); err != nil {
		return Link{}, ErrJSON
	}
	if l.V != Version {
		return Link{}, ErrVersion
	}
	if l.Provider == "" {
		return Link{}, ErrProvider
	}
	if l.Peer == "" {
		return Link{}, ErrPeer
	}
	return l, nil
}

// Encode собирает строку freeturn://.
func (l Link) Encode() string {
	l.V = Version
	// Ключ без профиля смысла не имеет и в ссылку не попадает.
	if l.Obf == "" || l.Obf == profile.ObfNone {
		l.Obf, l.Key = "", ""
	}
	data, err := json.Marshal(l)
	if err != nil {
		return ""
	}
	return Scheme + base64.RawURLEncoding.EncodeToString(data)
}

// FromProfile собирает ссылку из профиля.
//
// Ссылка на звонок VK и client-id уникальны для каждого получателя, поэтому
// вкладываются только по явному решению пользователя.
//
// Конфигурация WireGuard из профиля в ссылку НЕ попадает: в ней лежит
// приватный ключ владельца, и получатель подключался бы под его личностью.
// Гостю кладут его собственную конфигурацию - см. WithWGConf.
func FromProfile(p profile.Profile, includeVKLink bool, clientID string) Link {
	l := Link{
		Provider:       p.Client.Provider,
		Peer:           p.Client.ServerAddress,
		N:              p.Client.Threads,
		StreamsPerCred: p.Client.StreamsPerCred,
		ClientID:       strings.TrimSpace(clientID),
		DNSServers:     p.Client.CustomDNS,
		ManualCaptcha:  p.Client.ManualCaptcha,
		Name:           p.Name,
	}
	// Дефолтные значения не пишем: ссылка короче, QR плотнее.
	if p.Client.UseUDP {
		l.Transport = profile.TransportUDP
	}
	if p.Opts.TCPMode() {
		l.Mode = profile.ModeTCP
		if p.Opts.KCP != profile.DefaultKCP() {
			k := KCP(p.Opts.KCP)
			l.KCP = &k
		}
	}
	if p.Opts.ObfEnabled() && profile.ValidObfKey(p.Opts.ObfKey) {
		l.Obf, l.Key = p.Opts.ObfProfile, p.Opts.ObfKey
	}
	if p.Client.LocalPort != profile.DefaultListen {
		l.Listen = p.Client.LocalPort
	}
	if p.Client.DNSMode != profile.DNSAuto {
		l.DNSMode = p.Client.DNSMode
	}
	if includeVKLink {
		l.VKLink = p.Client.VKLink
	}
	return l
}

// WithWGConf вкладывает в ссылку конфигурацию WireGuard получателя.
// Вызывать её можно только с конфигурацией, выданной сервером именно этому
// гостю, а не с конфигурацией владельца.
func (l Link) WithWGConf(conf string) Link {
	l.WGConf = NormalizeWGConf(conf)
	return l
}

// ToProfile создаёт профиль из ссылки. Поля, которых в ссылке нет,
// остаются дефолтными.
func (l Link) ToProfile() profile.Profile {
	name := strings.TrimSpace(l.Name)
	if name == "" {
		name = l.Peer
	}
	p := profile.New(name)

	p.Client.Provider = l.Provider
	p.Client.ServerAddress = l.Peer
	p.Client.VKLink = l.VKLink
	p.Client.UseUDP = l.Transport == profile.TransportUDP
	p.Client.ManualCaptcha = l.ManualCaptcha
	p.Client.ClientID = l.ClientID
	if l.N > 0 {
		p.Client.Threads = l.N
	}
	if l.StreamsPerCred > 0 {
		p.Client.StreamsPerCred = l.StreamsPerCred
	}
	if l.Listen != "" {
		p.Client.LocalPort = l.Listen
	}
	if l.DNSMode != "" {
		p.Client.DNSMode = l.DNSMode
	}
	if l.DNSServers != "" {
		p.Client.CustomDNS = l.DNSServers
	}
	if l.WGConf != "" {
		p.Client.WireGuardConfig = l.WGConf
		p.Client.TunnelTransport = "wireguard"
	}

	if l.Mode == profile.ModeTCP {
		p.Opts.ProxyMode = profile.ModeTCP
	}
	if l.KCP != nil {
		p.Opts.KCP = profile.KCP(*l.KCP)
	}
	if l.Obf != "" {
		p.Opts.ObfProfile, p.Opts.ObfKey = l.Obf, l.Key
	}
	return p
}

// NormalizeWGConf убирает комментарии, пустые строки и MTU: получатель
// подставляет свой (константа туннеля), а ссылка становится короче.
// Повторяет ShareLinkBuilder.normalizeConf Android-клиента.
func NormalizeWGConf(conf string) string {
	var out []string
	for _, line := range strings.Split(conf, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, ";") {
			continue
		}
		if strings.Contains(l, "=") && strings.HasPrefix(strings.ToUpper(l), "MTU") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
