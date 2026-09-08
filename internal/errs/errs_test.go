package errs

import (
	"errors"
	"strings"
	"testing"
)

func TestExplain(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name:  "порт занят",
			lines: []string{"[ERROR] udprelay listen 127.0.0.1:9000: bind: Only one usage of each socket address"},
			want:  "порт занят",
		},
		{
			name:  "нет прав на маршруты",
			lines: []string{"[WARN] route manager disabled: access is denied"},
			want:  "права администратора",
		},
		{
			name:  "TURN недоступен",
			lines: []string{"[ERROR] no TURN candidates"},
			want:  "TURN-сервер недоступен",
		},
		{
			name:  "звонок умер",
			lines: []string{"[ERROR] vk provider [0]: get TURN creds: unexpected status 404"},
			want:  "ссылка на звонок истекла",
		},
		{
			name:  "обфускация не сошлась",
			lines: []string{"[ERROR] [STREAM 1] OBF unwrap failed: auth error (n=120)"},
			want:  "профиль и ключ обязаны совпадать",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Explain(tt.lines, errors.New("exit status 1"))
			if !strings.Contains(got, tt.want) {
				t.Errorf("Explain = %q, ожидалось упоминание %q", got, tt.want)
			}
		})
	}
}

func TestExplainFallsBackToLastError(t *testing.T) {
	got := Explain([]string{"[INFO] provider=vk", "[ERROR] что-то своё"}, nil)
	if !strings.Contains(got, "что-то своё") {
		t.Errorf("без совпадения правил ожидалась последняя строка ERROR, получено %q", got)
	}
}

func TestExplainWithoutLines(t *testing.T) {
	if got := Explain(nil, errors.New("exit status 2")); !strings.Contains(got, "exit status 2") {
		t.Errorf("ожидался код завершения, получено %q", got)
	}
}
