package netstat

import (
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

// virtualMarks - куски имён адаптеров, которые заводят себе виртуальные
// машины и контейнеры. Их адреса тоже частные, но ведут они внутрь этой
// же машины, и соседу по сети от них никакого толку.
var virtualMarks = []string{
	"vethernet", "wsl", "hyper-v", "virtualbox", "vmware", "vmnet",
	"docker", "loopback", "tailscale", "zerotier", "bluetooth", "teredo",
}

// LANAddrs собирает адреса, по которым сосед по локальной сети достучится
// до сокета, открытого на listen.
//
// Нужно это затем, что 0.0.0.0:1080 человеку передать нельзя, а искать
// свой адрес через ipconfig - лишний шаг ровно там, где он и бросит
// настройку. Возвращаются только частные адреса: публичный адрес
// интерфейса раздавать соседям смысла нет, а показывать его опасно.
//
// exclude - имя адаптера туннеля: его адрес тоже частный, но снаружи
// недоступен, и в списке он только сбивает с толку.
func LANAddrs(listen, exclude string) []string {
	if listen == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return nil
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return nil
	}

	// Сокет на конкретном адресе никаких подсказок не требует: он и так
	// написан в настройках ровно так, как его набирать.
	if addr, err := netip.ParseAddr(host); err == nil && !addr.IsUnspecified() {
		return nil
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if skipIface(iface.Name, exclude) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipnet.IP)
			if !ok {
				continue
			}
			ip = ip.Unmap()
			// IPv6 в локальной сети раздавать нечем: адреса link-local
			// требуют зоны, а глобальный - это уже не локальная сеть.
			if !ip.Is4() || !ip.IsPrivate() || ip.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, net.JoinHostPort(ip.String(), port))
		}
	}

	return order(out)
}

// order ставит первым тот адрес, который вероятнее всего и нужен соседу:
// в интерфейсе виден только он. Домашние и офисные сети почти всегда
// 192.168.x.x, поэтому начинаем с них.
func order(list []string) []string {
	sort.SliceStable(list, func(i, j int) bool {
		ri, rj := rank(list[i]), rank(list[j])
		if ri != rj {
			return ri < rj
		}
		return list[i] < list[j]
	})
	return list
}

// skipIface отсеивает туннель и виртуальные адаптеры.
func skipIface(name, exclude string) bool {
	if exclude != "" && strings.EqualFold(name, exclude) {
		return true
	}
	low := strings.ToLower(name)
	for _, mark := range virtualMarks {
		if strings.Contains(low, mark) {
			return true
		}
	}
	return false
}

// rank расставляет диапазоны по тому, насколько вероятно встретить в них
// обычную домашнюю или офисную сеть.
func rank(addr string) int {
	switch {
	case strings.HasPrefix(addr, "192.168."):
		return 0
	case strings.HasPrefix(addr, "10."):
		return 1
	default:
		// 172.16.0.0/12 - любимый диапазон докеров и виртуалок.
		return 2
	}
}
