package vps

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// ServerParams - фактические параметры серверной части.
type ServerParams struct {
	Listen     string `json:"listen"`
	Connect    string `json:"connect"`
	Mode       string `json:"mode"`
	ObfProfile string `json:"obf_profile"`
	ObfKey     string `json:"obf_key"`
	// Source сообщает, откуда взяты значения: аргументы запуска или конфиг.
	Source string `json:"source"`
	Error  string `json:"error"`
}

// paramsScript читает параметры, с которыми сервер реально работает.
//
// share-info отдаёт только то, что записано в install.conf, и умалчивает
// ключ обфускации, если его там нет. Аргументы запуска (run.args) точнее:
// именно с ними работает процесс. Формат файла - по строке на токен,
// парами «-флаг», «значение» (см. _write_args_file в install.sh).
const paramsScript = `
set -eu
PREFIX="${FT_PREFIX:-/opt/free-turn-proxy}"
listen=""; connect=""; mode=""; prof=""; key=""; src=""

if [ -r "$PREFIX/run.args" ]; then
    prev=""
    while IFS= read -r line; do
        case "$prev" in
            -listen)      listen="$line" ;;
            -connect)     connect="$line" ;;
            -mode)        mode="$line" ;;
            -obf-profile) prof="$line" ;;
            -obf-key)     key="$line" ;;
        esac
        prev="$line"
    done < "$PREFIX/run.args"
    [ -n "$listen" ] && src="run.args"
fi

if [ -r "$PREFIX/install.conf" ]; then
    # shellcheck disable=SC1090
    . "$PREFIX/install.conf" 2>/dev/null || true
    [ -z "$listen" ] && [ -n "${LISTEN_PORT:-}" ] && { listen="0.0.0.0:${LISTEN_PORT}"; src="install.conf"; }
    [ -z "$mode" ] && mode="${PROXY_MODE:-}"
    [ -z "$prof" ] && prof="${OBF_PROFILE:-}"
    # Ключ в аргументы не попадает, если профиль выключен, а в конфиге он есть.
    [ -z "$key" ] && key="${OBF_KEY:-}"
    [ -z "$connect" ] && [ -n "${BACKEND_PORT:-}" ] && connect="127.0.0.1:${BACKEND_PORT}"
fi

[ -n "$src" ] || { echo '{"error":"на сервере не найдены параметры запуска: /opt/free-turn-proxy пуст"}'; exit 0; }

printf '{"listen":"%s","connect":"%s","mode":"%s","obf_profile":"%s","obf_key":"%s","source":"%s"}\n' \
    "$listen" "$connect" "$mode" "$prof" "$key" "$src"
`

// ServerParams читает с сервера параметры, с которыми он работает.
func (c *Client) ServerParams(ctx context.Context, ssh profile.SSH) (ServerParams, error) {
	c.logf("$ читаю параметры серверной части")

	client, err := dial(ctx, ssh)
	if err != nil {
		return ServerParams{}, err
	}
	defer func() { _ = client.Close() }()

	out, errText, err := runScript(ctx, client, ssh, paramsScript, nil)
	if err != nil {
		return ServerParams{}, err
	}

	line := lastJSONLine(out)
	if line == "" {
		detail := strings.TrimSpace(errText)
		if detail == "" {
			detail = truncate(strings.TrimSpace(out), 200)
		}
		return ServerParams{}, fmt.Errorf("не удалось прочитать параметры сервера: %s", detail)
	}

	var p ServerParams
	if err := json.Unmarshal([]byte(line), &p); err != nil {
		return ServerParams{}, fmt.Errorf("непонятный ответ сервера: %s", truncate(line, 200))
	}
	if p.Error != "" {
		return ServerParams{}, fmt.Errorf("%s", p.Error)
	}
	c.logf("параметры сервера (%s): listen=%s connect=%s mode=%s obf=%s",
		p.Source, p.Listen, p.Connect, p.Mode, p.ObfProfile)
	return p, nil
}

// Port возвращает порт из адреса вида host:port; 0 - разобрать не удалось.
func Port(addr string) int {
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
