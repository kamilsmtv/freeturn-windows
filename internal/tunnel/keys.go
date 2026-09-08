package tunnel

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// KeyPair - пара ключей WireGuard в том виде, в каком они пишутся в conf.
type KeyPair struct {
	Private string `json:"private"`
	Public  string `json:"public"`
}

// GenerateKeyPair создаёт новую пару ключей X25519.
func GenerateKeyPair() (KeyPair, error) {
	var priv [KeyLen]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return KeyPair{}, fmt.Errorf("нет источника случайных чисел: %w", err)
	}
	// Клэмпинг X25519: так же поступает wg genkey.
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		return KeyPair{}, fmt.Errorf("не удалось вычислить публичный ключ: %w", err)
	}
	return KeyPair{
		Private: base64.StdEncoding.EncodeToString(priv[:]),
		Public:  base64.StdEncoding.EncodeToString(pub),
	}, nil
}

// PublicKeyFor вычисляет публичный ключ по приватному (base64).
func PublicKeyFor(privateBase64 string) (string, error) {
	key, err := ParseKey(privateBase64)
	if err != nil {
		return "", err
	}
	pub, err := curve25519.X25519(key[:], curve25519.Basepoint)
	if err != nil {
		return "", fmt.Errorf("не удалось вычислить публичный ключ: %w", err)
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}
