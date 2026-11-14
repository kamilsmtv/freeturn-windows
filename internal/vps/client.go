package vps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// ScriptURL - install.sh ядра. Скрипт не вкомпилирован: так он всегда
// соответствует текущей версии ядра, а мы не тащим чужой код в свой бинарь.
const ScriptURL = "https://github.com/hackdiaz-dev/free-turn-proxy/releases/latest/download/install.sh"

// Client выполняет команды управления сервером.
type Client struct {
	// CacheDir - куда класть скачанный install.sh.
	CacheDir string
	HTTP     *http.Client
	// Log получает ход выполнения: пригодится для живого журнала в UI.
	Log func(string)

	mu     sync.Mutex
	script string
}

// New создаёт клиента управления сервером.
func New(cacheDir string, log func(string)) *Client {
	return &Client{
		CacheDir: cacheDir,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
		Log:      log,
	}
}

func (c *Client) logf(format string, args ...any) {
	if c.Log != nil {
		c.Log(fmt.Sprintf(format, args...))
	}
}

// Script возвращает install.sh: сначала из памяти, затем из кэша на диске,
// затем из сети. Без сети рабочим остаётся последний скачанный скрипт.
func (c *Client) Script(ctx context.Context) (string, error) {
	c.mu.Lock()
	cached := c.script
	c.mu.Unlock()
	if cached != "" {
		return cached, nil
	}

	path := filepath.Join(c.CacheDir, "install.sh")

	script, err := c.fetchScript(ctx)
	if err != nil {
		if data, rerr := os.ReadFile(path); rerr == nil && len(data) > 0 {
			c.logf("нет связи с GitHub, используется сохранённый install.sh")
			return c.remember(string(data)), nil
		}
		return "", err
	}
	if c.CacheDir != "" {
		_ = os.WriteFile(path, []byte(script), 0o600)
	}
	return c.remember(script), nil
}

func (c *Client) remember(script string) string {
	c.mu.Lock()
	c.script = script
	c.mu.Unlock()
	return script
}

func (c *Client) fetchScript(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ScriptURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "FreeTurn-Windows")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("не удалось скачать install.sh: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("не удалось скачать install.sh: %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if !strings.Contains(string(data), "PROTO_VERSION") {
		return "", errors.New("скачанный install.sh не похож на скрипт управления сервером")
	}
	return string(data), nil
}

// Run выполняет одну подкоманду install.sh на сервере.
func (c *Client) Run(ctx context.Context, ssh profile.SSH, args ...string) (Response, error) {
	script, err := c.Script(ctx)
	if err != nil {
		return Response{}, err
	}

	c.logf("$ install.sh %s", strings.Join(args, " "))

	// Общий предел на команду: без него отказавший сервер держит интерфейс
	// в ожидании бесконечно, и человеку остаётся только закрыть окно.
	ctx, cancel := context.WithTimeout(ctx, commandTimeout(args))
	defer cancel()

	client, err := dial(ctx, ssh)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = client.Close() }()

	out, errText, err := runScript(ctx, client, ssh, script, args)
	if err != nil {
		return Response{}, err
	}

	resp := ParseOutput(out)
	// Скрипт валится через die, а тот пишет причину в stderr: без него
	// остаётся голое "unexpected exit 1".
	resp.Stderr = strings.TrimSpace(errText)
	// Версию протокола сверяем до разбора data: поля там - контракт, и
	// чужая версия означает, что мы поняли бы ответ неправильно. Молчать
	// здесь опаснее всего: ошибка вылезла бы позже и в другом месте.
	if err := checkProto(resp); err != nil {
		c.logf("ошибка: %s", err)
		return errResponse("proto_mismatch", err.Error()), nil
	}
	for _, line := range resp.Logs {
		c.logf("%s", line)
	}
	if !resp.OK() {
		if resp.Stderr != "" {
			c.logf("stderr: %s", resp.Stderr)
		}
		c.logf("ошибка: %s", Explain(resp))
	}
	return resp, nil
}

// Пределы на команду. Установка тянет пакеты и образ docker, поэтому ей
// нужно много; остальное - это чтение и правка файлов.
const (
	defaultTimeout = 3 * time.Minute
	applyTimeout   = 20 * time.Minute
)

// commandTimeout выбирает предел по имени подкоманды.
func commandTimeout(args []string) time.Duration {
	if len(args) == 0 {
		return defaultTimeout
	}
	switch args[0] {
	case "apply", "uninstall":
		return applyTimeout
	default:
		return defaultTimeout
	}
}

// Probe спрашивает у сервера его состояние.
func (c *Client) Probe(ctx context.Context, ssh profile.SSH) (ProbeData, Response, error) {
	resp, err := c.Run(ctx, ssh, "probe")
	if err != nil {
		return ProbeData{}, resp, err
	}
	var data ProbeData
	if resp.OK() {
		err = resp.Decode(&data)
	}
	return data, resp, err
}

// ApplyOptions - параметры установки и запуска сервера.
//
// Одна команда apply делает всё: ставит бинарь, поднимает WireGuard,
// пишет install.conf и запускает службу. Значения, которых нет, скрипт
// берёт из прошлой установки, поэтому передаём только осмысленные.
type ApplyOptions struct {
	// Backend: new - скрипт поднимает свой WireGuard; external - трафик
	// уходит в чужой VPN по адресу Connect.
	Backend string
	// Connect - адрес чужого VPN, только для external.
	Connect string
	// Host - адрес сервера в выдаваемых ссылках; пусто - внешний IP.
	Host string
	// ListenPort - внешний UDP-порт сервера.
	ListenPort int
	// WGPort - порт своего WireGuard; 0 - дефолт скрипта.
	WGPort int
	Mode   string
	// ObfProfile и ObfKey обязаны совпасть с клиентскими.
	ObfProfile string
	ObfKey     string
	// Version - версия ядра на сервере; пусто - последняя.
	Version   string
	KCP       profile.KCP
	KCPCustom bool
}

// Apply ставит, настраивает и запускает сервер одной командой.
func (c *Client) Apply(ctx context.Context, ssh profile.SSH, o ApplyOptions) (ApplyData, Response, error) {
	resp, err := c.Run(ctx, ssh, ApplyArgs(o)...)
	if err != nil {
		return ApplyData{}, resp, err
	}
	var data ApplyData
	if resp.OK() {
		err = resp.Decode(&data)
	}
	return data, resp, err
}

// ApplyArgs собирает аргументы подкоманды apply.
//
// Имена ключей - контракт с parse_opts в install.sh.
func ApplyArgs(o ApplyOptions) []string {
	args := []string{"apply"}

	backend := o.Backend
	if backend == "" {
		backend = BackendNew
	}
	args = append(args, "--backend="+backend)
	if backend == BackendExternal && strings.TrimSpace(o.Connect) != "" {
		args = append(args, "--connect="+strings.TrimSpace(o.Connect))
	}
	if host := strings.TrimSpace(o.Host); host != "" {
		args = append(args, "--host="+host)
	}
	if o.ListenPort > 0 {
		args = append(args, "--listen-port="+strconv.Itoa(o.ListenPort))
	}
	// Свой порт WireGuard осмыслен только для своего же бэкенда.
	if o.WGPort > 0 && backend == BackendNew {
		args = append(args, "--wg-port="+strconv.Itoa(o.WGPort))
	}
	if o.Mode != "" {
		args = append(args, "--mode="+o.Mode)
	}
	if o.ObfProfile != "" {
		args = append(args, "--obf-profile="+o.ObfProfile)
		if o.ObfProfile != profile.ObfNone && o.ObfKey != "" {
			args = append(args, "--obf-key="+o.ObfKey)
		}
	}
	if v := strings.TrimSpace(o.Version); v != "" {
		args = append(args, "--version="+v)
	}
	// Профиль ARQ имеет смысл только в tcp и только если он не дефолтный.
	if o.KCPCustom && o.Mode == profile.ModeTCP {
		k := o.KCP
		args = append(args,
			"--kcp-nodelay="+strconv.Itoa(k.NoDelay),
			"--kcp-interval="+strconv.Itoa(k.Interval),
			"--kcp-resend="+strconv.Itoa(k.Resend),
			"--kcp-nc="+strconv.Itoa(k.NC),
			"--kcp-sndwnd="+strconv.Itoa(k.SndWnd),
			"--kcp-rcvwnd="+strconv.Itoa(k.RcvWnd),
			"--kcp-mtu="+strconv.Itoa(k.MTU),
			"--kcp-acknodelay="+strconv.FormatBool(k.ACKNoDelay),
		)
	}
	return args
}

// Stop останавливает серверную часть.
func (c *Client) Stop(ctx context.Context, ssh profile.SSH) (Response, error) {
	return c.Run(ctx, ssh, "stop")
}

// Restart перезапускает серверную часть, не трогая настройки.
func (c *Client) Restart(ctx context.Context, ssh profile.SSH) (Response, error) {
	return c.Run(ctx, ssh, "restart")
}

// Logs забирает хвост журнала сервера.
func (c *Client) Logs(ctx context.Context, ssh profile.SSH, lines int) (string, Response, error) {
	if lines <= 0 {
		lines = 80
	}
	resp, err := c.Run(ctx, ssh, "logs", "--tail="+strconv.Itoa(lines))
	if err != nil || !resp.OK() {
		return "", resp, err
	}
	var data LogsData
	if err := resp.Decode(&data); err != nil {
		return "", resp, err
	}
	return strings.Join(data.Lines, "\n"), resp, nil
}

// ClientList перечисляет выданные доступы и параметры раздачи.
func (c *Client) ClientList(ctx context.Context, ssh profile.SSH) (ClientListData, Response, error) {
	resp, err := c.Run(ctx, ssh, "client-list")
	if err != nil {
		return ClientListData{}, resp, err
	}
	var data ClientListData
	if resp.OK() {
		err = resp.Decode(&data)
	}
	return data, resp, err
}

// ClientAdd заводит на сервере новый доступ.
//
// Ключи, адрес и Client ID выдаёт сервер, он же собирает готовую ссылку
// freeturn:// - обмениваться чем-либо вручную не нужно.
func (c *Client) ClientAdd(ctx context.Context, ssh profile.SSH, name string) (ClientData, Response, error) {
	return c.clientCmd(ctx, ssh, "client-add", name)
}

// ClientConf забирает конфигурацию уже заведённого доступа.
func (c *Client) ClientConf(ctx context.Context, ssh profile.SSH, name string) (ClientData, Response, error) {
	return c.clientCmd(ctx, ssh, "client-conf", name)
}

func (c *Client) clientCmd(ctx context.Context, ssh profile.SSH, cmd, name string) (ClientData, Response, error) {
	resp, err := c.Run(ctx, ssh, cmd, "--name="+strings.TrimSpace(name))
	if err != nil {
		return ClientData{}, resp, err
	}
	var data ClientData
	if resp.OK() {
		err = resp.Decode(&data)
	}
	return data, resp, err
}

// ClientRemove отзывает доступ.
func (c *Client) ClientRemove(ctx context.Context, ssh profile.SSH, name string) (Response, error) {
	return c.Run(ctx, ssh, "client-remove", "--name="+strings.TrimSpace(name))
}

// Uninstall удаляет с сервера ядро, WireGuard или всё сразу.
func (c *Client) Uninstall(ctx context.Context, ssh profile.SSH, target string, purge bool) (Response, error) {
	if target == "" {
		target = TargetFreeTurn
	}
	args := []string{"uninstall", "--target=" + target}
	if purge {
		args = append(args, "--purge")
	}
	return c.Run(ctx, ssh, args...)
}
