// Package sub загружает и разбирает подписки на серверы.
//
// Формат повторяет internal/sub ядра (docs/sub.md): текстовый файл, где
// строки "#ключ: значение" - параметры подписки, "##ключ: значение" -
// параметры предыдущего узла, а сами узлы задаются ссылками freeturn://.
package sub

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kamilsmtv/freeturn-windows/internal/link"
	"github.com/kamilsmtv/freeturn-windows/internal/profile"
)

// Sub - разобранная подписка.
type Sub struct {
	Name      string `json:"name"`
	Update    string `json:"update"`
	Refresh   string `json:"refresh"`
	Color     string `json:"color"`
	Icon      string `json:"icon"`
	Used      string `json:"used"`
	Available string `json:"available"`
	Nodes     []Node `json:"nodes"`
	// Skipped - сколько ссылок не удалось разобрать (ядро их тоже пропускает).
	Skipped int `json:"skipped"`
}

// Node - узел подписки.
type Node struct {
	Link      link.Link `json:"link"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Icon      string    `json:"icon"`
	Used      string    `json:"used"`
	Available string    `json:"available"`
	IP        string    `json:"ip"`
	Comment   string    `json:"comment"`
}

// Fetch скачивает и разбирает подписку.
func Fetch(ctx context.Context, url string) (*Sub, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("неверный адрес подписки: %w", err)
	}
	req.Header.Set("User-Agent", "FreeTurn-Windows")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("не удалось загрузить подписку: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("подписка ответила %s", resp.Status)
	}
	// Ограничение на размер: подписка - текстовый файл, мегабайта хватает с запасом.
	return Parse(io.LimitReader(resp.Body, 1<<20))
}

// Parse разбирает содержимое подписки.
func Parse(r io.Reader) (*Sub, error) {
	s := &Sub{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		switch {
		// Порядок проверок важен: "##" - это тоже "#".
		case strings.HasPrefix(line, "##"):
			if len(s.Nodes) == 0 {
				continue
			}
			key, val := keyValue(line, 2)
			applyNodeField(&s.Nodes[len(s.Nodes)-1], key, val)

		case strings.HasPrefix(line, "#"):
			key, val := keyValue(line, 1)
			applySubField(s, key, val)

		case link.LooksLike(line):
			l, err := link.Parse(line)
			if err != nil {
				s.Skipped++
				continue
			}
			s.Nodes = append(s.Nodes, Node{Link: l, Name: l.Name})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return s, nil
}

// keyValue разбирает строку вида "#ключ: значение", отбросив prefix символов.
func keyValue(line string, prefix int) (string, string) {
	parts := strings.SplitN(line[prefix:], ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func applySubField(s *Sub, key, val string) {
	switch key {
	case "name":
		s.Name = val
	case "update":
		s.Update = val
	case "refresh":
		s.Refresh = val
	case "color":
		s.Color = val
	case "icon":
		s.Icon = val
	case "used":
		s.Used = val
	case "available":
		s.Available = val
	}
}

func applyNodeField(n *Node, key, val string) {
	switch key {
	case "name":
		n.Name = val
	case "color":
		n.Color = val
	case "icon":
		n.Icon = val
	case "used":
		n.Used = val
	case "available":
		n.Available = val
	case "ip":
		n.IP = val
	case "comment":
		n.Comment = val
	}
}

// Profiles превращает узлы подписки в профили.
func (s *Sub) Profiles() []profile.Profile {
	out := make([]profile.Profile, 0, len(s.Nodes))
	for _, n := range s.Nodes {
		p := n.Link.ToProfile()
		// Имя из "##name" важнее имени внутри ссылки: его задаёт владелец подписки.
		if n.Name != "" {
			p.Name = n.Name
		}
		if p.Name == "" || p.Name == profile.FallbackName {
			p.Name = p.Client.ServerAddress
		}
		out = append(out, p)
	}
	return out
}

// RefreshInterval разбирает "#refresh" (например "6h"); 0 - не задан или негоден.
func (s *Sub) RefreshInterval() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(s.Refresh))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}
