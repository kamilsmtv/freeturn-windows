#!/usr/bin/env bash
# Снимки экранов для README: берём собранный фронтенд как есть и подставляем
# вместо бэкенда Wails демонстрационные данные (demo.js). Так на картинках
# настоящий интерфейс, а не макет, и пересобрать их можно одной командой.
#
# Нужен headless-браузер на базе Chromium: путь передаётся в CHROME.
# Пример: CHROME=~/.cache/ms-playwright/chromium_headless_shell-1228/chrome-headless-shell-linux64/chrome-headless-shell ./design/screenshots/make.sh
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
chrome=${CHROME:-google-chrome}
port=${PORT:-8731}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

[ -f "$root/frontend/dist/index.html" ] || { echo "сначала соберите фронтенд: cd frontend && npm run build" >&2; exit 1; }

cp -r "$root/frontend/dist" "$work/site"
cp "$root/design/screenshots/demo.js" "$work/site/demo.js"
# Подставной бэкенд должен объявиться до модуля приложения.
sed -i 's|<script type="module"|<script src="/demo.js"></script>\n    <script type="module"|' "$work/site/index.html"

# Сервер запускаем напрямую, без подоболочки: иначе `kill $!` убивает её,
# а python остаётся держать порт, и следующий запуск снимает 404.
python3 -m http.server "$port" --directory "$work/site" >/dev/null 2>&1 &
server=$!
trap 'kill "$server" 2>/dev/null; wait "$server" 2>/dev/null; rm -rf "$work"' EXIT

for _ in $(seq 30); do
  curl -sf -o /dev/null "http://127.0.0.1:$port/" && break
  sleep 0.2
done
curl -sf -o /dev/null "http://127.0.0.1:$port/" || { echo "сервер снимков не поднялся на порту $port" >&2; exit 1; }

shoot() { # индекс раздела в левой панели -> имя файла
  "$chrome" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
    --hide-scrollbars --force-device-scale-factor=2 --window-size=1100,760 \
    --virtual-time-budget=5000 --user-data-dir="$work/chrome" \
    --screenshot="$work/$2.png" "http://127.0.0.1:$port/?screen=$1" >/dev/null 2>&1
  python3 - "$work/$2.png" "$root/docs/screenshots/$2.png" <<'PY'
import sys
from PIL import Image
src, dst = sys.argv[1], sys.argv[2]
im = Image.open(src).convert("RGB")
im.resize((1400, round(im.height * 1400 / im.width)), Image.LANCZOS).save(dst, optimize=True, quality=92)
PY
  echo "docs/screenshots/$2.png"
}

shoot 0 home
shoot 1 server
shoot 2 log
shoot 4 settings
