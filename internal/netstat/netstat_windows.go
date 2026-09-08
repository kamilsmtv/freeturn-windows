package netstat

import (
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func read(adapter string) Counters {
	luid, name, err := findAdapter(adapter)
	if err != nil {
		return Counters{Adapter: adapter, Reason: err.Error()}
	}

	row := windows.MibIfRow2{InterfaceLuid: luid}
	// Level 0 (MibIfEntryNormal) отдаёт статистику вместе с описанием.
	if err := windows.GetIfEntry2Ex(0, &row); err != nil {
		return Counters{Adapter: name, Reason: "не удалось прочитать счётчики адаптера"}
	}
	return Counters{
		Available: true,
		Adapter:   name,
		RxBytes:   row.InOctets,
		TxBytes:   row.OutOctets,
	}
}

// findAdapter ищет адаптер по имени, которое видит пользователь
// (FriendlyName), а при неудаче - по описанию.
func findAdapter(want string) (luid uint64, name string, err error) {
	list, err := adapters()
	if err != nil {
		return 0, "", err
	}
	target := strings.ToLower(strings.TrimSpace(want))

	for _, a := range list {
		friendly := windows.UTF16PtrToString(a.FriendlyName)
		desc := windows.UTF16PtrToString(a.Description)
		if strings.ToLower(friendly) == target || strings.ToLower(desc) == target {
			return a.Luid, friendly, nil
		}
	}
	// Точного совпадения нет: WireGuard называет адаптер по имени туннеля,
	// но пользователь мог указать его частично.
	for _, a := range list {
		friendly := windows.UTF16PtrToString(a.FriendlyName)
		if target != "" && strings.Contains(strings.ToLower(friendly), target) {
			return a.Luid, friendly, nil
		}
	}
	return 0, "", &adapterNotFound{name: want}
}

type adapterNotFound struct{ name string }

func (e *adapterNotFound) Error() string {
	return "адаптер «" + e.name + "» не найден: туннель не поднят"
}

// adapters возвращает список адаптеров системы.
func adapters() ([]*windows.IpAdapterAddresses, error) {
	const flags = windows.GAA_FLAG_SKIP_UNICAST | windows.GAA_FLAG_SKIP_ANYCAST |
		windows.GAA_FLAG_SKIP_MULTICAST | windows.GAA_FLAG_SKIP_DNS_SERVER

	size := uint32(15000)
	for attempt := 0; attempt < 4; attempt++ {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))

		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, flags, 0, first, &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			// size уже содержит нужный объём - пробуем ещё раз.
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
