package backup

import (
	"encoding/json"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// Раскладка профиля в бэкапе повторяет ServerJson.kt Android-клиента:
// имена ключей - контракт совместимости.
type serverJSON struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	SSH          sshJSON         `json:"ssh"`
	Client       json.RawMessage `json:"client"`
	ProxyListen  string          `json:"proxyListen"`
	ProxyConnect string          `json:"proxyConnect"`
	Opts         optsJSON        `json:"opts"`
}

type sshJSON struct {
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

type optsJSON struct {
	ObfProfile  string  `json:"obfProfile"`
	ObfKey      string  `json:"obfKey"`
	ObfTimingMs int     `json:"obfTimingMs"`
	ProxyMode   string  `json:"proxyMode"`
	KCP         kcpJSON `json:"kcp"`
}

type kcpJSON struct {
	NoDelay    int  `json:"noDelay"`
	Interval   int  `json:"interval"`
	Resend     int  `json:"resend"`
	NC         int  `json:"nc"`
	SndWnd     int  `json:"sndWnd"`
	RcvWnd     int  `json:"rcvWnd"`
	MTU        int  `json:"mtu"`
	ACKNoDelay bool `json:"ackNoDelay"`
}

// clientKeys - поля объекта client, которыми управляет наша модель.
// Всё остальное (например splitTunnelApps с именами Android-пакетов)
// переносится через Extra нетронутым.
var clientKeys = map[string]bool{
	"serverAddress": true, "vkLink": true, "provider": true, "threads": true,
	"streamsPerCred": true, "useUdp": true, "manualCaptcha": true, "localPort": true,
	"debugMode": true, "dnsMode": true, "customDns": true, "magicSwitch": true,
	"magicTurn": true, "tunnelTransport": true, "wireGuardConfig": true,
	"wireGuardTunnelName": true, "splitTunnelMode": true, "logsEnabled": true,
	"clientId": true,
	// Ключи, которых в Android нет - они наши собственные.
	"routes": true, "magicPort": true, "splitTunnelSubnets": true, "subUrl": true,
}

func encodeServers(list []profile.Profile) []serverJSON {
	out := make([]serverJSON, 0, len(list))
	for _, p := range list {
		out = append(out, serverJSON{
			ID:   p.ID,
			Name: p.Name,
			SSH: sshJSON{
				IP: p.SSH.IP, Port: p.SSH.Port, Username: p.SSH.Username,
				Password: p.SSH.Password, AuthType: p.SSH.AuthType, SSHKey: p.SSH.SSHKey,
				HostFingerprint: p.SSH.HostFingerprint, RootMode: p.SSH.RootMode,
				SudoPassword: p.SSH.SudoPassword,
			},
			Client:       encodeClient(p),
			ProxyListen:  p.ProxyListen,
			ProxyConnect: p.ProxyConnect,
			Opts: optsJSON{
				ObfProfile: p.Opts.ObfProfile, ObfKey: p.Opts.ObfKey,
				ObfTimingMs: p.Opts.ObfTimingMs, ProxyMode: p.Opts.ProxyMode,
				KCP: kcpJSON(p.Opts.KCP),
			},
		})
	}
	return out
}

// encodeClient пишет известные поля поверх сохранённых чужих: так бэкап,
// пришедший из Android, вернётся туда без потерь.
func encodeClient(p profile.Profile) json.RawMessage {
	obj := map[string]any{}
	for k, v := range p.Extra {
		var decoded any
		if json.Unmarshal(v, &decoded) == nil {
			obj[k] = decoded
		}
	}
	c := p.Client
	obj["serverAddress"] = c.ServerAddress
	obj["vkLink"] = c.VKLink
	obj["provider"] = c.Provider
	obj["threads"] = c.Threads
	obj["streamsPerCred"] = c.StreamsPerCred
	obj["useUdp"] = c.UseUDP
	obj["manualCaptcha"] = c.ManualCaptcha
	obj["localPort"] = c.LocalPort
	obj["debugMode"] = c.DebugMode
	obj["dnsMode"] = c.DNSMode
	obj["customDns"] = c.CustomDNS
	obj["magicSwitch"] = c.MagicSwitch
	obj["magicTurn"] = c.MagicTurn
	obj["tunnelTransport"] = c.TunnelTransport
	obj["wireGuardConfig"] = c.WireGuardConfig
	obj["wireGuardTunnelName"] = c.WireGuardTunnelName
	obj["splitTunnelMode"] = c.SplitTunnelMode
	obj["logsEnabled"] = c.LogsEnabled
	obj["clientId"] = c.ClientID
	// Поля, которых в Android-клиенте нет: он игнорирует незнакомые ключи.
	obj["routes"] = c.Routes
	obj["magicPort"] = c.MagicPort
	obj["splitTunnelSubnets"] = c.SplitTunnelSubnets
	if p.SubURL != "" {
		obj["subUrl"] = p.SubURL
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}

func decodeServers(raw []json.RawMessage) []profile.Profile {
	out := make([]profile.Profile, 0, len(raw))
	for _, item := range raw {
		var s serverJSON
		if json.Unmarshal(item, &s) != nil {
			continue
		}
		p := profile.New(s.Name)
		if s.ID != "" {
			p.ID = s.ID
		}
		p.ProxyListen = orDefault(s.ProxyListen, p.ProxyListen)
		p.ProxyConnect = orDefault(s.ProxyConnect, p.ProxyConnect)
		p.SSH = profile.SSH{
			IP: s.SSH.IP, Port: orDefaultInt(s.SSH.Port, 22),
			Username: orDefault(s.SSH.Username, "root"), Password: s.SSH.Password,
			AuthType: orDefault(s.SSH.AuthType, profile.AuthPassword), SSHKey: s.SSH.SSHKey,
			HostFingerprint: s.SSH.HostFingerprint,
			RootMode:        orDefault(s.SSH.RootMode, profile.RootDirect),
			SudoPassword:    s.SSH.SudoPassword,
		}
		p.Opts = profile.Opts{
			ObfProfile:  orDefault(s.Opts.ObfProfile, profile.ObfNone),
			ObfKey:      s.Opts.ObfKey,
			ObfTimingMs: clamp(s.Opts.ObfTimingMs, 0, profile.ObfTimingMax),
			ProxyMode:   orDefault(s.Opts.ProxyMode, profile.ModeUDP),
			KCP:         profile.KCP(s.Opts.KCP),
		}
		if !p.Opts.KCP.Valid() {
			p.Opts.KCP = profile.DefaultKCP()
		}
		decodeClient(&p, s.Client)
		out = append(out, p)
	}
	return out
}

func decodeClient(p *profile.Profile, raw json.RawMessage) {
	var obj map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &obj) != nil {
		return
	}

	get := func(key string, dst any) {
		if v, ok := obj[key]; ok {
			_ = json.Unmarshal(v, dst)
		}
	}
	c := &p.Client
	get("serverAddress", &c.ServerAddress)
	get("vkLink", &c.VKLink)
	get("provider", &c.Provider)
	get("threads", &c.Threads)
	get("streamsPerCred", &c.StreamsPerCred)
	get("useUdp", &c.UseUDP)
	get("manualCaptcha", &c.ManualCaptcha)
	get("localPort", &c.LocalPort)
	get("debugMode", &c.DebugMode)
	get("dnsMode", &c.DNSMode)
	get("customDns", &c.CustomDNS)
	get("magicSwitch", &c.MagicSwitch)
	get("magicTurn", &c.MagicTurn)
	get("magicPort", &c.MagicPort)
	get("tunnelTransport", &c.TunnelTransport)
	get("wireGuardConfig", &c.WireGuardConfig)
	get("wireGuardTunnelName", &c.WireGuardTunnelName)
	get("splitTunnelMode", &c.SplitTunnelMode)
	get("splitTunnelSubnets", &c.SplitTunnelSubnets)
	get("logsEnabled", &c.LogsEnabled)
	get("clientId", &c.ClientID)
	get("routes", &c.Routes)
	get("subUrl", &p.SubURL)

	// Всё, чего наша модель не знает, сохраняем как есть.
	extra := map[string]json.RawMessage{}
	for k, v := range obj {
		if !clientKeys[k] {
			extra[k] = v
		}
	}
	if len(extra) > 0 {
		p.Extra = extra
	}

	// Значения из чужого бэкапа приводим к тому, что понимает ядро.
	if c.Provider == "" {
		c.Provider = profile.ProviderVK
	}
	if c.Threads <= 0 {
		c.Threads = profile.DefaultN
	}
	if c.StreamsPerCred <= 0 {
		c.StreamsPerCred = profile.DefaultStreamsPerCred
	}
	if c.LocalPort == "" {
		c.LocalPort = profile.DefaultListen
	}
	if c.DNSMode == "" {
		c.DNSMode = profile.DNSAuto
	}
	// Android-клиент выставляет platform mobile; на Windows профиль десктопный.
	c.Platform = profile.PlatformDesktop
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func orDefaultInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func clamp(v, lo, hi int) int {
	switch {
	case v < lo:
		return lo
	case v > hi:
		return hi
	default:
		return v
	}
}
