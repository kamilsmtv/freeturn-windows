package wg

import (
	"fmt"
	"strings"

	"github.com/kamilsmtv/freeturn-windows/internal/tunnel"
)

// TemplateOptions - что подставить в заготовку конфигурации клиента.
type TemplateOptions struct {
	// Address - адрес клиента внутри туннеля.
	Address string
	// Endpoint - локальный сокет ядра, куда пойдёт туннель.
	Endpoint string
	// ServerPublicKey известен не всегда: без него конфиг остаётся заготовкой.
	ServerPublicKey string
	DNS             string
}

// DefaultClientAddress - первый свободный адрес в сети, которую поднимает
// install.sh ядра (10.13.13.0/24; .1 занимает сам сервер).
const DefaultClientAddress = "10.13.13.2/32"

// Template собирает конфигурацию клиента со свежей парой ключей.
//
// Нужна, когда сервер поднят не нами и его конфиг взять неоткуда: в файле
// уже проставлены Endpoint, MTU и keepalive, а руками остаётся вписать
// только публичный ключ сервера.
func Template(o TemplateOptions) (conf string, keys tunnel.KeyPair, err error) {
	keys, err = tunnel.GenerateKeyPair()
	if err != nil {
		return "", keys, err
	}

	address := strings.TrimSpace(o.Address)
	if address == "" {
		address = DefaultClientAddress
	}
	endpoint := strings.TrimSpace(o.Endpoint)
	if endpoint == "" {
		endpoint = "127.0.0.1:9000"
	}
	serverKey := strings.TrimSpace(o.ServerPublicKey)
	if serverKey == "" {
		serverKey = "ВСТАВЬТЕ_ПУБЛИЧНЫЙ_КЛЮЧ_СЕРВЕРА"
	}

	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", keys.Private)
	fmt.Fprintf(&b, "Address = %s\n", address)
	if dns := strings.TrimSpace(o.DNS); dns != "" {
		fmt.Fprintf(&b, "DNS = %s\n", dns)
	}
	fmt.Fprintf(&b, "MTU = %d\n", MTU)

	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "PublicKey = %s\n", serverKey)
	b.WriteString("AllowedIPs = 0.0.0.0/0\n")
	fmt.Fprintf(&b, "Endpoint = %s\n", endpoint)
	b.WriteString("PersistentKeepalive = 25\n")

	return b.String(), keys, nil
}

// ClientConfOptions - значения для сборки конфигурации клиента.
type ClientConfOptions struct {
	PrivateKey      string
	Address         string
	ServerPublicKey string
	Endpoint        string
	DNS             string
}

// ClientConf собирает конфигурацию клиента по готовым значениям: ключи и
// адрес уже известны, генерировать нечего.
func ClientConf(o ClientConfOptions) string {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", o.PrivateKey)
	fmt.Fprintf(&b, "Address = %s\n", o.Address)
	if dns := strings.TrimSpace(o.DNS); dns != "" {
		fmt.Fprintf(&b, "DNS = %s\n", dns)
	}
	fmt.Fprintf(&b, "MTU = %d\n", MTU)

	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "PublicKey = %s\n", o.ServerPublicKey)
	b.WriteString("AllowedIPs = 0.0.0.0/0\n")
	fmt.Fprintf(&b, "Endpoint = %s\n", o.Endpoint)
	b.WriteString("PersistentKeepalive = 25\n")
	return b.String()
}
