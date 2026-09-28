package socks

import (
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

// handshake проводит приветствие, авторизацию и разбор запроса CONNECT.
// Возвращает адрес назначения в виде host:port.
func handshake(conn net.Conn, o Options) (string, error) {
	if err := negotiate(conn, o); err != nil {
		return "", err
	}
	return request(conn)
}

// negotiate выбирает способ авторизации и проводит её.
func negotiate(conn net.Conn, o Options) error {
	head := make([]byte, 2)
	if _, err := io.ReadFull(conn, head); err != nil {
		return err
	}
	if head[0] != version5 {
		// Сюда попадают HTTP-клиенты, настроенные на этот порт по ошибке.
		return fmt.Errorf("версия протокола %d не поддерживается, нужен SOCKS5", head[0])
	}

	methods := make([]byte, int(head[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}

	want := byte(methodNoAuth)
	if o.User != "" {
		want = methodUserPass
	}
	if !contains(methods, want) {
		// Отказ обязан быть отправлен: иначе клиент показывает обрыв
		// связи вместо внятной ошибки авторизации.
		_, _ = conn.Write([]byte{version5, methodNone})
		if want == methodUserPass {
			return errors.New("клиент не умеет авторизацию по логину и паролю")
		}
		return errors.New("клиент не предложил подходящий способ авторизации")
	}
	if _, err := conn.Write([]byte{version5, want}); err != nil {
		return err
	}
	if want == methodUserPass {
		return authenticate(conn, o)
	}
	return nil
}

// authenticate проверяет логин и пароль по RFC 1929.
func authenticate(conn net.Conn, o Options) error {
	ver := make([]byte, 1)
	if _, err := io.ReadFull(conn, ver); err != nil {
		return err
	}
	if ver[0] != authVersion {
		return fmt.Errorf("версия авторизации %d не поддерживается", ver[0])
	}

	user, err := readField(conn)
	if err != nil {
		return err
	}
	pass, err := readField(conn)
	if err != nil {
		return err
	}

	// Сравнение за постоянное время: пароль подбирается по сети, и
	// разница во времени ответа подсказывает длину совпавшего префикса.
	okUser := subtle.ConstantTimeCompare([]byte(user), []byte(o.User)) == 1
	okPass := subtle.ConstantTimeCompare([]byte(pass), []byte(o.Password)) == 1
	if !okUser || !okPass {
		_, _ = conn.Write([]byte{authVersion, 0x01})
		return errAuth
	}
	if _, err := conn.Write([]byte{authVersion, 0x00}); err != nil {
		return err
	}
	return nil
}

// request разбирает запрос и возвращает адрес назначения.
func request(conn net.Conn) (string, error) {
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return "", err
	}
	if head[0] != version5 {
		return "", fmt.Errorf("версия протокола %d не поддерживается", head[0])
	}
	if head[1] != cmdConnect {
		// BIND и UDP ASSOCIATE не поддержаны; клиенты уходят на TCP.
		_ = reply(conn, replyBadCommand, nil)
		return "", fmt.Errorf("команда %d не поддерживается, доступен только CONNECT", head[1])
	}

	var host string
	switch head[3] {
	case addrIPv4:
		buf := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		host = net.IP(buf).String()
	case addrIPv6:
		buf := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		host = net.IP(buf).String()
	case addrDomain:
		name, err := readField(conn)
		if err != nil {
			return "", err
		}
		if name == "" {
			_ = reply(conn, replyBadAddress, nil)
			return "", errors.New("пустое имя узла в запросе")
		}
		host = name
	default:
		_ = reply(conn, replyBadAddress, nil)
		return "", fmt.Errorf("тип адреса %d не поддерживается", head[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBuf)
	if port == 0 {
		_ = reply(conn, replyBadAddress, nil)
		return "", errors.New("нулевой порт в запросе")
	}
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

// reply отправляет ответ на запрос CONNECT. Адрес привязки клиенты, как
// правило, игнорируют, но поле обязано быть корректным.
func reply(conn net.Conn, code byte, bound net.Addr) error {
	out := []byte{version5, code, 0x00}

	addr, ok := bound.(*net.TCPAddr)
	if !ok || addr == nil || addr.IP == nil {
		out = append(out, addrIPv4, 0, 0, 0, 0, 0, 0)
		_, err := conn.Write(out)
		return err
	}

	if v4 := addr.IP.To4(); v4 != nil {
		out = append(out, addrIPv4)
		out = append(out, v4...)
	} else {
		out = append(out, addrIPv6)
		out = append(out, addr.IP.To16()...)
	}
	out = binary.BigEndian.AppendUint16(out, uint16(addr.Port))
	_, err := conn.Write(out)
	return err
}

// readField читает строку, перед которой идёт байт длины.
func readField(conn net.Conn) (string, error) {
	n := make([]byte, 1)
	if _, err := io.ReadFull(conn, n); err != nil {
		return "", err
	}
	if n[0] == 0 {
		return "", nil
	}
	buf := make([]byte, int(n[0]))
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func contains(list []byte, v byte) bool {
	for _, b := range list {
		if b == v {
			return true
		}
	}
	return false
}
