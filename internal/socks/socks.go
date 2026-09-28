// Package socks поднимает SOCKS5-сервер для раздачи туннеля в локальную сеть.
//
// Смысл в том, что в режиме VPN туннель - настоящий адаптер Windows, и
// системные маршруты уже уводят в него весь исходящий трафик процесса.
// Поэтому серверу не нужно ничего знать про WireGuard: он принимает
// соединение снаружи и делает обычный Dial, который уходит в туннель сам.
//
// Поддерживается только CONNECT. UDP ASSOCIATE не реализован: клиенты в
// ответ на отказ уходят на TCP, и для браузера этого достаточно.
//
// Единственный запрет - подключения к самой этой машине (см. dialer).
// Остальную локальную сеть не ограничиваем: подключившийся сосед и так
// стоит в ней и дотянется до тех же узлов напрямую, без нас.
package socks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Версия протокола и коды из RFC 1928.
const (
	version5 = 0x05

	methodNoAuth   = 0x00
	methodUserPass = 0x02
	methodNone     = 0xFF

	cmdConnect = 0x01

	addrIPv4   = 0x01
	addrDomain = 0x03
	addrIPv6   = 0x04

	replyOK          = 0x00
	replyFailure     = 0x01
	replyNotAllowed  = 0x02
	replyUnreachable = 0x04
	replyRefused     = 0x05
	replyBadCommand  = 0x07
	replyBadAddress  = 0x08
)

// authVersion - версия подпротокола логина и пароля (RFC 1929).
const authVersion = 0x01

// handshakeTimeout ограничивает приветствие: подключившийся и замолчавший
// сосед не должен занимать горутину бесконечно.
const handshakeTimeout = 15 * time.Second

// dialTimeout - сколько ждём узел назначения. Через TURN-релей путь
// небыстрый, но минута ожидания для браузера уже бесполезна.
const dialTimeout = 30 * time.Second

// authLogEvery - как часто пускать в журнал сообщение о неверном пароле.
// Без ограничения перебор с соседней машины вытеснит из журнала всё
// остальное, а смысл сообщения теряется уже после первого.
const authLogEvery = 10 * time.Second

// ErrAlreadyUp - сервер уже запущен.
var ErrAlreadyUp = errors.New("SOCKS5-сервер уже запущен")

// errLoopback - попытка дотянуться через раздачу до самой этой машины.
var errLoopback = errors.New("подключения к этому компьютеру через раздачу запрещены")

// errAuth - неверный логин или пароль. В отличие от прочего сора на
// сокете об этом стоит сказать вслух: владелец ждёт подключения соседа.
var errAuth = errors.New("неверный логин или пароль")

// Options - параметры запуска.
type Options struct {
	// Listen - адрес сокета, обычно 0.0.0.0:1080.
	Listen string
	// User и Password включают обязательную авторизацию. Пустая пара
	// означает открытый сервер - решение принимает вызывающий.
	User     string
	Password string
	// Log получает сообщения для журнала приложения.
	Log func(level, msg string)
	// Dial переопределяет способ подключения к узлу назначения; нужен тестам.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)

	// allowLoopback снимает запрет на подключения к этой же машине.
	// Нужен только тестам: у них узел назначения живёт на localhost.
	allowLoopback bool
}

// session - один запуск сервера. Отдельная структура нужна из-за
// WaitGroup: переиспользовать её между запусками нельзя, Add во время
// чужого Wait - это гонка.
type session struct {
	ln   net.Listener
	stop chan struct{}
	wg   sync.WaitGroup
	// active - сколько соединений обслуживается прямо сейчас.
	active atomic.Int64
	// lastAuthLog - время последнего сообщения о неверном пароле.
	lastAuthLog atomic.Int64
}

// Server - SOCKS5-сервер, который можно запускать и останавливать.
type Server struct {
	mu   sync.Mutex
	live *session
}

// New создаёт сервер; сокет при этом не открывается.
func New() *Server { return &Server{} }

// Start открывает сокет и начинает принимать соединения.
func (s *Server) Start(o Options) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.live != nil {
		return ErrAlreadyUp
	}

	listen := strings.TrimSpace(o.Listen)
	if listen == "" {
		return errors.New("не задан адрес SOCKS5-сервера")
	}

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return listenError(listen, err)
	}

	live := &session{ln: ln, stop: make(chan struct{})}
	s.live = live

	live.wg.Add(1)
	go live.accept(o)
	return nil
}

// Stop закрывает сокет и дожидается завершения обработчиков.
func (s *Server) Stop() {
	s.mu.Lock()
	live := s.live
	s.live = nil
	s.mu.Unlock()

	if live == nil {
		return
	}
	close(live.stop)
	_ = live.ln.Close()
	live.wg.Wait()
}

// Running сообщает, открыт ли сокет.
func (s *Server) Running() bool { return s.session() != nil }

// Addr возвращает адрес, на котором сервер принимает соединения.
// Он берётся у сокета: в Listen мог стоять порт 0.
func (s *Server) Addr() string {
	live := s.session()
	if live == nil {
		return ""
	}
	return live.ln.Addr().String()
}

// Active - число обслуживаемых соединений.
func (s *Server) Active() int {
	live := s.session()
	if live == nil {
		return 0
	}
	return int(live.active.Load())
}

func (s *Server) session() *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live
}

func (c *session) accept(o Options) {
	defer c.wg.Done()
	for {
		conn, err := c.ln.Accept()
		if err != nil {
			// Закрытый сокет - это наша остановка, а не сбой.
			select {
			case <-c.stop:
				return
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			// Ошибка может быть и неустранимой (кончились дескрипторы):
			// без паузы цикл раскрутится вхолостую и зальёт журнал.
			logf(o, "warn", "SOCKS5: соединение не принято: "+err.Error())
			select {
			case <-c.stop:
				return
			case <-time.After(time.Second):
			}
			continue
		}

		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.serve(conn, o)
		}()
	}
}

// serve обслуживает одно клиентское соединение.
func (c *session) serve(conn net.Conn, o Options) {
	c.active.Add(1)
	defer c.active.Add(-1)
	defer func() { _ = conn.Close() }()

	// Остановка сервера обрывает и уже принятые соединения, иначе
	// «Отключить» в интерфейсе оставляло бы соседям живой канал.
	//
	// Закрывать нужно обе стороны. Если оборвать только клиентскую,
	// перекачка так и останется ждать молчащий узел назначения - а с ней
	// и Stop, который дожидается всех обработчиков. Молчащее соединение
	// (открытый SSH, веб-сокет) подвесило бы отключение навсегда.
	var mu sync.Mutex
	var remote net.Conn
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-c.stop:
			_ = conn.Close()
			mu.Lock()
			if remote != nil {
				_ = remote.Close()
			}
			mu.Unlock()
		case <-done:
		}
	}()

	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	target, err := handshake(conn, o)
	if err != nil {
		c.logHandshake(o, conn, err)
		return
	}

	// Дальше идёт подключение к узлу назначения, и оно само по себе
	// дольше приветствия. Со старым дедлайном ответ клиенту не успел бы
	// записаться, и тот видел бы обрыв вместо открытого канала.
	_ = conn.SetDeadline(time.Now().Add(dialTimeout + handshakeTimeout))

	dial := o.Dial
	if dial == nil {
		dial = dialer(o).DialContext
	}
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	r, err := dial(ctx, "tcp", target)
	if err != nil {
		_ = reply(conn, replyCodeFor(err), nil)
		logf(o, "debug", "SOCKS5: "+target+" недоступен: "+err.Error())
		return
	}
	defer func() { _ = r.Close() }()

	mu.Lock()
	remote = r
	mu.Unlock()
	// Остановка могла случиться, пока шло подключение: тогда сторож её
	// уже проспал, и закрыть вторую сторону некому, кроме нас.
	select {
	case <-c.stop:
		return
	default:
	}

	if err := reply(conn, replyOK, r.LocalAddr()); err != nil {
		return
	}

	// Дальше общее время сеанса не ограничено: это обычный поток данных.
	_ = conn.SetDeadline(time.Time{})
	_ = r.SetDeadline(time.Time{})
	pipe(conn, r)
}

// dialer собирает исходящее подключение с одним запретом: до самой этой
// машины.
//
// Раздача задумана как выход в интернет через туннель, а не как доступ к
// хозяйскому компьютеру. Без запрета сосед дотянулся бы по 127.0.0.1 до
// всего, что слушает локально: панелей управления, баз, общих папок - и всё это в
// обход брандмауэра, который такие порты наружу не пускает.
//
// Проверка стоит в Control, а не на имени узла: туда адрес приходит уже
// разрешённым, поэтому имя, указывающее на localhost, тоже не пройдёт.
func dialer(o Options) *net.Dialer {
	d := &net.Dialer{Timeout: dialTimeout}
	if o.allowLoopback {
		return d
	}
	d.Control = func(_, address string, _ syscall.RawConn) error {
		// Адрес сюда приходит от резолвера системы и разбирается всегда;
		// неразобранный пропускаем - придумывать политику для того, чего
		// не бывает, вреднее, чем довериться системе.
		ap, err := netip.ParseAddrPort(address)
		if err == nil && (ap.Addr().IsLoopback() || ap.Addr().IsUnspecified()) {
			return errLoopback
		}
		return nil
	}
	return d
}

// logHandshake пишет в журнал только то, что владельцу полезно.
//
// Сканеры портов и случайные подключения в общей сети - обычное дело, и
// сообщать о каждом незачем. А вот неверный пароль стоит показать: чаще
// всего это сосед, которому дали не тот пароль.
func (c *session) logHandshake(o Options, conn net.Conn, err error) {
	if !errors.Is(err, errAuth) {
		logf(o, "debug", "SOCKS5: приветствие от "+peerOf(conn)+" не принято: "+err.Error())
		return
	}

	now := time.Now().UnixNano()
	last := c.lastAuthLog.Load()
	if now-last < int64(authLogEvery) {
		return
	}
	// Гонку разрешаем в пользу одного сообщения: проигравший промолчит.
	if !c.lastAuthLog.CompareAndSwap(last, now) {
		return
	}
	logf(o, "warn", "SOCKS5: неверный логин или пароль, узел "+peerOf(conn))
}

// pipe перекачивает данные в обе стороны, пока одна из сторон не закроется.
func pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	shovel := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		// Полузакрытие даёт второй стороне дочитать ответ.
		if tcp, ok := dst.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}
	go shovel(a, b)
	go shovel(b, a)
	wg.Wait()
}

// logf отправляет сообщение в журнал приложения, если он задан.
func logf(o Options, level, msg string) {
	if o.Log != nil {
		o.Log(level, msg)
	}
}

// peerOf - адрес подключившегося для журнала.
func peerOf(conn net.Conn) string {
	if a := conn.RemoteAddr(); a != nil {
		return a.String()
	}
	return "неизвестного узла"
}

// replyCodeFor переводит ошибку подключения в код ответа SOCKS5.
func replyCodeFor(err error) byte {
	var ne net.Error
	switch {
	case errors.Is(err, errLoopback):
		return replyNotAllowed
	case isRefused(err):
		return replyRefused
	case isUnreachable(err):
		return replyUnreachable
	case errors.As(err, &ne) && ne.Timeout():
		return replyUnreachable
	default:
		return replyFailure
	}
}

// listenError объясняет занятый порт человеческими словами: сообщение
// системы про сокет пользователю ничего не говорит.
func listenError(listen string, err error) error {
	if isAddrInUse(err) {
		return fmt.Errorf("порт %s уже занят другой программой - выберите другой", listen)
	}
	return fmt.Errorf("не удалось открыть %s: %w", listen, err)
}
