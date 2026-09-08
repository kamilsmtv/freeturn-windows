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
const ScriptURL = "https://raw.githubusercontent.com/samosvalishe/free-turn-proxy/master/scripts/install.sh"

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

// InstallOptions - параметры установки и запуска ядра на сервере.
type InstallOptions struct {
	// Listen - адрес, который будет слушать сервер (0.0.0.0:56000).
	Listen string
	// Connect - локальный бэкенд: WireGuard или Xray.
	Connect string
	Mode    string
	// ObfProfile и ObfKey обязаны совпасть с клиентскими.
	ObfProfile  string
	ObfKey      string
	ObfTimingMs int
	// ClientID владельца попадает в allowlist сервера.
	ClientID string
	// WithWGPkg разрешает поставить пакет WireGuard, если его нет.
	WithWGPkg bool
	KCP       profile.KCP
	KCPCustom bool
}

// Install ставит ядро на сервер.
func (c *Client) Install(ctx context.Context, ssh profile.SSH, o InstallOptions) (InstallData, Response, error) {
	args := []string{"install"}
	if o.WithWGPkg {
		args = append(args, "--with-wg-pkg")
	}
	resp, err := c.Run(ctx, ssh, args...)
	if err != nil {
		return InstallData{}, resp, err
	}
	var data InstallData
	if resp.OK() {
		err = resp.Decode(&data)
	}
	return data, resp, err
}

// WGSetup поднимает на сервере интерфейс WireGuard и возвращает конфиг клиента.
func (c *Client) WGSetup(ctx context.Context, ssh profile.SSH, port int, endpoint string) (WGSetupData, Response, error) {
	resp, err := c.Run(ctx, ssh, "wg-setup",
		"--port="+strconv.Itoa(port), "--endpoint="+endpoint)
	if err != nil {
		return WGSetupData{}, resp, err
	}
	var data WGSetupData
	if resp.OK() {
		err = resp.Decode(&data)
	}
	return data, resp, err
}

// Start запускает серверную часть с нужными параметрами.
func (c *Client) Start(ctx context.Context, ssh profile.SSH, o InstallOptions) (Response, error) {
	return c.Run(ctx, ssh, StartArgs(o)...)
}

// StartArgs собирает аргументы подкоманды start.
//
// Имена ключей - контракт с parse_rpc_args в install.sh; --listen и
// --connect обязательны, остальное скрипт подставит сам.
func StartArgs(o InstallOptions) []string {
	args := []string{"start", "--listen=" + o.Listen, "--connect=" + o.Connect}

	if o.Mode != "" {
		args = append(args, "--mode="+o.Mode)
	}
	if o.ObfProfile != "" && o.ObfProfile != profile.ObfNone {
		args = append(args, "--obf-profile="+o.ObfProfile, "--obf-key="+o.ObfKey)
		if o.ObfTimingMs > 0 {
			args = append(args, "--obf-timing="+strconv.Itoa(o.ObfTimingMs)+"ms")
		}
	}
	if o.ClientID != "" {
		args = append(args, "--client-id="+o.ClientID)
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
	return DecodeBase64(data.LogB64), resp, nil
}

// ShareInfo возвращает параметры, которые сервер раздаёт клиентам.
func (c *Client) ShareInfo(ctx context.Context, ssh profile.SSH) (ShareInfoData, Response, error) {
	resp, err := c.Run(ctx, ssh, "share-info")
	if err != nil {
		return ShareInfoData{}, resp, err
	}
	var data ShareInfoData
	if resp.OK() {
		err = resp.Decode(&data)
	}
	return data, resp, err
}

// PeerOptions - параметры нового пира WireGuard.
type PeerOptions struct {
	// NameB64 - имя пира в base64; install.sh требует его обязательно.
	NameB64 string
	// Endpoint попадает в конфиг клиента. Туда же смотрит и Android-клиент:
	// это локальный сокет ядра, а не адрес VPS.
	Endpoint string
	// ClientID заодно добавляется в allowlist сервера.
	ClientID string
	// DNS попадает в конфиг клиента; пусто - сервер подставит свой дефолт.
	DNS string
}

// PeerAdd заводит нового пира WireGuard и возвращает готовый конфиг клиента.
//
// Ключи генерирует сервер: он же выделяет адрес, дописывает [Peer] в свою
// конфигурацию и вкладывает в ответ свой публичный ключ и параметры
// AmneziaWG. Обмениваться ключами вручную не нужно.
func (c *Client) PeerAdd(ctx context.Context, ssh profile.SSH, o PeerOptions) (PeerAddData, Response, error) {
	args := PeerAddArgs(o)
	resp, err := c.Run(ctx, ssh, args...)
	if err != nil {
		return PeerAddData{}, resp, err
	}
	var data PeerAddData
	if resp.OK() {
		err = resp.Decode(&data)
	}
	return data, resp, err
}

// PeerAddArgs собирает аргументы подкоманды peer-add.
func PeerAddArgs(o PeerOptions) []string {
	endpoint := strings.TrimSpace(o.Endpoint)
	if endpoint == "" {
		endpoint = profile.DefaultListen
	}

	// --name-b64 и --endpoint скрипт требует обязательно.
	args := []string{"peer-add", "--name-b64=" + o.NameB64, "--endpoint=" + endpoint}
	if o.ClientID != "" {
		args = append(args, "--client-id="+o.ClientID)
	}
	if dns := strings.TrimSpace(o.DNS); dns != "" {
		args = append(args, "--dns="+dns)
	}
	return args
}

// ClientAdd добавляет client-id в allowlist сервера.
func (c *Client) ClientAdd(ctx context.Context, ssh profile.SSH, clientID, nameB64 string) (Response, error) {
	args := []string{"client-add", "--client-id=" + clientID}
	if nameB64 != "" {
		args = append(args, "--name-b64="+nameB64)
	}
	return c.Run(ctx, ssh, args...)
}

// Uninstall удаляет ядро с сервера.
func (c *Client) Uninstall(ctx context.Context, ssh profile.SSH, dryRun bool) (Response, error) {
	args := []string{"uninstall"}
	if dryRun {
		args = append(args, "--dry-run")
	}
	return c.Run(ctx, ssh, args...)
}

// wgToolsScript ставит утилиты WireGuard, если на сервере нет ни awg, ни wg,
// ни docker.
//
// Без них install.sh не может сгенерировать ключи в awg_bootstrap и молча
// падает с «unexpected exit 1»: генератор ключей у него - это awg, wg или
// docker, и запасного пути нет. Ключи X25519 у WireGuard и AmneziaWG общие,
// поэтому wireguard-tools достаточно.
const wgToolsScript = `set -u
if command -v awg >/dev/null 2>&1 || command -v wg >/dev/null 2>&1; then
  echo present
  exit 0
fi
if command -v docker >/dev/null 2>&1; then
  # У install.sh docker идёт раньше wg, поэтому образ должен быть на месте:
  # иначе ключи не создаст ни он, ни поставленный рядом wireguard-tools.
  # Запущенный контейнер AmneziaWG подходит сам по себе - он идёт ещё раньше.
  if [ "$(docker inspect -f '{{.State.Running}}' freeturn-awg 2>/dev/null || echo false)" = "true" ]; then
    echo present
    exit 0
  fi
  if docker image inspect ghcr.io/samosvalishe/freeturn-awg:latest >/dev/null 2>&1 \
     || docker pull ghcr.io/samosvalishe/freeturn-awg:latest >/dev/null 2>&1; then
    echo present
    exit 0
  fi
  echo "docker-image-missing" >&2
  exit 4
fi
if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq >/dev/null 2>&1
  apt-get install -y -qq wireguard-tools >/dev/null 2>&1 || apt-get install -y -qq wireguard >/dev/null 2>&1
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y wireguard-tools >/dev/null 2>&1
elif command -v yum >/dev/null 2>&1; then
  yum install -y wireguard-tools >/dev/null 2>&1
elif command -v apk >/dev/null 2>&1; then
  apk add --no-cache wireguard-tools >/dev/null 2>&1
elif command -v pacman >/dev/null 2>&1; then
  pacman -Sy --noconfirm wireguard-tools >/dev/null 2>&1
else
  echo "no-package-manager" >&2
  exit 2
fi
if command -v wg >/dev/null 2>&1; then
  echo installed
  exit 0
fi
echo "install-failed" >&2
exit 3
`

// EnsureWGTools проверяет и при необходимости ставит утилиты WireGuard.
//
// Возвращает true, если пакет пришлось устанавливать.
func (c *Client) EnsureWGTools(ctx context.Context, ssh profile.SSH) (bool, error) {
	c.logf("проверяю утилиты WireGuard на сервере")

	client, err := dial(ctx, ssh)
	if err != nil {
		return false, err
	}
	defer func() { _ = client.Close() }()

	installed, err := classifyWGTools(runShell(ctx, client, ssh, wgToolsScript))
	if installed {
		c.logf("на сервер установлен пакет wireguard-tools")
	}
	return installed, err
}

// classifyWGTools разбирает ответ скрипта проверки утилит WireGuard.
//
// Метки скрипт печатает в stdout, а причины отказа - в stderr, куда до них
// может добавиться и вывод sudo, поэтому ищем вхождение, а не равенство.
func classifyWGTools(out, errText string, err error) (bool, error) {
	stdout := strings.TrimSpace(out)
	stderr := strings.TrimSpace(errText)

	switch {
	case strings.Contains(stdout, "present"):
		return false, nil
	case strings.Contains(stdout, "installed"):
		return true, nil
	case strings.Contains(stderr, "no-package-manager"):
		return false, errors.New("на сервере нет ни awg, ни wg, ни docker, и пакетный менеджер не распознан: " +
			"поставьте wireguard-tools вручную")
	case strings.Contains(stderr, "docker-image-missing"):
		return false, errors.New("на сервере есть docker, но образ AmneziaWG недоступен: " +
			"install.sh создаёт ключи именно через него. Дайте серверу доступ к ghcr.io " +
			"или поставьте пакет wireguard-tools и уберите docker из PATH")
	case strings.Contains(stderr, "install-failed"):
		return false, errors.New("не удалось поставить wireguard-tools: поставьте пакет вручную и повторите")
	}

	reason := stderr
	if err != nil {
		if reason != "" {
			reason += ": "
		}
		reason += err.Error()
	}
	if reason == "" {
		reason = "сервер ничего не ответил"
	}
	return false, errors.New("не удалось проверить утилиты WireGuard на сервере: " + reason)
}
