package main

import (
	"errors"
	"strings"

	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/settings"
	"github.com/kamilsmtv/freeturn-windows/internal/wg"
)

// SplitShared - общие правила раздельного туннелирования для интерфейса.
type SplitShared struct {
	// Exists - набор уже заведён. Пока его нет, включение общих правил
	// на профиле создаёт набор из собственных правил этого профиля.
	Exists  bool   `json:"exists"`
	Mode    string `json:"mode"`
	Subnets string `json:"subnets"`
	// Users и Total - сколько профилей пользуются набором и сколько всего:
	// правка общих правил затронет их все, и об этом надо сказать.
	Users int `json:"users"`
	Total int `json:"total"`
}

// GetSplitShared возвращает общие правила и сколько профилей на них.
func (a *App) GetSplitShared() SplitShared {
	snap := a.profiles.All()
	out := SplitShared{Total: len(snap.List)}
	for _, p := range snap.List {
		if p.Client.SplitTunnelShared {
			out.Users++
		}
	}
	if r := a.settings.Get().SplitShared; r != nil {
		out.Exists = true
		out.Mode = r.Mode
		out.Subnets = r.Subnets
	}
	return out
}

// SetSplitShared сохраняет общие правила. Подсети проверяются сразу:
// иначе ошибка всплыла бы только при подключении, и сразу во всех
// профилях, что на них ссылаются.
func (a *App) SetSplitShared(mode, subnets string) error {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "all"
	}
	if _, err := wg.AllowedIPs(mode, subnets); err != nil {
		return errors.New("общие правила: " + err.Error())
	}
	s := a.settings.Get()
	s.SplitShared = &settings.SplitRules{Mode: mode, Subnets: strings.TrimSpace(subnets)}
	return a.settings.Save(s)
}

// effectiveSplit возвращает режим и подсети, которые действуют для профиля:
// общие, если профиль на них подписан и набор заведён, иначе свои.
func (a *App) effectiveSplit(p profile.Profile) (mode, subnets string) {
	return splitFor(p, a.settings.Get().SplitShared)
}

// splitFor - правило выбора без обращения к хранилищу. Если профиль
// подписан на общий набор, а набора нет (например, его не было в
// восстановленном бэкапе), остаются свои правила профиля: молча пустить
// весь трафик в обход было бы хуже.
func splitFor(p profile.Profile, shared *settings.SplitRules) (mode, subnets string) {
	if p.Client.SplitTunnelShared && shared != nil {
		return shared.Mode, shared.Subnets
	}
	return p.Client.SplitTunnelMode, p.Client.SplitTunnelSubnets
}
