package socks

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

// UDP ASSOCIATE (RFC 1928, раздел 7).
//
// Без него у соседей нет QUIC, звонков и игр: браузер уходит на TCP, а
// мессенджеры с голосом просто молчат. Устроено так:
//
//   - один UDP-сокет слушает тот же адрес и порт, что и TCP. Так в
//     брандмауэре хватает одного правила на известный порт, а не дыры на
//     случайных;
//   - пускаем датаграммы только с адресов, у которых открыто авторизованное
//     TCP-соединение с командой UDP ASSOCIATE. Закрылось управляющее
//     соединение - кончилась и ассоциация;
//   - наружу каждая ассоциация шлёт через свой несвязанный сокет. Привязка к
//     адресу локальной сети увела бы трафик мимо туннеля, а без неё маршрут
//     выбирает система - тот же, что у TCP через Dial.

// cmdUDPAssociate - команда UDP ASSOCIATE.
const cmdUDPAssociate = 0x03

// maxDatagram - больше UDP через IPv4 не унесёт.
const maxDatagram = 64 * 1024

// resolveTimeout - сколько ждём DNS для датаграммы с именем узла. Пока
// ждём, стоит весь приём, поэтому срок короткий.
const resolveTimeout = 2 * time.Second

// nameTTL и failTTL - сколько помнить ответ DNS и отказ. Отказ помним,
// чтобы поток датаграмм на несуществующее имя не вешал приём на каждой.
const (
	nameTTL = 5 * time.Minute
	failTTL = 30 * time.Second
)

// nameEntry - запись кеша имён; пустой ip означает отказ.
type nameEntry struct {
	ip    netip.Addr
	until time.Time
}

// relay - общий UDP-сокет сервера и таблица ассоциаций.
type relay struct {
	pc   *net.UDPConn
	o    Options
	stop chan struct{}

	mu sync.Mutex
	// byAddr - ассоциации, уже знающие порт клиента.
	byAddr map[netip.AddrPort]*assoc
	// pending - ассоциации, ещё не получившие первой датаграммы, по IP
	// клиента. Порт клиенты обычно не объявляют (шлют 0.0.0.0:0), и узнать
	// его можно только из первой датаграммы.
	pending map[netip.Addr][]*assoc
	// live - все ассоциации IP-адреса; нужны, когда приложение соседа шлёт
	// с другого порта, чем объявило.
	live map[netip.Addr][]*assoc
}

// assoc - одна ассоциация: управляющее TCP-соединение и свой исходящий сокет.
type assoc struct {
	r      *relay
	ip     netip.Addr
	out    *net.UDPConn
	mu     sync.Mutex
	client netip.AddrPort // куда отдавать ответы; пусто до первой датаграммы
	names  map[string]nameEntry
	done   chan struct{}
}

// listenRelay открывает UDP на том же адресе, что и TCP-сокет.
func listenRelay(tcpAddr net.Addr, o Options, stop chan struct{}) (*relay, error) {
	ta, ok := tcpAddr.(*net.TCPAddr)
	if !ok {
		return nil, errors.New("неизвестный тип адреса сокета")
	}
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: ta.IP, Port: ta.Port, Zone: ta.Zone})
	if err != nil {
		return nil, err
	}
	return &relay{
		pc:      pc,
		o:       o,
		stop:    stop,
		byAddr:  map[netip.AddrPort]*assoc{},
		pending: map[netip.Addr][]*assoc{},
		live:    map[netip.Addr][]*assoc{},
	}, nil
}

// close закрывает общий сокет; исходящие закроют их ассоциации.
func (r *relay) close() { _ = r.pc.Close() }

// open заводит ассоциацию для клиента с адресом ip.
func (r *relay) open(ip netip.Addr, declared netip.AddrPort) (*assoc, error) {
	out, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}
	a := &assoc{
		r:     r,
		ip:    ip.Unmap(),
		out:   out,
		names: map[string]nameEntry{},
		done:  make(chan struct{}),
	}

	r.mu.Lock()
	// Объявленный адрес берём, только если он указывает на того же
	// клиента: иначе сосед мог бы попросить слать ответы третьей машине.
	if declared.Port() != 0 && (declared.Addr().Unmap() == a.ip || declared.Addr().IsUnspecified()) {
		a.client = netip.AddrPortFrom(a.ip, declared.Port())
		r.byAddr[a.client] = a
	} else {
		r.pending[a.ip] = append(r.pending[a.ip], a)
	}
	r.live[a.ip] = append(r.live[a.ip], a)
	r.mu.Unlock()

	go a.readReplies()
	return a, nil
}

// close снимает ассоциацию из таблиц и закрывает её сокет.
func (a *assoc) close() {
	r := a.r
	r.mu.Lock()
	// Запасной путь в lookup мог привязать к ассоциации несколько портов.
	for k, v := range r.byAddr {
		if v == a {
			delete(r.byAddr, k)
		}
	}
	r.pending[a.ip] = without(r.pending[a.ip], a)
	if len(r.pending[a.ip]) == 0 {
		delete(r.pending, a.ip)
	}
	r.live[a.ip] = without(r.live[a.ip], a)
	if len(r.live[a.ip]) == 0 {
		delete(r.live, a.ip)
	}
	r.mu.Unlock()
	_ = a.out.Close()
	<-a.done
}

func without(list []*assoc, a *assoc) []*assoc {
	out := list[:0]
	for _, x := range list {
		if x != a {
			out = append(out, x)
		}
	}
	return out
}

// lookup находит ассоциацию для датаграммы от src.
func (r *relay) lookup(src netip.AddrPort) *assoc {
	src = netip.AddrPortFrom(src.Addr().Unmap(), src.Port())
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.byAddr[src]; a != nil {
		return a
	}
	if q := r.pending[src.Addr()]; len(q) > 0 {
		a := q[0]
		r.pending[src.Addr()] = q[1:]
		if len(q) == 1 {
			delete(r.pending, src.Addr())
		}
		a.mu.Lock()
		a.client = src
		a.mu.Unlock()
		r.byAddr[src] = a
		return a
	}
	// Ассоциации у этого IP есть, но все уже привязаны к другим портам:
	// значит, приложение шлёт с нового сокета. Отдаём последней - чужим
	// адресам сюда всё равно не попасть, а сосед с паролем уже свой.
	if l := r.live[src.Addr()]; len(l) > 0 {
		a := l[len(l)-1]
		r.byAddr[src] = a
		return a
	}
	return nil
}

// serve читает датаграммы соседей и отправляет их по назначению.
func (r *relay) serve() {
	buf := make([]byte, maxDatagram)
	for {
		n, src, err := r.pc.ReadFromUDPAddrPort(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			select {
			case <-r.stop:
				return
			default:
			}
			continue
		}
		a := r.lookup(src)
		if a == nil {
			// Чужой адрес без авторизованного TCP: молча отбрасываем,
			// отвечать сканеру незачем.
			continue
		}
		// Ответы уходят туда, откуда пришла последняя датаграмма: если
		// приложение соседа сменило сокет, старый порт ответов уже не ждёт.
		a.mu.Lock()
		a.client = netip.AddrPortFrom(src.Addr().Unmap(), src.Port())
		a.mu.Unlock()
		a.send(buf[:n])
	}
}

// send разбирает заголовок SOCKS5 и отправляет данные узлу назначения.
func (a *assoc) send(pkt []byte) {
	host, port, data, ok := parseUDPHeader(pkt)
	if !ok {
		return
	}
	dst, err := a.resolve(host)
	if err != nil {
		logf(a.r.o, "debug", "SOCKS5 UDP: "+host+" не разрешён: "+err.Error())
		return
	}
	if checkTarget(a.r.o, dst) != nil {
		return
	}
	_, _ = a.out.WriteToUDPAddrPort(data, netip.AddrPortFrom(dst, port))
}

// resolve переводит имя в адрес с небольшим кешем: имя приходит в каждой
// датаграмме, и спрашивать DNS на каждую незачем.
func (a *assoc) resolve(host string) (netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Unmap(), nil
	}
	now := time.Now()
	if e, ok := a.names[host]; ok && now.Before(e.until) {
		if !e.ip.IsValid() {
			return netip.Addr{}, errors.New("недавно не разрешилось")
		}
		return e.ip, nil
	}
	if len(a.names) > 256 {
		clear(a.names)
	}
	ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err == nil && len(ips) == 0 {
		err = errors.New("нет адресов")
	}
	if err != nil {
		a.names[host] = nameEntry{until: now.Add(failTTL)}
		return netip.Addr{}, err
	}
	// IPv4 предпочтительнее: туннель уводит только 0.0.0.0/0, а IPv6
	// политика раздачи всё равно не пропустит (см. checkTarget).
	ip := ips[0].Unmap()
	for _, c := range ips {
		if c.Unmap().Is4() {
			ip = c.Unmap()
			break
		}
	}
	a.names[host] = nameEntry{ip: ip, until: now.Add(nameTTL)}
	return ip, nil
}

// readReplies возвращает ответы узлов соседу, заворачивая их в заголовок.
func (a *assoc) readReplies() {
	defer close(a.done)
	buf := make([]byte, maxDatagram)
	for {
		n, from, err := a.out.ReadFromUDPAddrPort(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		a.mu.Lock()
		client := a.client
		a.mu.Unlock()
		if !client.IsValid() {
			continue
		}
		pkt := appendUDPHeader(make([]byte, 0, n+22), from)
		pkt = append(pkt, buf[:n]...)
		_, _ = a.r.pc.WriteToUDPAddrPort(pkt, client)
	}
}

// parseUDPHeader разбирает RSV(2) FRAG ATYP DST.ADDR DST.PORT DATA.
// Фрагментированные датаграммы отбрасываем: их не шлёт ни один живой
// клиент, а RFC разрешает их не поддерживать.
func parseUDPHeader(p []byte) (host string, port uint16, data []byte, ok bool) {
	if len(p) < 4 || p[2] != 0 {
		return "", 0, nil, false
	}
	rest := p[4:]
	switch p[3] {
	case addrIPv4:
		if len(rest) < net.IPv4len+2 {
			return "", 0, nil, false
		}
		host = netip.AddrFrom4([4]byte(rest[:4])).String()
		rest = rest[4:]
	case addrIPv6:
		if len(rest) < net.IPv6len+2 {
			return "", 0, nil, false
		}
		host = netip.AddrFrom16([16]byte(rest[:16])).String()
		rest = rest[16:]
	case addrDomain:
		if len(rest) < 1 || len(rest) < 1+int(rest[0])+2 || rest[0] == 0 {
			return "", 0, nil, false
		}
		host = string(rest[1 : 1+int(rest[0])])
		rest = rest[1+int(rest[0]):]
	default:
		return "", 0, nil, false
	}
	port = binary.BigEndian.Uint16(rest[:2])
	if port == 0 {
		return "", 0, nil, false
	}
	return host, port, rest[2:], true
}

// appendUDPHeader дописывает заголовок с адресом отправителя ответа.
func appendUDPHeader(b []byte, from netip.AddrPort) []byte {
	b = append(b, 0, 0, 0)
	ip := from.Addr().Unmap()
	if ip.Is4() {
		v := ip.As4()
		b = append(b, addrIPv4)
		b = append(b, v[:]...)
	} else {
		v := ip.As16()
		b = append(b, addrIPv6)
		b = append(b, v[:]...)
	}
	return binary.BigEndian.AppendUint16(b, from.Port())
}

// declaredAddr переводит адрес из запроса UDP ASSOCIATE в AddrPort.
// Имя узла здесь бессмысленно - клиент объявляет свой адрес, - поэтому
// оно даёт пустой адрес, как и 0.0.0.0:0.
func declaredAddr(target string) netip.AddrPort {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return netip.AddrPort{}
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return netip.AddrPort{}
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.AddrPort{}
	}
	return netip.AddrPortFrom(ip, uint16(n))
}
