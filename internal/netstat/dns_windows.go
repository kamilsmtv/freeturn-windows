package netstat

import (
	"errors"
	"net/netip"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func systemDNS(excludeAdapter string) []string {
	list, err := adaptersWithDNS()
	if err != nil {
		return nil
	}

	exclude := strings.ToLower(strings.TrimSpace(excludeAdapter))
	seen := map[string]bool{}
	var out []string

	for _, a := range list {
		if a.OperStatus != windows.IfOperStatusUp {
			continue
		}
		name := strings.ToLower(windows.UTF16PtrToString(a.FriendlyName))
		// Адаптер туннеля пропускаем: его DNS доступен только через туннель.
		if exclude != "" && strings.Contains(name, exclude) {
			continue
		}

		for dns := a.FirstDnsServerAddress; dns != nil; dns = dns.Next {
			raw := dns.Address.IP()
			if raw == nil {
				continue
			}
			ip, ok := netip.AddrFromSlice(raw.To4())
			if !ok || !ip.Is4() || ip.IsUnspecified() {
				continue
			}
			s := ip.String()
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// adaptersWithDNS - список адаптеров вместе с их DNS-серверами.
func adaptersWithDNS() ([]*windows.IpAdapterAddresses, error) {
	const flags = windows.GAA_FLAG_SKIP_ANYCAST | windows.GAA_FLAG_SKIP_MULTICAST

	size := uint32(15000)
	for attempt := 0; attempt < 4; attempt++ {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))

		err := windows.GetAdaptersAddresses(windows.AF_INET, flags, 0, first, &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if err != nil {
			return nil, err
		}

		var out []*windows.IpAdapterAddresses
		for a := first; a != nil; a = a.Next {
			out = append(out, a)
		}
		return out, nil
	}
	return nil, windows.ERROR_BUFFER_OVERFLOW
}
