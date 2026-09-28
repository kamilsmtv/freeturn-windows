// Команда livecheck прогоняет протокол install.sh по настоящему серверу.
//
// Нужна там, где юнит-тесты бессильны: проверить, что мы действительно
// говорим с сервером на одном языке. В сборку приложения не входит.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/link"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
	"github.com/kamilsmtv/freeturn-windows/internal/vps"
)

func main() {
	c := vps.New(os.TempDir(), func(line string) { fmt.Println("[log]", line) })
	ssh := profile.SSH{
		IP: os.Getenv("FT_IP"), Port: port(),
		Username: os.Getenv("FT_USER"), Password: os.Getenv("FT_PASS"),
		AuthType: profile.AuthPassword, RootMode: profile.RootSudoPass,
		SudoPassword:    os.Getenv("FT_PASS"),
		HostFingerprint: os.Getenv("FT_FP"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()

	show := func(name string, v any, resp vps.Response, err error) {
		fmt.Println("=====", name)
		switch {
		case err != nil:
			fmt.Println("ОШИБКА:", err)
		case !resp.OK():
			fmt.Println("отказ:", vps.Explain(resp))
		default:
			j, _ := json.MarshalIndent(v, "", "  ")
			fmt.Println(string(j))
		}
	}

	for _, step := range strings.Split(os.Getenv("FT_STEPS"), ",") {
		switch strings.TrimSpace(step) {
		case "probe":
			d, r, err := c.Probe(ctx, ssh)
			show("probe", d, r, err)
		case "apply":
			d, r, err := c.Apply(ctx, ssh, vps.ApplyOptions{
				Backend: vps.BackendNew, Host: ssh.IP, ListenPort: 56000,
				Mode: profile.ModeUDP, ObfProfile: profile.ObfRtpOpus3,
			})
			// Целиком не печатаем: в конфигурации и ссылке приватные ключи.
			d.Owner.ConfB64 = short(vps.DecodeBase64(d.Owner.ConfB64))
			d.Owner.Link = short(d.Owner.Link)
			d.ObfKey = short(d.ObfKey)
			show("apply", d, r, err)
		case "clients":
			d, r, err := c.ClientList(ctx, ssh)
			d.Share.ObfKey = short(d.Share.ObfKey)
			show("client-list", d, r, err)
		case "add":
			d, r, err := c.ClientAdd(ctx, ssh, "phone")
			d.ConfB64 = short(vps.DecodeBase64(d.ConfB64))
			d.Link = short(d.Link)
			show("client-add", d, r, err)
		case "conf":
			who := os.Getenv("FT_NAME")
			if who == "" {
				who = vps.OwnerName
			}
			d, r, err := c.ClientConf(ctx, ssh, who)
			if err == nil && r.OK() {
				if l, perr := link.Parse(d.Link); perr == nil {
					fmt.Printf("ссылка: peer=%s obf=%s cid=%s wg=%v (%d символов конфига)\n",
						l.Peer, l.Obf, l.ClientID, l.WGConf != "", len(l.WGConf))
				} else {
					fmt.Println("ссылку разобрать не вышло:", perr)
				}
			}
			d.ConfB64 = short(vps.DecodeBase64(d.ConfB64))
			d.Link = short(d.Link)
			show("client-conf", d, r, err)
		case "parse":
			// Ссылку собирает сервер, а разбирает её наш парсер: если он
			// её не поймёт, приложение молча соберёт свою - и разойдётся
			// с сервером в параметрах.
			d, r, err := c.ClientAdd(ctx, ssh, "parsecheck")
			if err != nil || !r.OK() {
				show("client-add (parse)", nil, r, err)
				break
			}
			l, perr := link.Parse(d.Link)
			if perr != nil {
				fmt.Println("===== parse\nОШИБКА разбора ссылки сервера:", perr)
				break
			}
			l.Key = short(l.Key)
			show("parse", l, r, nil)
			if _, rerr := c.ClientRemove(ctx, ssh, "parsecheck"); rerr != nil {
				fmt.Println("убрать проверочный доступ не вышло:", rerr)
			}
		case "external":
			d, r, err := c.Apply(ctx, ssh, vps.ApplyOptions{
				Backend: vps.BackendExternal, Connect: "127.0.0.1:443",
				Host: ssh.IP, ListenPort: 56000, Mode: profile.ModeTCP,
				ObfProfile: profile.ObfRtpOpus3,
			})
			d.Owner.ConfB64, d.Owner.Link = short(vps.DecodeBase64(d.Owner.ConfB64)), short(d.Owner.Link)
			d.ObfKey = short(d.ObfKey)
			show("apply external", d, r, err)
		case "dropwg":
			r, err := c.Uninstall(ctx, ssh, vps.TargetWG, false)
			show("uninstall --target=wg", "ok", r, err)
		case "badname":
			d, r, err := c.ClientAdd(ctx, ssh, os.Getenv("FT_NAME"))
			d.ConfB64, d.Link = "", short(d.Link)
			show("client-add (имя "+os.Getenv("FT_NAME")+")", d, r, err)
		case "remove":
			name := os.Getenv("FT_NAME")
			if name == "" {
				name = "phone"
			}
			r, err := c.ClientRemove(ctx, ssh, name)
			show("client-remove "+name, "ok", r, err)
		case "logs":
			text, r, err := c.Logs(ctx, ssh, 12)
			show("logs", text, r, err)
		case "restart":
			r, err := c.Restart(ctx, ssh)
			show("restart", "ok", r, err)
		case "stop":
			r, err := c.Stop(ctx, ssh)
			show("stop", "ok", r, err)
		case "uninstall":
			r, err := c.Uninstall(ctx, ssh, "all", true)
			show("uninstall", "ok", r, err)
		}
	}
}

// port - порт SSH из окружения; пусто - обычный 22.
func port() int {
	if v, err := strconv.Atoi(os.Getenv("FT_PORT")); err == nil && v > 0 {
		return v
	}
	return 22
}

// short прячет секреты: в конфигурации и ссылке есть приватные ключи.
func short(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 20 {
		return s
	}
	return fmt.Sprintf("%s…(%d символов)", s[:20], len(s))
}
