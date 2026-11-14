package socks

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// echoServer поднимает узел назначения, который возвращает полученное.
func echoServer(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

// start поднимает сервер на случайном порту localhost.
func start(t *testing.T, o Options) *Server {
	t.Helper()
	if o.Listen == "" {
		o.Listen = "127.0.0.1:0"
	}
	// Узлы назначения в тестах живут на localhost, а боевая политика их
	// запрещает - иначе пришлось бы поднимать их во внешней сети.
	o.allowLoopback = true
	s := New()
	if err := s.Start(o); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

// dialSocks проводит приветствие и запрос CONNECT, возвращая соединение.
func dialSocks(t *testing.T, addr, user, pass, target string) (net.Conn, byte) {
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
	if _, err := c.Write([]byte{version5, 1, method}); err != nil {
		t.Fatalf("приветствие: %v", err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(c, resp); err != nil {
		t.Fatalf("ответ на приветствие: %v", err)
	}
	if resp[1] == methodNone {
		return c, methodNone
	}

	if method == methodUserPass {
		out := []byte{authVersion, byte(len(user))}
		out = append(out, user...)
		out = append(out, byte(len(pass)))
		out = append(out, pass...)
		if _, err := c.Write(out); err != nil {
			t.Fatalf("авторизация: %v", err)
		}
		ack := make([]byte, 2)
		if _, err := io.ReadFull(c, ack); err != nil {
			t.Fatalf("ответ авторизации: %v", err)
		}
		if ack[1] != 0 {
			return c, methodNone
		}
	}

	host, port, err := net.SplitHostPort(target)
	if err != nil {
		t.Fatalf("адрес назначения: %v", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("порт назначения: %v", err)
	}
	req := []byte{version5, cmdConnect, 0x00, addrDomain, byte(len(host))}
	req = append(req, host...)
	req = binary.BigEndian.AppendUint16(req, uint16(n))
	if _, err := c.Write(req); err != nil {
		t.Fatalf("запрос: %v", err)
	}

	head := make([]byte, 4)
	if _, err := io.ReadFull(c, head); err != nil {
		t.Fatalf("ответ на запрос: %v", err)
	}
	// Дочитываем адрес привязки, иначе он останется в потоке данных.
	switch head[3] {
	case addrIPv4:
		_, _ = io.CopyN(io.Discard, c, net.IPv4len+2)
	case addrIPv6:
		_, _ = io.CopyN(io.Discard, c, net.IPv6len+2)
	}
	return c, head[1]
}

func TestConnectNoAuth(t *testing.T) {
	echo := echoServer(t)
	s := start(t, Options{})

	c, code := dialSocks(t, s.Addr(), "", "", echo.Addr().String())
	if code != replyOK {
		t.Fatalf("код ответа = %d, ожидался %d", code, replyOK)
	}

	if _, err := c.Write([]byte("привет")); err != nil {
		t.Fatalf("запись: %v", err)
	}
	buf := make([]byte, len("привет"))
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if string(buf) != "привет" {
		t.Errorf("эхо вернуло %q", buf)
	}
}

func TestConnectUserPass(t *testing.T) {
	echo := echoServer(t)
	s := start(t, Options{User: "kamil", Password: "секрет"})

	if _, code := dialSocks(t, s.Addr(), "kamil", "секрет", echo.Addr().String()); code != replyOK {
		t.Fatalf("верный пароль отвергнут, код %d", code)
	}
	if _, code := dialSocks(t, s.Addr(), "kamil", "другой", echo.Addr().String()); code != methodNone {
		t.Errorf("неверный пароль принят, код %d", code)
	}
}

// Сервер с паролем обязан отказать клиенту, который пароля не предлагает:
// иначе раздача оказалась бы открытой при живой настройке авторизации.
func TestNoAuthRejectedWhenPasswordSet(t *testing.T) {
	echo := echoServer(t)
	s := start(t, Options{User: "kamil", Password: "секрет"})

	c, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := c.Write([]byte{version5, 1, methodNoAuth}); err != nil {
		t.Fatalf("приветствие: %v", err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(c, resp); err != nil {
		t.Fatalf("ответ: %v", err)
	}
	if resp[1] != methodNone {
		t.Fatalf("сервер принял способ %d вместо отказа", resp[1])
	}
	_ = echo
}

func TestUnsupportedCommand(t *testing.T) {
	s := start(t, Options{})

	c, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	_, _ = c.Write([]byte{version5, 1, methodNoAuth})
	_, _ = io.ReadFull(c, make([]byte, 2))
	// UDP ASSOCIATE.
	_, _ = c.Write([]byte{version5, 0x03, 0x00, addrIPv4, 127, 0, 0, 1, 0, 80})

	head := make([]byte, 4)
	if _, err := io.ReadFull(c, head); err != nil {
		t.Fatalf("ответ: %v", err)
	}
	if head[1] != replyBadCommand {
		t.Errorf("код ответа = %d, ожидался %d", head[1], replyBadCommand)
	}
}

// Трафик обязан уходить через Dial: на этом держится весь замысел -
// соединение делает наш процесс, и системные маршруты уводят его в туннель.
func TestDialIsUsed(t *testing.T) {
	echo := echoServer(t)
	seen := make(chan string, 1)
	s := start(t, Options{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			seen <- addr
			return (&net.Dialer{}).DialContext(ctx, network, echo.Addr().String())
		},
	})

	if _, code := dialSocks(t, s.Addr(), "", "", "example.org:443"); code != replyOK {
		t.Fatalf("код ответа = %d", code)
	}
	select {
	case addr := <-seen:
		if addr != "example.org:443" {
			t.Errorf("Dial получил %q", addr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Dial не вызван")
	}
}

func TestStopClosesListener(t *testing.T) {
	s := start(t, Options{})
	addr := s.Addr()
	s.Stop()

	if s.Running() {
		t.Error("Running = true после Stop")
	}
	if _, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		t.Error("сокет остался открытым после Stop")
	}
}

func TestStartTwice(t *testing.T) {
	s := start(t, Options{})
	if err := s.Start(Options{Listen: "127.0.0.1:0"}); !errors.Is(err, ErrAlreadyUp) {
		t.Errorf("повторный Start вернул %v", err)
	}
}

func TestBusyPortError(t *testing.T) {
	s := start(t, Options{})
	other := New()
	err := other.Start(Options{Listen: s.Addr()})
	if err == nil {
		other.Stop()
		t.Fatal("занятый порт принят")
	}
	if !strings.Contains(err.Error(), "занят") {
		t.Errorf("невнятная ошибка занятого порта: %v", err)
	}
}

// silentServer принимает соединения и молчит в них: так ведут себя
// открытый SSH или веб-сокет в простое.
func silentServer(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("silent: %v", err)
	}
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		for _, c := range held {
			_ = c.Close()
		}
		mu.Unlock()
	})
	return ln
}

// Stop обязан завершиться, даже когда через сервер идёт молчащее
// соединение: иначе «Отключить» и выход из приложения подвисали бы,
// пока узел назначения не соизволит что-нибудь прислать.
func TestStopWithIdleConnection(t *testing.T) {
	silent := silentServer(t)
	s := start(t, Options{})

	if _, code := dialSocks(t, s.Addr(), "", "", silent.Addr().String()); code != replyOK {
		t.Fatalf("код ответа = %d", code)
	}

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop не завершился за 5 секунд: молчащее соединение держит сервер")
	}
}

// Остановка обязана оборвать чужой канал, а не оставить соседу рабочий
// выход в сеть уже после отключения.
func TestStopBreaksLiveConnection(t *testing.T) {
	echo := echoServer(t)
	s := start(t, Options{})

	c, code := dialSocks(t, s.Addr(), "", "", echo.Addr().String())
	if code != replyOK {
		t.Fatalf("код ответа = %d", code)
	}
	s.Stop()

	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte("ping")); err == nil {
		if _, err = io.ReadFull(c, make([]byte, 4)); err == nil {
			t.Error("соединение пережило остановку сервера")
		}
	}
}

// Через раздачу нельзя дотянуться до самой этой машины: иначе сосед
// получил бы доступ к локальным службам в обход брандмауэра.
func TestLoopbackDenied(t *testing.T) {
	echo := echoServer(t)

	s := New()
	if err := s.Start(Options{Listen: "127.0.0.1:0"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(s.Stop)

	_, code := dialSocks(t, s.Addr(), "", "", echo.Addr().String())
	if code != replyNotAllowed {
		t.Errorf("код ответа = %d, ожидался запрет %d", code, replyNotAllowed)
	}
}

// Запрет должен работать и по имени узла: клиент указывает имя, а куда
// оно указывает, выясняется уже при подключении.
func TestLoopbackDeniedByName(t *testing.T) {
	echo := echoServer(t)
	_, port, err := net.SplitHostPort(echo.Addr().String())
	if err != nil {
		t.Fatalf("адрес эха: %v", err)
	}

	s := New()
	if err := s.Start(Options{Listen: "127.0.0.1:0"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(s.Stop)

	_, code := dialSocks(t, s.Addr(), "", "", net.JoinHostPort("localhost", port))
	if code != replyNotAllowed {
		t.Errorf("код ответа = %d, ожидался запрет %d", code, replyNotAllowed)
	}
}

// IPv6 туннель не ведёт: такой запрос ушёл бы мимо него, поэтому запрещён.
func TestCheckTarget(t *testing.T) {
	for addr, want := range map[string]error{
		"8.8.8.8":         nil,
		"::ffff:8.8.8.8":  nil,
		"127.0.0.1":       errLoopback,
		"0.0.0.0":         errLoopback,
		"::1":             errLoopback,
		"2001:4860::8888": errIPv6,
	} {
		if got := checkTarget(Options{}, netip.MustParseAddr(addr)); !errors.Is(got, want) {
			t.Errorf("%s: %v, want %v", addr, got, want)
		}
	}
}

func TestIPv6TargetDenied(t *testing.T) {
	s := New()
	if err := s.Start(Options{Listen: "127.0.0.1:0"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(s.Stop)
	if _, code := dialSocks(t, s.Addr(), "", "", "[2001:db8::1]:443"); code != replyNotAllowed {
		t.Errorf("код ответа = %d, ожидался запрет %d", code, replyNotAllowed)
	}
}
