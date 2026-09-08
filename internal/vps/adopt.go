package vps

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// AdoptedPeer - результат добавления клиента в уже поднятый WireGuard.
type AdoptedPeer struct {
	Interface       string `json:"iface"`
	ServerPublicKey string `json:"server_pub"`
	Port            int    `json:"port"`
	ClientIP        string `json:"client_ip"`
	Network         string `json:"network"`
	// Saved сообщает, попал ли пир в конфигурацию сервера. Если нет, он
	// живёт только до перезагрузки интерфейса.
	Saved bool `json:"saved"`
}

// adoptScript добавляет клиента в существующий интерфейс WireGuard.
//
// Нужен, когда на сервере работает обычный WireGuard, а не AmneziaWG:
// install.sh умеет заводить пиров только для второй, и на первой его
// команда peer-add срывается. Скрипт печатает один JSON-объект - тот же
// принцип, что у протокола install.sh.
const adoptScript = `
set -eu
CLIENT_PUB="$1"
IFACE="${2:-}"

command -v wg >/dev/null 2>&1 || { echo '{"error":"на сервере нет утилиты wg"}'; exit 0; }

if [ -z "$IFACE" ]; then
    IFACE="$(wg show interfaces 2>/dev/null | awk '{print $1}' | head -n1)"
fi
[ -n "$IFACE" ] || { echo '{"error":"на сервере не поднят ни один интерфейс WireGuard"}'; exit 0; }

SRV_PUB="$(wg show "$IFACE" public-key 2>/dev/null || true)"
PORT="$(wg show "$IFACE" listen-port 2>/dev/null || echo 0)"
[ -n "$SRV_PUB" ] || { echo '{"error":"не удалось прочитать публичный ключ сервера"}'; exit 0; }

# Сеть туннеля берём из адреса самого интерфейса.
ADDR="$(ip -4 -o addr show dev "$IFACE" 2>/dev/null | awk '{print $4}' | head -n1)"
[ -n "$ADDR" ] || { echo '{"error":"у интерфейса нет адреса IPv4"}'; exit 0; }
NET="$(echo "$ADDR" | cut -d/ -f1 | cut -d. -f1-3)"

# Свободный адрес: максимум из занятых плюс один.
MAX=1
for ip in $(wg show "$IFACE" allowed-ips 2>/dev/null | awk '{print $2}' | cut -d/ -f1); do
    case "$ip" in
        "$NET".*) n="${ip##*.}"; [ "$n" -gt "$MAX" ] && MAX="$n" || true ;;
    esac
done
SELF="$(echo "$ADDR" | cut -d/ -f1)"
case "$SELF" in "$NET".*) n="${SELF##*.}"; [ "$n" -gt "$MAX" ] && MAX="$n" || true ;; esac
CLIENT_IP="$NET.$((MAX + 1))"

wg set "$IFACE" peer "$CLIENT_PUB" allowed-ips "$CLIENT_IP/32"

# Сохраняем в конфиг, чтобы пир пережил перезагрузку сервера. Интерфейс
# может быть поднят не через wg-quick - тогда сохранить нечем.
SAVED=true
wg-quick save "$IFACE" >/dev/null 2>&1 || SAVED=false

printf '{"iface":"%s","server_pub":"%s","port":%s,"client_ip":"%s","network":"%s","saved":%s}\n' \
    "$IFACE" "$SRV_PUB" "${PORT:-0}" "$CLIENT_IP" "$NET" "$SAVED"
`

// AdoptWireGuard добавляет наш публичный ключ в существующий WireGuard
// сервера и возвращает всё, что нужно для конфигурации клиента.
func (c *Client) AdoptWireGuard(ctx context.Context, ssh profile.SSH, clientPub, iface string) (AdoptedPeer, error) {
	c.logf("$ добавляю клиента в существующий WireGuard сервера")

	client, err := dial(ctx, ssh)
	if err != nil {
		return AdoptedPeer{}, err
	}
	defer func() { _ = client.Close() }()

	out, errText, err := runScript(ctx, client, ssh, adoptScript, []string{clientPub, iface})
	if err != nil {
		return AdoptedPeer{}, err
	}

	line := lastJSONLine(out)
	if line == "" {
		detail := strings.TrimSpace(errText)
		if detail == "" {
			detail = truncate(strings.TrimSpace(out), 200)
		}
		return AdoptedPeer{}, fmt.Errorf("сервер не ответил на добавление клиента: %s", detail)
	}

	var res struct {
		AdoptedPeer
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &res); err != nil {
		return AdoptedPeer{}, fmt.Errorf("непонятный ответ сервера: %s", truncate(line, 200))
	}
	if res.Error != "" {
		return AdoptedPeer{}, fmt.Errorf("%s", res.Error)
	}
	c.logf("клиент добавлен: интерфейс %s, адрес %s", res.Interface, res.ClientIP)
	return res.AdoptedPeer, nil
}

// lastJSONLine возвращает последнюю строку, начинающуюся с '{'.
func lastJSONLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); strings.HasPrefix(line, "{") {
			return line
		}
	}
	return ""
}
