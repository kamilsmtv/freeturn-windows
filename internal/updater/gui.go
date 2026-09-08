package updater

import (
	"context"
	"time"
)

// GUIStatus - результат проверки обновлений самого приложения.
// Самозамены .exe нет: показываем уведомление со ссылкой на релиз.
type GUIStatus struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	UpdateReady bool   `json:"updateReady"`
	Changelog   string `json:"changelog"`
	ReleaseURL  string `json:"releaseUrl"`
	CheckedAt   string `json:"checkedAt"`
	Error       string `json:"error"`
}

// CheckGUI сравнивает текущую версию GUI с последним релизом его репозитория.
func CheckGUI(ctx context.Context, gh *GitHub, repo, current string, force bool) GUIStatus {
	st := GUIStatus{Current: NormalizeVersion(current), CheckedAt: time.Now().Format(time.RFC3339)}

	rel, err := gh.LatestRelease(ctx, repo, force)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.Latest = NormalizeVersion(rel.TagName)
	st.Changelog = rel.Body
	st.ReleaseURL = rel.HTMLURL
	// Сборка "dev" - не релиз: не уговариваем разработчика обновиться.
	st.UpdateReady = st.Current != "" && st.Current != "dev" && IsNewer(st.Latest, st.Current)
	return st
}
