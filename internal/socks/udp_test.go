package socks

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"
)

// udpEcho поднимает UDP-узел, возвращающий датаграммы отправителю.
func udpEcho(t *testing.T) *net.UDPConn {
	t.Helper()
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("udp echo: %v", err)
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := pc.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteToUDP(buf[:n], from)
		}
	}()
	t.Cleanup(func() { _ = pc.Close() })
	return pc
}

// associate проводит приветствие и UDP ASSOCIATE, возвращая управляющее
// соединение, код ответа и адрес ретранслятора.
func associate(t *testing.T, addr, user, pass string) (net.Conn, byte, *net.UDPAddr) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	method := byte(methodNoAuth)
	if user != "" {
		method = methodUserPass
	}
	_, _ = c.Write([]byte{version5, 1, method})
	if _, err := io.ReadFull(c, make([]byte, 2)); err != nil {
		t.Fatalf("приветствие: %v", err)
	}
	if user != "" {
		out := append([]byte{authVersion, byte(len(user))}, user...)
		out = append(out, byte(len(pass)))
		out = append(out, pass...)
		_, _ = c.Write(out)
		ack := make([]byte, 2)
		if _, err := io.ReadFull(c, ack); err != nil || ack[1] != 0 {
			t.Fatalf("авторизация не прошла")
		}
	}
	_, _ = c.Write([]byte{version5, cmdUDPAssociate, 0, addrIPv4, 0, 0, 0, 0, 0, 0})
	resp := make([]byte, 10)
	if _, err := io.ReadFull(c, resp); err != nil {
		t.Fatalf("ответ: %v", err)
	}
	if resp[1] != replyOK {
		return c, resp[1], nil
	}
	return c, replyOK, &net.UDPAddr{IP: net.IP(resp[4:8]), Port: int(binary.BigEndian.Uint16(resp[8:10]))}
}

func wrap(dst netip.AddrPort, data []byte) []byte {
	return append(appendUDPHeader(nil, dst), data...)
}

func TestUDPAssociateRelays(t *testing.T) {
	echo := udpEcho(t)
	s := start(t, Options{UDP: true, User: "u", Password: "p"})
	if !s.UDP() {
		t.Fatal("UDP не поднялся")
	}

	_, code, relayAddr := associate(t, s.Addr(), "u", "p")
	if code != replyOK {
		t.Fatalf("код ответа = %d", code)
	}
	if relayAddr.Port != s.live.ln.Addr().(*net.TCPAddr).Port {
		t.Errorf("UDP-порт %d не совпадает с TCP", relayAddr.Port)
	}

	cl, err := net.DialUDP("udp", nil, relayAddr)
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer func() { _ = cl.Close() }()
	_ = cl.SetDeadline(time.Now().Add(5 * time.Second))

	dst := echo.LocalAddr().(*net.UDPAddr).AddrPort()
	if _, err := cl.Write(wrap(dst, []byte("ping"))); err != nil {
		t.Fatalf("Write: %v", err)
	}
	buf := make([]byte, 512)
	n, err := cl.Read(buf)
	if err != nil {
		t.Fatalf("ответ не пришёл: %v", err)
	}
	want := wrap(dst, []byte("ping"))
	if !bytes.Equal(buf[:n], want) {
		t.Errorf("получено %x, ожидалось %x", buf[:n], want)
	}
}

// Датаграммы с адреса без авторизованного TCP-соединения отбрасываются.
func TestUDPStrangerDropped(t *testing.T) {
	echo := udpEcho(t)
	s := start(t, Options{UDP: true})

	ctl, code, relayAddr := associate(t, s.Addr(), "", "")
	if code != replyOK {
		t.Fatalf("код ответа = %d", code)
	}
	// Управляющее соединение закрыто - ассоциации больше нет.
	_ = ctl.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.live.udp.mu.Lock()
		n := len(s.live.udp.live)
		s.live.udp.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cl, err := net.DialUDP("udp", nil, relayAddr)
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer func() { _ = cl.Close() }()
	_ = cl.SetDeadline(time.Now().Add(300 * time.Millisecond))
	_, _ = cl.Write(wrap(echo.LocalAddr().(*net.UDPAddr).AddrPort(), []byte("x")))
	if _, err := cl.Read(make([]byte, 64)); err == nil {
		t.Fatal("ответ пришёл без ассоциации")
	}
}

// Без UDP в настройках команда отклоняется, и клиент уходит на TCP.
func TestUDPDisabled(t *testing.T) {
	s := start(t, Options{})
	if _, code, _ := associate(t, s.Addr(), "", ""); code != replyBadCommand {
		t.Errorf("код ответа = %d, ожидался %d", code, replyBadCommand)
	}
}

// Ассоциация не держит остановку сервера.
func TestUDPStop(t *testing.T) {
	s := start(t, Options{UDP: true})
	if _, code, _ := associate(t, s.Addr(), "", ""); code != replyOK {
		t.Fatalf("код ответа = %d", code)
	}
	done := make(chan struct{})
	go func() { s.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop завис на живой ассоциации")
	}
}

func TestParseUDPHeader(t *testing.T) {
	pkt := []byte{0, 0, 0, addrDomain, 3, 'a', '.', 'b', 0, 53, 'x'}
	host, port, data, ok := parseUDPHeader(pkt)
	if !ok || host != "a.b" || port != 53 || string(data) != "x" {
		t.Errorf("разбор: %q %d %q %v", host, port, data, ok)
	}
	for _, bad := range [][]byte{
		{0, 0, 1, addrIPv4, 1, 2, 3, 4, 0, 53}, // фрагмент
		{0, 0, 0, addrIPv4, 1, 2},              // обрезан
		{0, 0, 0, addrDomain, 9, 'a'},          // имя длиннее пакета
		{0, 0, 0, addrIPv4, 1, 2, 3, 4, 0, 0},  // нулевой порт
		{0, 0, 0, 0x09, 1, 2, 3, 4, 0, 53},     // неизвестный тип
	} {
		if _, _, _, ok := parseUDPHeader(bad); ok {
			t.Errorf("принят битый заголовок %x", bad)
		}
	}
}
