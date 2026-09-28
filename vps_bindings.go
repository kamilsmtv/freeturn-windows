package main

import (
	"errors"
	"strings"

	"github.com/kamilsmtv/freeturn-windows/internal/core"
	"github.com/kamilsmtv/freeturn-windows/internal/link"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/vps"
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
		// Адрес VPS и доступ к нему живут на вкладке «Сервер», а спотыкается
		// об это чаще всего кнопка в профиле - поэтому говорим, куда идти.
		return VPSResult{Error: "не заполнен доступ по SSH: адрес VPS, пользователь и пароль " +
			"задаются на вкладке «Сервер»"}
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
	if msg := vps.TimeoutMessage(err); msg != "" {
		return VPSResult{Error: msg}
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

// VPSApply ставит, настраивает и запускает сервер одной командой.
//
// В протоколе 3 отдельных install, wg-setup и start больше нет: apply
// делает всё сам, поднимает WireGuard и возвращает готовую конфигурацию
// владельца. Её сразу кладём в профиль - отдельно запрашивать нечего.
func (a *App) VPSApply(id string) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		// Пустой ключ - не беда: сервер сгенерирует его сам и вернёт в
		// ответе. А вот испорченный он молча не исправит.
		if p.Opts.ObfEnabled() && p.Opts.ObfKey != "" && !profile.ValidObfKey(p.Opts.ObfKey) {
			return VPSResult{Error: "ключ обфускации в профиле испорчен: нужны 64 hex-символа. " +
				"Очистите поле - сервер создаст ключ сам"}, nil
		}
		// Чужому бэкенду адрес обязателен: без него серверу некуда
		// отдавать расшифрованный трафик, а скрипт ответит невнятно.
		if externalBackend(p) && !core.ValidHostPort(strings.TrimSpace(p.ProxyConnect)) {
			return VPSResult{Error: "заполните поле «Адрес своего VPN на сервере» — " +
				"например 127.0.0.1:443 для 3x-ui или 127.0.0.1:51820 для WireGuard"}, nil
		}

		data, resp, err := a.vps.Apply(a.bg(), p.SSH, a.applyOptions(p))
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}

		// Ключ сервер мог сгенерировать сам - тогда он в ответе, и без него
		// клиент не соберёт рабочее соединение.
		if data.ObfKey != "" {
			p.Opts.ObfKey = data.ObfKey
		}
		if conf := vps.DecodeBase64(data.Owner.ConfB64); conf != "" {
			p.Client.WireGuardConfig = conf
			p.Client.TunnelTransport = "wireguard"
		}
		if data.Owner.ClientID != "" {
			p.Client.ClientID = data.Owner.ClientID
		}
		// Фактические порты знает только сервер: часть из них он выбирает
		// сам (порт WireGuard), а часть подставляет по умолчанию, если мы
		// их не передали. Без этого шага профиль остался бы с адресом, по
		// которому никто не слушает. Ошибку глотаем: сервер уже настроен.
		if probe, presp, perr := a.vps.Probe(a.bg(), p.SSH); perr == nil && presp.OK() {
			cfg := probe.Config
			if connect := backendAddr(cfg); connect != "" {
				p.ProxyConnect = connect
			}
			if cfg.ListenPort > 0 {
				p.ProxyListen = "0.0.0.0:" + itoa(cfg.ListenPort)
				host := strings.TrimSpace(cfg.Host)
				if host == "" {
					host = strings.TrimSpace(p.SSH.IP)
				}
				if host != "" {
					p.Client.ServerAddress = host + ":" + itoa(cfg.ListenPort)
				}
			}
		} else if port := vps.Port(p.ProxyListen); port > 0 && p.SSH.IP != "" {
			p.Client.ServerAddress = p.SSH.IP + ":" + itoa(port)
		}

		saved, err := a.profiles.Save(p)
		if err != nil {
			return VPSResult{}, err
		}
		a.emitProfiles()

		text := "Сервер установлен и запущен, конфигурация владельца сохранена в профиль"
		if externalBackend(p) {
			text = "Сервер установлен и запущен, трафик уходит в ваш VPN на " +
				strings.TrimSpace(p.ProxyConnect) + ". Конфигурацию клиента берите в нём же — " +
				"FreeTurn работает транспортом и своих ключей не выдаёт"
			text += staleTunnelNote(p)
		}
		if data.NeedsRestart {
			text += ". Параметры раздачи изменились - выданные раньше доступы нужно раздать заново"
		}
		return VPSResult{OK: true, Profile: &saved, Text: text}, nil
	})
}

// VPSRestart перезапускает серверную часть, не трогая настройки.
func (a *App) VPSRestart(id string) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		resp, err := a.vps.Restart(a.bg(), p.SSH)
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}
		return VPSResult{OK: true, Text: "Сервер перезапущен"}, nil
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

// applyOptions собирает параметры установки сервера из профиля.
func (a *App) applyOptions(p profile.Profile) vps.ApplyOptions {
	o := vps.ApplyOptions{
		Backend:    vps.BackendNew,
		Host:       strings.TrimSpace(p.SSH.IP),
		ListenPort: vps.Port(p.ProxyListen),
		Mode:       p.Opts.ProxyMode,
		ObfProfile: p.Opts.ObfProfile,
		ObfKey:     p.Opts.ObfKey,
		KCP:        p.Opts.KCP,
		KCPCustom:  p.Opts.KCP != profile.DefaultKCP(),
	}
	if externalBackend(p) {
		o.Backend = vps.BackendExternal
		o.Connect = strings.TrimSpace(p.ProxyConnect)
	}
	return o
}

// externalBackend сообщает, что за релеем стоит чужой VPN.
//
// Выбор хранится в профиле, но режим проброса tcp решает за него: свой
// WireGuard скрипт поднимает только для udp, и это его правило, не наше.
func externalBackend(p profile.Profile) bool {
	return p.ServerBackend == profile.ServerBackendExternal || p.Opts.TCPMode()
}

// VPSImportParams переносит в профиль параметры, с которыми сервер реально
// работает: адрес и порт, режим, профиль и ключ обфускации.
//
// В протоколе 3 всё это отдаёт probe: раньше приходилось читать install.conf
// и run.args своим скриптом, потому что share-info умалчивал ключ.
func (a *App) VPSImportParams(id string) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		data, resp, err := a.vps.Probe(a.bg(), p.SSH)
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}
		if !data.Installed {
			return VPSResult{Error: "на сервере ещё нет ядра - сначала установите его"}, nil
		}

		cfg := data.Config
		var notes []string
		if cfg.ListenPort > 0 {
			host := strings.TrimSpace(cfg.Host)
			if host == "" {
				host = p.SSH.IP
			}
			addr := host + ":" + itoa(cfg.ListenPort)
			if p.Client.ServerAddress != addr {
				notes = append(notes, "адрес сервера: "+addr)
			}
			p.Client.ServerAddress = addr
			p.ProxyListen = "0.0.0.0:" + itoa(cfg.ListenPort)
		}
		if connect := backendAddr(cfg); connect != "" {
			p.ProxyConnect = connect
		}
		// Выбор бэкенда тоже переносим: иначе профиль считал бы, что за
		// релеем наш WireGuard, и следующая установка подняла бы его на
		// сервере, где владелец нарочно поставил свой VPN.
		if cfg.Backend != "" {
			p.ServerBackend = cfg.Backend
			if cfg.Backend == profile.ServerBackendExternal {
				notes = append(notes, "за релеем ваш VPN на "+p.ProxyConnect)
			}
		}
		if cfg.Mode != "" {
			p.Opts.ProxyMode = cfg.Mode
		}
		if cfg.ObfProfile != "" {
			p.Opts.ObfProfile = cfg.ObfProfile
			notes = append(notes, "обфускация: "+cfg.ObfProfile)
		}
		if cfg.ObfKey != "" {
			p.Opts.ObfKey = cfg.ObfKey
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

		text := "Параметры сервера перенесены в профиль"
		if len(notes) > 0 {
			text += " — " + strings.Join(notes, ", ")
		}
		return VPSResult{OK: true, Profile: &saved, Text: text}, nil
	})
}

// VPSFetchWGConfig забирает с сервера конфигурацию владельца.
//
// Сервер выдаёт её сам при установке, но профиль мог приехать с другой
// машины или конфигурация могла устареть после смены параметров.
func (a *App) VPSFetchWGConfig(id string) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		data, resp, err := a.vps.ClientConf(a.bg(), p.SSH, vps.OwnerName)
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}

		conf := vps.DecodeBase64(data.ConfB64)
		if conf == "" {
			if externalBackend(p) {
				return VPSResult{Error: "сервер отдаёт трафик в ваш VPN и своих ключей не выдаёт: " +
					"конфигурацию клиента возьмите в нём (например в панели 3x-ui)"}, nil
			}
			return VPSResult{Error: "сервер не вернул конфигурацию владельца"}, nil
		}

		p.Client.WireGuardConfig = conf
		p.Client.TunnelTransport = "wireguard"
		if data.Client.ClientID != "" {
			p.Client.ClientID = data.Client.ClientID
		}
		saved, err := a.profiles.Save(p)
		if err != nil {
			return VPSResult{}, err
		}
		a.emitProfiles()
		return VPSResult{
			OK:      true,
			Profile: &saved,
			Text:    "Сервер выдал конфигурацию: адрес " + data.Client.IP + ", ключи и параметры внутри",
		}, nil
	})
}

// VPSRemoveWireGuard убирает с сервера поднятый нами интерфейс.
//
// Нужен при переходе на свой VPN: скрипт не даёт сменить бэкенд, пока его
// WireGuard на месте. Действие необратимое - выданные им конфигурации
// клиентов перестанут работать, поэтому спрашивать должен интерфейс.
func (a *App) VPSRemoveWireGuard(id string) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		resp, err := a.vps.Uninstall(a.bg(), p.SSH, vps.TargetWG, false)
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}
		return VPSResult{OK: true, Text: "WireGuard, поднятый нами, удалён с сервера" + staleTunnelNote(p)}, nil
	})
}

// VPSClients перечисляет выданные сервером доступы.
func (a *App) VPSClients(id string) (vps.ClientListData, error) {
	p, err := a.profiles.Get(id)
	if err != nil {
		return vps.ClientListData{}, err
	}
	data, resp, err := a.vps.ClientList(a.bg(), p.SSH)
	if err != nil {
		return vps.ClientListData{}, err
	}
	if !resp.OK() {
		return vps.ClientListData{}, errors.New(vps.Explain(resp))
	}
	return data, nil
}

// VPSAddPeer заводит на сервере доступ для гостя и возвращает готовую
// ссылку.
//
// Ссылку собирает сам сервер: в ней его адрес, режим, обфускация и свежий
// Client ID гостя. Нам остаётся вложить конфигурацию WireGuard, которую он
// выдал этому же гостю.
func (a *App) VPSAddPeer(id, name string) (string, error) {
	p, err := a.profiles.Get(id)
	if err != nil {
		return "", err
	}
	// Имя уходит в имена файлов на сервере, и кириллицу он не примет.
	if !vps.ValidClientName(strings.TrimSpace(name)) {
		return "", errors.New("имя доступа сервер принимает только латиницей: буквы, цифры, " +
			"точка, дефис и подчёркивание, до 32 символов")
	}

	data, resp, err := a.vps.ClientAdd(a.bg(), p.SSH, name)
	if err != nil {
		return "", err
	}
	if !resp.OK() {
		return "", errors.New(vps.Explain(resp))
	}
	return guestLink(p, name, data), nil
}

// VPSPeerLink забирает ссылку уже заведённого гостя.
//
// Без неё доступ, выданный однажды, оставался бы недостижимым: завести
// второй раз сервер не даёт («уже есть»), а показать первый было нечем.
func (a *App) VPSPeerLink(id, name string) (string, error) {
	p, err := a.profiles.Get(id)
	if err != nil {
		return "", err
	}

	data, resp, err := a.vps.ClientConf(a.bg(), p.SSH, name)
	if err != nil {
		return "", err
	}
	if !resp.OK() {
		return "", errors.New(vps.Explain(resp))
	}
	return guestLink(p, name, data), nil
}

// guestLink достаёт ссылку из ответа сервера, а если тот её не дал -
// собирает свою.
func guestLink(p profile.Profile, name string, data vps.ClientData) string {
	conf := vps.DecodeBase64(data.ConfB64)
	if raw := strings.TrimSpace(data.Link); raw != "" {
		l, lerr := link.Parse(raw)
		switch {
		case lerr != nil:
			// Ссылку не поняли - соберём свою ниже.
		case strings.TrimSpace(l.WGConf) != "":
			// Сервер вложил конфигурацию сам: отдаём его ссылку как есть.
			// Пересобирать её нельзя - поля, которых нет в нашей модели,
			// при этом потерялись бы, а сервер новее клиента почти всегда.
			return raw
		default:
			return l.WithWGConf(conf).Encode()
		}
	}

	// Сервер ссылку не дал или она непонятная - собираем свою, как раньше.
	guest := p
	guest.Name = name
	return link.FromProfile(guest, false, data.Client.ClientID).WithWGConf(conf).Encode()
}

// VPSRemovePeer отзывает доступ гостя.
func (a *App) VPSRemovePeer(id, name string) VPSResult {
	return a.vpsCall(id, func(p profile.Profile) (VPSResult, error) {
		resp, err := a.vps.ClientRemove(a.bg(), p.SSH, name)
		if err != nil {
			return VPSResult{}, err
		}
		if !resp.OK() {
			return VPSResult{Error: vps.Explain(resp)}, nil
		}
		return VPSResult{OK: true, Text: "Доступ «" + name + "» отозван"}, nil
	})
}

// staleTunnelNote предупреждает, что конфигурация в профиле больше не
// подойдёт.
//
// Молчать тут нельзя: профиль остаётся в режиме VPN со старыми ключами,
// подключение будет срываться на рукопожатии, и виноватым будет выглядеть
// приложение, а не сменившийся бэкенд сервера.
func staleTunnelNote(p profile.Profile) string {
	if p.Client.TunnelTransport != "wireguard" || strings.TrimSpace(p.Client.WireGuardConfig) == "" {
		return ""
	}
	return ". Внимание: в профиле осталась прежняя конфигурация WireGuard - " +
		"она больше не подойдёт. Замените её конфигурацией из своего VPN " +
		"или переключите профиль в режим прокси"
}

// backendAddr - адрес, куда сервер отдаёт расшифрованный трафик.
//
// Для чужого VPN он записан в install.conf, а для своего WireGuard поле
// connect остаётся пустым: скрипт держит интерфейс на той же машине и
// помнит только его порт.
func backendAddr(cfg vps.ConfigData) string {
	if addr := strings.TrimSpace(cfg.Connect); addr != "" {
		return addr
	}
	if cfg.Backend == vps.BackendNew && cfg.WGPort > 0 {
		return "127.0.0.1:" + itoa(cfg.WGPort)
	}
	return ""
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
