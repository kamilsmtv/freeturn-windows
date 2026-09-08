package main

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/link"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/tunnel"
	"github.com/kamilsmtv/freeturn-windows/internal/vps"
	"github.com/kamilsmtv/freeturn-windows/internal/wg"
)

// EventVPSLog - строка хода выполнения команды на сервере.
const EventVPSLog = "vps:log"

// VPSResult - результат команды для UI.
type VPSResult struct {
	OK bool `json:"ok"`
	// Fingerprint заполняется, когда ключ сервера ещё не подтверждён.
	Fingerprint string `json:"fingerprint"`
	Error       string `json:"error"`
	// Probe заполняется командой probe.
	Probe *vps.ProbeData `json:"probe,omitempty"`
	// Text - произвольный текстовый результат (журнал сервера).
	Text string `json:"text,omitempty"`
	// Profile - обновлённый профиль, если команда изменила его параметры.
	Profile *profile.Profile `json:"profile,omitempty"`
}

// vpsCall выполняет операцию с сервером, приводя ошибки к виду для UI.
func (a *App) vpsCall(id string, fn func(p profile.Profile) (VPSResult, error)) VPSResult {
	p, err := a.profiles.Get(id)
	if err != nil {
		return VPSResult{Error: err.Error()}
	}
	if strings.TrimSpace(p.SSH.IP) == "" {
		return VPSResult{Error: "не задан адрес сервера в разделе SSH"}
	}

	res, err := fn(p)
	if err == nil {
		return res
	}

	// Первое подключение: показываем отпечаток, чтобы пользователь его сверил.
	var unknown *vps.ErrUnknownHost
	if errors.As(err, &unknown) {
		return VPSResult{Fingerprint: unknown.Fingerprint, Error: unknown.Error()}
	}
	return VPSResult{Error: err.Error()}
}

// TrustHostKey сохраняет отпечаток ключа сервера в профиле.
func (a *App) TrustHostKey(id, fingerprint string) error {
	p, err := a.profiles.Get(id)
	if err != nil {
		return err
	}
	p.SSH.HostFingerprint = fingerprint
	_, err = a.profiles.Save(p)
	a.emitProfiles()
	return err
}

// VPSProbe спрашивает у сервера его состояние.
func (a *App) VPSProbe(id string) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		data, resp, err := a.vps.Probe(a.bg(), p.SSH)
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}
		return VPSResult{OK: true, Probe: &data}, nil
	})
}

// VPSInstall ставит ядро на сервер.
func (a *App) VPSInstall(id string, withWGPkg bool) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		data, resp, err := a.vps.Install(a.bg(), p.SSH, vps.InstallOptions{WithWGPkg: withWGPkg})
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}
		return VPSResult{OK: true, Text: "Ядро " + data.Version + " (" + data.Stage + "), запуск через " + data.Runtime}, nil
	})
}

// VPSStart запускает серверную часть с параметрами профиля.
func (a *App) VPSStart(id string) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		// Ключ обфускации и client-id обязаны совпасть с клиентскими,
		// поэтому берём их из того же профиля.
		if p.Opts.ObfEnabled() && !profile.ValidObfKey(p.Opts.ObfKey) {
			return VPSResult{Error: "перед запуском задайте ключ обфускации в профиле"}, nil
		}

		resp, err := a.vps.Start(a.bg(), p.SSH, a.startOptions(p))
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}
		return VPSResult{OK: true, Text: "Сервер запущен"}, nil
	})
}

// VPSStop останавливает серверную часть.
func (a *App) VPSStop(id string) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		resp, err := a.vps.Stop(a.bg(), p.SSH)
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}
		return VPSResult{OK: true, Text: "Сервер остановлен"}, nil
	})
}

// VPSLogs забирает хвост журнала сервера.
func (a *App) VPSLogs(id string, lines int) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		text, resp, err := a.vps.Logs(a.bg(), p.SSH, lines)
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}
		return VPSResult{OK: true, Text: text}, nil
	})
}

// VPSSetupWireGuard поднимает на сервере WireGuard и кладёт конфиг в профиль.
func (a *App) VPSSetupWireGuard(id string) VPSResult {
	return a.vpsCall(id, a.setupWireGuard)
}

// setupWireGuard поднимает на сервере интерфейс AmneziaWG, забирает конфиг
// его владельца и перезапускает ядро на выданный порт.
//
// Порядок повторяет мастер Android-клиента: wg-setup возвращает фактический
// порт интерфейса, и сервер должен слать трафик именно туда (-connect),
// иначе релей будет стучаться в пустоту.
func (a *App) setupWireGuard(p profile.Profile) (VPSResult, error) {
	// На сервере уже может работать обычный WireGuard: wg-setup поднимает
	// рядом с ним вторую реализацию (AmneziaWG) и часто срывается, а смысла
	// в этом нет - достаточно добавить себя пиром в работающий интерфейс.
	//
	// Ошибку share-info глотаем намеренно: на сервере без установленного
	// ядра команды ещё нет, и это как раз случай для wg-setup.
	if info, resp, err := a.vps.ShareInfo(a.bg(), p.SSH); err == nil && resp.OK() && !info.CanAddPeers() {
		if probe, presp, perr := a.vps.Probe(a.bg(), p.SSH); perr == nil && presp.OK() && probe.WG.Present {
			return a.adoptWireGuard(p, info)
		}
	}
	return a.installWireGuard(p)
}

// installWireGuard разворачивает на сервере AmneziaWG через wg-setup.
//
// Вызывать его напрямую стоит только там, где уже известно, что своего
// WireGuard на сервере нет: сам он этого не проверяет.
func (a *App) installWireGuard(p profile.Profile) (VPSResult, error) {
	port := a.backendPortFor(p)

	// install.sh генерирует ключи интерфейса через awg, wg или docker;
	// если их нет, он падает без внятной причины - доставляем заранее.
	//
	// Срок ограничиваем: apt-get умеет ждать чужой dpkg-блокировки сколько
	// угодно, а команда без конца выглядит как зависшее приложение.
	toolsCtx, cancel := context.WithTimeout(a.bg(), 5*time.Minute)
	defer cancel()
	if _, err := a.vps.EnsureWGTools(toolsCtx, p.SSH); err != nil {
		return VPSResult{Error: err.Error()}, nil
	}

	data, resp, err := a.vps.WGSetup(a.bg(), p.SSH, port, endpointFor(p))
	if err != nil {
		return VPSResult{}, err
	}
	if !resp.OK() {
		return VPSResult{Error: vps.Explain(resp)}, nil
	}

	if data.WG.Port > 0 {
		port = data.WG.Port
	}

	// Конфигурацию профиля не затираем: если сервер её не вернул, старая
	// пригодится хотя бы как есть, а новую сейчас запросим отдельно.
	conf := vps.DecodeBase64(data.ClientConfB64)
	if conf != "" {
		p.Client.WireGuardConfig = conf
	}
	p.Client.TunnelTransport = "wireguard"
	p.ProxyConnect = "127.0.0.1:" + itoa(port)
	saved, err := a.profiles.Save(p)
	if err != nil {
		return VPSResult{}, err
	}
	a.emitProfiles()

	text := "WireGuard поднят на порту " + itoa(port)
	if conf == "" {
		// wg-setup только поднимает интерфейс: конфигурацию клиента
		// install.sh отдаёт исключительно в peer-add, поэтому сразу
		// просим себе пира - иначе профиль остался бы без ключей.
		res, perr := a.peerConfig(saved)
		if perr != nil {
			return VPSResult{}, perr
		}
		if !res.OK || res.Profile == nil {
			return VPSResult{Error: "интерфейс поднят на порту " + itoa(port) +
				", но получить конфигурацию клиента не удалось: " + res.Error}, nil
		}
		saved = *res.Profile
		text += ", " + res.Text
	} else {
		text += ", конфигурация клиента сохранена в профиль"
	}
	// Ядро на сервере должно отдавать трафик в этот интерфейс.
	if restart, rerr := a.vps.Start(a.bg(), saved.SSH, a.startOptions(saved)); rerr != nil {
		text += ". Перезапустить сервер не удалось: " + rerr.Error()
	} else if !restart.OK() {
		text += ". Перезапустите сервер вручную: " + vps.Explain(restart)
	} else {
		text += ", сервер перезапущен на этот порт"
	}
	return VPSResult{OK: true, Profile: &saved, Text: text}, nil
}

// adoptWireGuard добавляет клиента в WireGuard, который уже работает на
// сервере, и собирает по его данным конфигурацию.
//
// install.sh заводит пиров только для AmneziaWG, а обычный WireGuard он
// лишь видит. Разворачивать поверх рабочего сервера вторую реализацию -
// лишний риск, поэтому добавляем себя в существующий интерфейс.
func (a *App) adoptWireGuard(p profile.Profile, info vps.ShareInfoData) (VPSResult, error) {
	keys, err := tunnel.GenerateKeyPair()
	if err != nil {
		return VPSResult{}, err
	}

	// Ошибку отдаём наверх: vpsCall сам приведёт её к виду для интерфейса.
	peer, err := a.vps.AdoptWireGuard(a.bg(), p.SSH, keys.Public, "")
	if err != nil {
		return VPSResult{}, err
	}

	// DNS в конфигурации туннеля не заполняем: поле «Свои DNS» профиля
	// относится к ядру и указывает на резолвер физической сети, внутри
	// туннеля он недоступен. Туннель подставит свой резолвер сам.
	conf := wg.ClientConf(wg.ClientConfOptions{
		PrivateKey:      keys.Private,
		Address:         peer.ClientIP + "/32",
		ServerPublicKey: peer.ServerPublicKey,
		Endpoint:        endpointFor(p),
	})

	p.Client.WireGuardConfig = conf
	p.Client.TunnelTransport = "wireguard"
	if peer.Port > 0 {
		p.ProxyConnect = "127.0.0.1:" + itoa(peer.Port)
	}
	// Параметры обфускации сервера обязаны совпасть с клиентскими; берём
	// их из аргументов запуска, а не из share-info: там ключа может не быть.
	if params, perr := a.vps.ServerParams(a.bg(), p.SSH); perr == nil {
		applyServerParams(&p, params)
	} else {
		applyShareInfo(&p, info)
	}

	saved, err := a.profiles.Save(p)
	if err != nil {
		return VPSResult{}, err
	}
	a.emitProfiles()
	text := "Клиент добавлен в WireGuard сервера (интерфейс " + peer.Interface +
		", адрес " + peer.ClientIP + "), конфигурация сохранена в профиль"
	if !peer.Saved {
		text += ". Внимание: записать пира в конфигурацию сервера не удалось - " +
			"он проживёт до перезагрузки интерфейса"
	}
	return VPSResult{OK: true, Profile: &saved, Text: text}, nil
}

// applyServerParams переносит в профиль фактические параметры сервера.
func applyServerParams(p *profile.Profile, params vps.ServerParams) {
	if params.Mode != "" {
		p.Opts.ProxyMode = params.Mode
	}
	if params.ObfProfile != "" {
		p.Opts.ObfProfile = params.ObfProfile
	}
	if params.ObfKey != "" {
		p.Opts.ObfKey = params.ObfKey
	}
	if port := vps.Port(params.Listen); port > 0 && p.SSH.IP != "" {
		p.Client.ServerAddress = p.SSH.IP + ":" + itoa(port)
	}
}

// applyShareInfo переносит в профиль режим и обфускацию сервера.
func applyShareInfo(p *profile.Profile, info vps.ShareInfoData) {
	if info.Mode != "" {
		p.Opts.ProxyMode = info.Mode
	}
	if info.ObfProfile != "" {
		p.Opts.ObfProfile = info.ObfProfile
	}
	if info.ObfKey != "" {
		p.Opts.ObfKey = info.ObfKey
	}
}

// backendPortFor выбирает порт локального бэкенда: сначала тот, что уже
// поднят на сервере, затем указанный в профиле, затем дефолт WireGuard.
func (a *App) backendPortFor(p profile.Profile) int {
	if probe, resp, err := a.vps.Probe(a.bg(), p.SSH); err == nil && resp.OK() && probe.WG.Port > 0 {
		return probe.WG.Port
	}
	if port := backendPort(p.ProxyConnect); port > 0 {
		return port
	}
	return 51820
}

// startOptions собирает параметры запуска серверной части из профиля.
func (a *App) startOptions(p profile.Profile) vps.InstallOptions {
	clientID := p.Client.ClientID
	if clientID == "" {
		clientID = a.ownClientID()
	}
	return vps.InstallOptions{
		Listen:      p.ProxyListen,
		Connect:     p.ProxyConnect,
		Mode:        p.Opts.ProxyMode,
		ObfProfile:  p.Opts.ObfProfile,
		ObfKey:      p.Opts.ObfKey,
		ObfTimingMs: p.Opts.ObfTimingMs,
		ClientID:    clientID,
		KCP:         p.Opts.KCP,
		KCPCustom:   p.Opts.KCP != profile.DefaultKCP(),
	}
}

// VPSImportShareInfo переносит в профиль параметры, с которыми сервер
// реально работает: адрес и порт, режим, профиль и ключ обфускации.
//
// share-info для этого не хватает: он отдаёт только содержимое install.conf
// и умалчивает ключ, если там его нет, - а именно из-за несовпадения ключа
// сервер рвёт handshake.
func (a *App) VPSImportShareInfo(id string) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		params, err := a.vps.ServerParams(a.bg(), p.SSH)
		if err != nil {
			return VPSResult{}, err
		}

		var notes []string
		if port := vps.Port(params.Listen); port > 0 {
			// Адрес сервера собираем из его же SSH-адреса и порта прослушивания.
			addr := p.SSH.IP + ":" + itoa(port)
			if p.Client.ServerAddress != addr {
				notes = append(notes, "адрес сервера: "+addr)
			}
			p.Client.ServerAddress = addr
		}
		if params.Connect != "" {
			p.ProxyConnect = params.Connect
		}
		if params.Mode != "" {
			p.Opts.ProxyMode = params.Mode
		}
		if params.ObfProfile != "" {
			p.Opts.ObfProfile = params.ObfProfile
			notes = append(notes, "обфускация: "+params.ObfProfile)
		}
		if params.ObfKey != "" {
			p.Opts.ObfKey = params.ObfKey
		}

		// Профиль обфускации без ключа сделает конфигурацию нерабочей,
		// поэтому о такой ситуации говорим прямо.
		if p.Opts.ObfEnabled() && !profile.ValidObfKey(p.Opts.ObfKey) {
			return VPSResult{Error: "сервер работает с обфускацией «" + p.Opts.ObfProfile +
				"», но ключ не сохранён в /opt/free-turn-proxy. Возьмите его на сервере " +
				"(он же в install.conf) и впишите в профиль вручную"}, nil
		}

		saved, err := a.profiles.Save(p)
		if err != nil {
			return VPSResult{}, err
		}
		a.emitProfiles()

		text := "Параметры сервера перенесены в профиль (источник: " + params.Source + ")"
		if len(notes) > 0 {
			text += " — " + strings.Join(notes, ", ")
		}
		return VPSResult{OK: true, Profile: &saved, Text: text}, nil
	})
}

// VPSFetchWGConfig просит сервер завести пира для этого профиля и кладёт
// полученный конфиг в профиль.
//
// Так же поступает Android-клиент: ключи и адрес выдаёт сервер, а вместе с
// ними приходят его публичный ключ и параметры AmneziaWG. Обмениваться
// ключами вручную не нужно.
func (a *App) VPSFetchWGConfig(id string) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		// peer-add дописывает [Peer] в конфигурацию AmneziaWG. Ни
		// probe.wg.present, ни share-info.wg_backend для проверки не годятся:
		// оба истинны и для обычного WireGuard, а нужной конфигурации при
		// этом нет - команда срывается на дозаписи в несуществующий файл.
		// Точный признак один: backend_type = awg.
		info, resp, err := a.vps.ShareInfo(a.bg(), p.SSH)
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}
		if !info.CanAddPeers() {
			// На сервере может уже работать обычный WireGuard - тогда
			// разворачивать AmneziaWG незачем, достаточно добавить себя
			// в него пиром.
			if probe, presp, perr := a.vps.Probe(a.bg(), p.SSH); perr == nil && presp.OK() && probe.WG.Present {
				return a.adoptWireGuard(p, info)
			}
			// Своего WireGuard на сервере нет - разворачиваем AmneziaWG,
			// повторно спрашивать сервер об этом уже незачем.
			return a.installWireGuard(p)
		}
		return a.peerConfig(p)
	})
}

// peerConfig просит сервер завести пира AmneziaWG для этого профиля и
// сохраняет выданную конфигурацию.
//
// Так же поступает Android-клиент: ключи и адрес выдаёт сервер, а вместе с
// ними приходят его публичный ключ и параметры AmneziaWG. Обмениваться
// ключами вручную не нужно.
func (a *App) peerConfig(p profile.Profile) (VPSResult, error) {
	clientID := p.Client.ClientID
	if clientID == "" {
		clientID = a.ownClientID()
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = "windows"
	}

	data, resp, err := a.vps.PeerAdd(a.bg(), p.SSH, vps.PeerOptions{
		NameB64:  base64.StdEncoding.EncodeToString([]byte(name)),
		Endpoint: endpointFor(p),
		ClientID: clientID,
		DNS:      strings.TrimSpace(p.Client.CustomDNS),
	})
	if err != nil {
		return VPSResult{}, err
	}
	if !resp.OK() {
		return VPSResult{Error: vps.Explain(resp)}, nil
	}

	conf := vps.DecodeBase64(data.ClientConfB64)
	if conf == "" {
		return VPSResult{Error: "сервер не вернул конфигурацию клиента"}, nil
	}

	p.Client.WireGuardConfig = conf
	p.Client.TunnelTransport = "wireguard"
	p.Client.ClientID = clientID
	saved, err := a.profiles.Save(p)
	if err != nil {
		return VPSResult{}, err
	}
	a.emitProfiles()
	return VPSResult{
		OK:      true,
		Profile: &saved,
		Text:    "Сервер выдал конфигурацию: адрес " + data.Peer.IP + ", ключи и параметры обфускации внутри",
	}, nil
}

// VPSAddPeer заводит на сервере нового пира WireGuard и возвращает ссылку
// для гостя вместе с его конфигом.
func (a *App) VPSAddPeer(id, name string) (string, error) {
	p, err := a.profiles.Get(id)
	if err != nil {
		return "", err
	}

	nameB64 := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(name)))
	data, resp, err := a.vps.PeerAdd(a.bg(), p.SSH, vps.PeerOptions{
		NameB64:  nameB64,
		Endpoint: endpointFor(p),
		ClientID: profile.NewClientID(),
		DNS:      strings.TrimSpace(p.Client.CustomDNS),
	})
	if err != nil {
		return "", err
	}
	if !resp.OK() {
		return "", errors.New(vps.Explain(resp))
	}

	guest := p
	guest.Name = name
	// Гостю уходит конфигурация, которую сервер выдал именно ему: класть
	// свою нельзя, в ней приватный ключ владельца.
	l := link.FromProfile(guest, false, data.ClientID).
		WithWGConf(vps.DecodeBase64(data.ClientConfB64))
	return l.Encode(), nil
}

// backendPort достаёт порт из адреса host:port.
func backendPort(addr string) int {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return 0
	}
	n := 0
	for _, r := range addr[idx+1:] {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	if n < 1 || n > 65535 {
		return 0
	}
	return n
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
