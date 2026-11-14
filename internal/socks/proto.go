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

// handshake проводит приветствие, авторизацию и разбор запроса.
// Возвращает команду и адрес из запроса в виде host:port.
func handshake(conn net.Conn, o Options, udp bool) (byte, string, error) {
	if err := negotiate(conn, o); err != nil {
		return 0, "", err
	}
	return request(conn, udp)
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

// request разбирает запрос и возвращает команду и адрес. udp разрешает
// UDP ASSOCIATE; без него, как и на BIND, клиент получает отказ и уходит
// на TCP.
func request(conn net.Conn, udp bool) (byte, string, error) {
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return 0, "", err
	}
	if head[0] != version5 {
		return 0, "", fmt.Errorf("версия протокола %d не поддерживается", head[0])
	}
	cmd := head[1]
	if cmd != cmdConnect && (cmd != cmdUDPAssociate || !udp) {
		_ = reply(conn, replyBadCommand, nil)
		return 0, "", fmt.Errorf("команда %d не поддерживается", cmd)
	}

	var host string
	switch head[3] {
	case addrIPv4:
		buf := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return 0, "", err
		}
		host = net.IP(buf).String()
	case addrIPv6:
		buf := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return 0, "", err
		}
		host = net.IP(buf).String()
	case addrDomain:
		name, err := readField(conn)
		if err != nil {
			return 0, "", err
		}
		if name == "" {
			_ = reply(conn, replyBadAddress, nil)
			return 0, "", errors.New("пустое имя узла в запросе")
		}
		host = name
	default:
		_ = reply(conn, replyBadAddress, nil)
		return 0, "", fmt.Errorf("тип адреса %d не поддерживается", head[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return 0, "", err
	}
	port := binary.BigEndian.Uint16(portBuf)
	// В UDP ASSOCIATE клиент объявляет свой адрес, и нули там обычны.
	if port == 0 && cmd == cmdConnect {
		_ = reply(conn, replyBadAddress, nil)
		return 0, "", errors.New("нулевой порт в запросе")
	}
	return cmd, net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

// reply отправляет ответ на запрос. Адрес привязки клиенты, как
// правило, игнорируют, но поле обязано быть корректным.
func reply(conn net.Conn, code byte, bound net.Addr) error {
	out := []byte{version5, code, 0x00}

	var addr *net.TCPAddr
	switch b := bound.(type) {
	case *net.TCPAddr:
		addr = b
	case *net.UDPAddr:
		if b != nil {
			addr = &net.TCPAddr{IP: b.IP, Port: b.Port}
		}
	}
	if addr == nil || addr.IP == nil {
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
