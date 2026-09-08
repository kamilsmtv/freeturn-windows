// Package wg готовит конфигурацию WireGuard для работы поверх ядра.
//
// На Windows встроенного туннеля у клиента ядра нет (tunnel.* доступен
// только gomobile-сборке), поэтому VPN-режим - это внешний WireGuard или
// AmneziaWG, которому мы подсовываем Endpoint локального сокета ядра.
package wg

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// Режимы раздельного туннелирования. Аналога Android per-app на Windows нет
// (см. docs/analysis.md), поэтому делим трафик по подсетям.
const (
	SplitAll     = "all"
	SplitInclude = "include"
	SplitExclude = "exclude"
)

// DefaultAllowedIPs - весь трафик в туннель.
var DefaultAllowedIPs = []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}

// AllowedIPs считает список подсетей для поля AllowedIPs.
//
//   - all: весь трафик (0.0.0.0/0);
//   - include: только перечисленные подсети;
//   - exclude: всё, кроме перечисленных - как это делает калькулятор
//     wg-quick: 0.0.0.0/0 разбивается на минимальный набор префиксов,
//     не пересекающихся с исключениями.
func AllowedIPs(mode, subnets string) ([]netip.Prefix, error) {
	list, err := ParseSubnets(subnets)
	if err != nil {
		return nil, err
	}

	switch mode {
	case SplitInclude:
		if len(list) == 0 {
			return nil, errors.New("в режиме «только выбранные» нужно указать хотя бы одну подсеть")
		}
		return compact(list), nil

	case SplitExclude:
		if len(list) == 0 {
			return DefaultAllowedIPs, nil
		}
		return excludeFrom(netip.MustParsePrefix("0.0.0.0/0"), compact(list)), nil

	default:
		return DefaultAllowedIPs, nil
	}
}

// ParseSubnets разбирает список подсетей, разделённых запятой, пробелом или
// переводом строки. Одиночный адрес считается /32.
func ParseSubnets(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, raw := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\r' || r == '\t' || r == ';'
	}) {
		p, err := parseOne(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func parseOne(raw string) (netip.Prefix, error) {
	if strings.Contains(raw, "/") {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("не удалось разобрать подсеть %q", raw)
		}
		if !p.Addr().Is4() {
			return netip.Prefix{}, fmt.Errorf("поддерживаются только адреса IPv4, а %q - нет", raw)
		}
		return p.Masked(), nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil || !addr.Is4() {
		return netip.Prefix{}, fmt.Errorf("не удалось разобрать адрес %q", raw)
	}
	return netip.PrefixFrom(addr, 32), nil
}

// excludeFrom вычитает из base список подсетей, возвращая минимальный
// набор префиксов. Работает рекурсивным делением пополам: если подсеть
// пересекается с исключением, делим её и повторяем для половин.
func excludeFrom(base netip.Prefix, exclusions []netip.Prefix) []netip.Prefix {
	// Полностью съеденная подсеть не даёт ничего.
	for _, ex := range exclusions {
		if ex.Bits() <= base.Bits() && ex.Contains(base.Addr()) {
			return nil
		}
	}
	// Ни одно исключение не пересекается - подсеть уходит целиком.
	if !overlapsAny(base, exclusions) {
		return []netip.Prefix{base}
	}
	// Дальше делить некуда: /32 либо исключён (обработано выше), либо цел.
	if base.Bits() >= 32 {
		return nil
	}

	left, right := split(base)
	return append(excludeFrom(left, exclusions), excludeFrom(right, exclusions)...)
}

func overlapsAny(p netip.Prefix, list []netip.Prefix) bool {
	for _, other := range list {
		if p.Overlaps(other) {
			return true
		}
	}
	return false
}

// split делит подсеть на две половины на один бит длиннее.
func split(p netip.Prefix) (netip.Prefix, netip.Prefix) {
	bits := p.Bits() + 1
	left := netip.PrefixFrom(p.Addr(), bits)

	// Правая половина начинается с адреса, у которого поднят добавленный бит.
	octets := p.Addr().As4()
	octets[(bits-1)/8] |= 1 << (7 - (bits-1)%8)
	right := netip.PrefixFrom(netip.AddrFrom4(octets), bits)
	return left, right
}

// compact убирает дубли и подсети, поглощённые более широкими соседями.
func compact(list []netip.Prefix) []netip.Prefix {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Bits() != list[j].Bits() {
			return list[i].Bits() < list[j].Bits()
		}
		return list[i].Addr().Less(list[j].Addr())
	})

	var out []netip.Prefix
	for _, p := range list {
		covered := false
		for _, kept := range out {
			if kept.Bits() <= p.Bits() && kept.Contains(p.Addr()) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, p)
		}
	}
	return out
}

// FormatPrefixes собирает список для строки AllowedIPs.
func FormatPrefixes(list []netip.Prefix) string {
	parts := make([]string, 0, len(list))
	for _, p := range list {
		parts = append(parts, p.String())
	}
	return strings.Join(parts, ", ")
}
