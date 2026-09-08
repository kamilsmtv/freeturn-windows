package sub

import (
	"strings"
	"testing"

	"github.com/kamilsmtv/freeturn-windows/internal/link"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

func mkLink(t *testing.T, peer, name string) string {
	t.Helper()
	return link.Link{Provider: "vk", Peer: peer, Name: name}.Encode()
}

func TestParse(t *testing.T) {
	body := strings.Join([]string{
		"#name: Мои серверы",
		"#refresh: 6h",
		"#color: #4A90E2",
		"",
		mkLink(t, "1.2.3.4:56000", "из ссылки"),
		"##name: RU-1",
		"##ip: 1.2.3.4",
		"##comment: Основной",
		"",
		mkLink(t, "5.6.7.8:56000", ""),
		"##name: EU-Backup",
		"freeturn://мусор",
	}, "\n")

	s, err := Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Name != "Мои серверы" || s.Refresh != "6h" || s.Color != "#4A90E2" {
		t.Errorf("заголовок подписки разобран неверно: %+v", s)
	}
	if len(s.Nodes) != 2 {
		t.Fatalf("узлов = %d, want 2", len(s.Nodes))
	}
	if s.Skipped != 1 {
		t.Errorf("испорченная ссылка должна пропускаться со счётчиком, Skipped = %d", s.Skipped)
	}
	if s.Nodes[0].Name != "RU-1" || s.Nodes[0].IP != "1.2.3.4" || s.Nodes[0].Comment != "Основной" {
		t.Errorf("параметры узла разобраны неверно: %+v", s.Nodes[0])
	}
	if got := s.RefreshInterval().Hours(); got != 6 {
		t.Errorf("интервал обновления = %v ч, want 6", got)
	}
}

func TestParseNodeFieldsBeforeAnyNode(t *testing.T) {
	// "##" до первой ссылки применять некуда - ядро такие строки игнорирует.
	s, err := Parse(strings.NewReader("##name: сирота\n" + mkLink(t, "1.2.3.4:1", "")))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(s.Nodes) != 1 || s.Nodes[0].Name != "" {
		t.Errorf("осиротевшее поле не должно попадать в узел: %+v", s.Nodes)
	}
}

func TestProfiles(t *testing.T) {
	s, err := Parse(strings.NewReader(mkLink(t, "1.2.3.4:56000", "имя из ссылки") + "\n##name: имя из подписки"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := s.Profiles()
	if len(got) != 1 {
		t.Fatalf("профилей = %d", len(got))
	}
	// Имя владельца подписки важнее имени внутри ссылки.
	if got[0].Name != "имя из подписки" {
		t.Errorf("имя профиля = %q", got[0].Name)
	}
	if got[0].Client.ServerAddress != "1.2.3.4:56000" || got[0].Client.Provider != profile.ProviderVK {
		t.Errorf("параметры профиля разошлись: %+v", got[0].Client)
	}
}

func TestRefreshIntervalInvalid(t *testing.T) {
	s := &Sub{Refresh: "как-нибудь потом"}
	if s.RefreshInterval() != 0 {
		t.Error("негодный интервал должен давать 0")
	}
}
