# Анализ референсов

Источники (клонированы в `reference/`, в .gitignore):

- `samosvalishe/free-turn-proxy` — ядро. Лицензия: **Happy Bunny License (HBL)**.
- `samosvalishe/turn-proxy-android` — Android-клиент. Лицензия: **GPL-3.0**.

Всё ниже — из исходников, не из догадок. Спорные места помечены **TODO**.

---

## 1. Флаги клиента ядра (`cmd/client`, `internal/config/cliflags.go`)

| Флаг | Дефолт | Заметки |
|---|---|---|
| `-listen` | `127.0.0.1:9000` | локальный ip:port для WG (udp) / Xray (tcp) |
| `-peer` | обязателен | адрес VPS `host:port` |
| `-provider` | `vk` | единственный реализованный |
| `-link` | пусто | устарел, игнорируется при `-links` |
| `-links` | обяз. для vk | ссылки VK Calls через запятую |
| `-n` | `10` | параллельных TURN-потоков |
| `-transport` | `tcp` | `tcp` \| `udp` — транспорт до TURN |
| `-mode` | `udp` | `udp` (WireGuard) \| `tcp` (Xray/sing-box) |
| `-turn` / `-port` | из creds | ручной override TURN-сервера |
| `-obf-profile` | `none` | `none\|rtpopus\|rtpopus2\|rtpopus3` |
| `-obf-key` | пусто | 64 hex-символа (32 байта) |
| `-obf-timing` | `0` | `time.Duration`, напр. `20ms` |
| `-gen-obf-key` | false | печатает ключ и выходит |
| `-manual-captcha` | false | только vk |
| `-streams-per-cred` | `10` | только vk |
| `-platform` | `desktop` | `desktop\|mobile`, только vk |
| `-dns-mode` | `auto` | `plain\|doh\|auto` |
| `-dns-servers` | пусто | `ip[:port][,...]` |
| `-client-id` | авто | `^[0-9a-f]{32}$` (по валидатору Android) |
| `-sub` | пусто | URL подписки |
| `-routes` | false | **есть в коде (`cliflags.go:62`), но отсутствует в `docs/flags.md`**; авто-маршруты к TURN, требует админ-прав |
| `-debug` | false | |

KCP (только `-mode tcp`): `-kcp-nodelay 1`, `-kcp-interval 20`, `-kcp-resend 2`,
`-kcp-nc 1`, `-kcp-sndwnd 512`, `-kcp-rcvwnd 512`, `-kcp-mtu 1200` (300..1350),
`-kcp-acknodelay=true`. В `-mode udp` любое отличие от дефолта — фатальная ошибка старта.

Позиционный аргумент: `client "freeturn://..." -link "..."` — URI переопределяет флаги.

### Важные находки

1. **Флага `-version` нет.** `cmd/client/main.go:60` печатает в лог первой строкой:
   `Free Turn Proxy client version=<ver>` (ldflags `-X main.version`). Версию ядра
   определяем по этой строке, а не отдельным вызовом. **TODO для тебя:** подтверждаешь
   такой способ (запустить с заведомо неполным конфигом и прочитать первую строку)?
2. **`-config`-файла в CLI нет.** JSON-схема `config.ClientJSON` (`internal/config/json.go`)
   существует только для gomobile (Android/iOS). На Windows конфиг собираем аргументами
   командной строки; эталон сборки — `internal/config/args.go:ClientArgs` (порядок и
   правило «не писать флаг, равный дефолту»). Заметь: `ClientArgs` **не** эмитит `-routes`.
3. **Встроенного WireGuard-туннеля в CLI нет.** `tunnel.*` доступен только через gomobile.
   Значит VPN-режим на Windows = внешний WireGuard/AmneziaWG-клиент с
   `Endpoint = 127.0.0.1:9000`, `MTU = 1280` (константа сервера, `WG_MTU` в install.sh).
4. **Счётчиков трафика в stdout ядра нет.** `internal/stats` доступен только gomobile-API;
   в CLI формат байт печатается лишь в tcprelay per-connection. Следствие для GUI —
   см. раздел 6.

---

## 2. Схема `freeturn://` (`internal/uri/uri.go`, `docs/uri.md`)

`freeturn://` + base64url-без-padding(JSON). Версия `v=1`, чужая версия отвергается.

Поля: `v`, `provider`, `peer` (обязательные) и опциональные `transport`, `mode`,
`obf`, `key`, `n`, `spc`, `cid`, `listen`, `dns`, `dnss`, `mcap`, `kcp`
(`{nodelay,interval,resend,nc,sndwnd,rcvwnd,mtu,acknodelay}` — ключи строчные),
`name`, `wg` (WG-конфиг текстом). Пустые/дефолтные поля опускаются.
`obf`/`key` пишутся только когда профиль != `none`.

**Ссылка VK Calls в URI ядра не входит** — она у каждого клиента своя.
Но Android добавляет своё поле **`vk`** (`FreeturnLink.kt:53`) — ядро его молча игнорирует
(парсер не strict). Расширение поддерживаем на чтение и запись, отметим в README.

---

## 3. Модель конфига Android-клиента

`Server` (`data/server/Server.kt`) = `id`, `name`, `ssh: SshConfig`, `client: ClientConfig`,
`proxyListen` (`0.0.0.0:56000`), `proxyConnect` (`127.0.0.1:40537`), `opts: ServerOpts`.

- `ClientConfig`: `serverAddress`, `vkLink`, `provider`, `threads` (12),
  `streamsPerCred` (12; но декодер ServerJson подставляет 6 — расхождение в самом Android),
  `useUdp`, `manualCaptcha`, `localPort` (`127.0.0.1:9000`), `debugMode`, `useCarrierDns`,
  `dnsMode`, `customDns`, `syncServerSwitches`, `magicSwitch`, `magicTurn`,
  `tunnelTransport` (`none|wireguard`), `wireGuardConfig`, `wireGuardTunnelName`,
  `splitTunnelMode` (`all|include|exclude`), `splitTunnelApps`, `logsEnabled`, `clientId`.
- `ServerOpts`: `obfProfile`, `obfKey`, `obfTimingMs` (0..60, шаг 5), `proxyMode`, `kcp`.
- `SshConfig`: `ip`, `port` 22, `username` root, `password`, `authType` `PASSWORD|SSH_KEY`,
  `sshKey`, `hostFingerprint`, `rootMode` `ROOT|SUDO_NOPASS|SUDO_PASS`, `sudoPassword`.

Сериализация — `ServerJson` (плоский JSON, ключи = имена полей; контракт хранения).

### Формат бэкапа (`data/backup/`)

Открытый слой (`SettingsBackup`): `{"v":4,"servers":[...],"activeId","ownClientId",
"dynamicTheme","nerdMode","privacyMode","seasonalDecor","restartServerOnSwitch",
"hotspotProxy","suppressUpdatePrompt","suppressTgPrompt"}`. `ownClientId` обязателен и
валидируется по `^[0-9a-f]{32}$`.

Шифрование (`BackupCrypto`): конверт JSON
`{"magic":"freeturn-backup","v":1,"kdf":"pbkdf2-sha256","iter":210000,
"salt":b64(16B),"iv":b64(12B),"ct":b64}` — PBKDF2-HMAC-SHA256 210k итераций,
AES-256-GCM, тег 128 бит, base64 без переносов. **Воспроизводимо на Go 1:1.**

Расхождения, которые будут (и попадут в README): поля `dynamicTheme`, `seasonalDecor`,
`hotspotProxy`, `splitTunnelApps` (package-имена Android) на Windows смысла не имеют —
читаем и сохраняем как есть при round-trip, но не применяем.

---

## 4. Управление VPS

Android гоняет по SSH свой bundle `server-control/src/*.sh` (GPL-3.0) с протоколом v2:
один JSON-объект на запуск, `{"proto":2,"result":"ok"|"err","code","msg","stage","data","logs"}`.

**Тот же протокол уже встроен в `scripts/install.sh` самого ядра** (`install.sh:2580+`):
подкоманды `probe, install, wg-setup, start, stop, logs, share-info, share-list,
peer-add, peer-conf, peer-remove, client-add, client-remove, uninstall`; аргументы вида
`--listen= --connect= --mode= --obf-profile= --obf-key= --obf-timing= --tail= --port=
--endpoint= --name-b64= --pubkey= --client-id= --sha256= --dns= --target= --with-wg-pkg --dry-run`.

Вывод: **берём install.sh ядра (HBL), а не GPL-bundle Android** — та же функциональность
без лицензионного заражения. Парсер ответа повторяем по смыслу `ControlResponseParser`
(последняя строка, начинающаяся с `{`; префикс `ERROR:` = транспортная ошибка;
коды `sudo_auth_failed`, `sudo_requiretty`, `transport`).

---

## 5. Релизные ассеты (`.goreleaser.yaml`)

- Формат `binary` без архива, шаблон `{{.Binary}}-{{.Os}}-{{.Arch}}` →
  **`client-windows-amd64.exe`**, `server-windows-amd64.exe`, `client-linux-amd64`, …
- Чек-суммы: **`checksums.txt`** в ассетах релиза.
- Стабильный URL: `https://github.com/samosvalishe/free-turn-proxy/releases/latest/download/client-windows-amd64.exe`.
- Версия в бинарь идёт через `-X main.version={{.Version}}`; теги вида `vX.Y.Z`.

---

## 6. Следствия для Windows-клиента (то, что придётся ограничить)

1. **Счётчики трафика.** Ядро их не печатает. Реально доступное: счётчики
   WG-адаптера через `GetIfTable2` (IP Helper) в VPN-режиме. В proxy-режиме без адаптера
   честного источника нет — покажем «н/д». Альтернатива — считать по логам `-debug`
   (шумно и неточно), не предлагаю.
2. **Раздельное туннелирование по приложениям.** Аналога Android VpnService на Windows нет
   (WFP-драйвер — отдельный продукт). Делаем **по подсетям/маршрутам**: `AllowedIPs`
   WG-конфига + собственные `route add`. Ограничение опишем в README.
3. **VPN-режим.** Изначально предполагался внешний WireGuard, но это расходится с
   задумкой Android-версии (там туннель поднимает само ядро). Решение: GUI ведёт
   туннель сам - тот же `amneziawg-go`, что и в ядре, плюс адаптер Wintun; конфиг
   сервера разбирается приложением, `Endpoint` подменяется на локальный сокет ядра.
   Внешний клиент остаётся опцией для режима прокси (Xray, sing-box).
4. **Версия ядра** читается из первой строки лога (см. 1.1).

## 7. Типовые ошибки для маппинга (docs/troubleshooting.md + строки логов)

`no TURN candidates` / `all TURN candidates failed`, `get TURN creds`,
`connect to TURN server`, `failed to connect DTLS`, `udprelay listen …: address already in use`
(порт 9000 занят), `route manager disabled` (нет прав администратора),
`vk: no links configured`, зависание на creds = ссылка на звонок умерла,
`Fatal provider error`, несовпадение `-obf-profile`/`-obf-key` с сервером.
