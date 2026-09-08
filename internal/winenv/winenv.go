// Package winenv отвечает на вопросы об окружении Windows: есть ли права
// администратора и установлен ли runtime WebView2.
package winenv

// Report - снимок окружения для стартовой проверки в UI.
type Report struct {
	Admin           bool   `json:"admin"`
	WebView2        bool   `json:"webView2"`
	WebView2Version string `json:"webView2Version"`
	Windows         bool   `json:"windows"`
}

// WebView2DownloadURL - официальный Evergreen-бутстраппер Microsoft.
const WebView2DownloadURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

// Check собирает отчёт об окружении.
func Check() Report {
	return Report{
		Admin:           IsAdmin(),
		WebView2:        webView2Version() != "",
		WebView2Version: webView2Version(),
		Windows:         isWindows,
	}
}
