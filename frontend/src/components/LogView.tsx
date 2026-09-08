import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { backend, EVENTS, onEvent, type LogLine } from "../lib/api";
import { guard } from "../lib/effect";
import { Spinner } from "./ui";

const LEVELS = [
  { value: "all", label: "Все" },
  { value: "info", label: "Инфо" },
  { value: "warn", label: "Предупреждения" },
  { value: "error", label: "Ошибки" },
] as const;

const LEVEL_STYLE: Record<LogLine["level"], string> = {
  info: "text-zinc-700 dark:text-zinc-300",
  warn: "text-amber-600 dark:text-amber-400",
  error: "text-red-600 dark:text-red-400",
  debug: "text-zinc-400 dark:text-zinc-500",
};

export function LogView() {
  const [lines, setLines] = useState<LogLine[]>([]);
  const [level, setLevel] = useState<(typeof LEVELS)[number]["value"]>("all");
  const [query, setQuery] = useState("");
  const [follow, setFollow] = useState(true);
  const [saved, setSaved] = useState("");
  const [saving, setSaving] = useState(false);
  const logBox = useRef<HTMLDivElement>(null);

  useEffect(() => guard("LogView/subscribe", () => {
    backend()?.CoreLog().then((l) => setLines(l ?? []));
    return onEvent<LogLine>(EVENTS.coreLog, (l) =>
      // Хвост держим ограниченным: полный журнал живёт на стороне ядра.
      setLines((prev) => (prev.length > 5000 ? [...prev.slice(-4000), l] : [...prev, l])),
    );
  }), []);

  const shown = useMemo(() => {
    const q = query.trim().toLowerCase();
    return lines.filter((l) => {
      if (level === "error" && l.level !== "error") return false;
      if (level === "warn" && l.level !== "warn" && l.level !== "error") return false;
      if (level === "info" && l.level === "debug") return false;
      return !q || l.text.toLowerCase().includes(q);
    });
  }, [lines, level, query]);

  useEffect(() => {
    if (!follow) return;
    // Прокручиваем сам блок журнала, иначе уезжает вся страница.
    const box = logBox.current;
    if (box) box.scrollTop = box.scrollHeight;
  }, [shown.length, follow]);

  const asText = () => shown.map((l) => `${l.time} ${l.text}`).join("\n");

  return (
    <div className="flex h-full flex-col gap-3.5">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-xl font-semibold">Журнал ядра</h1>
        <span className="text-xs text-zinc-400 dark:text-zinc-600">
          {query || level !== "all" ? `показано ${shown.length} из ${lines.length}` : `${lines.length} строк`}
        </span>
        <span className="h-px flex-1 bg-zinc-200 dark:bg-zinc-800" />

        <div className="flex h-8 items-center gap-2 rounded-lg border border-zinc-200 bg-white px-3 dark:border-zinc-800 dark:bg-zinc-900">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" className="text-zinc-400">
            <circle cx="11" cy="11" r="6.5" />
            <path d="M16 16l4 4" />
          </svg>
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Поиск по тексту"
            className="w-40 bg-transparent text-[13px] outline-none"
          />
        </div>

        {LEVELS.map((l) => (
          <Chip key={l.value} active={level === l.value} onClick={() => setLevel(l.value)}>
            {l.label}
          </Chip>
        ))}

        <Chip active={follow} onClick={() => setFollow(!follow)}>
          Прокрутка
        </Chip>
        <Chip onClick={() => navigator.clipboard.writeText(asText())}>Копировать</Chip>
        <Chip
          busy={saving}
          onClick={async () => {
            setSaving(true);
            try {
              const path = await backend()?.ExportLog();
              if (path) setSaved(path);
            } catch (e) {
              setSaved(String(e));
            } finally {
              setSaving(false);
            }
          }}
        >
          Сохранить
        </Chip>
        <Chip
          onClick={() => {
            backend()?.CoreClearLog();
            setLines([]);
          }}
        >
          Очистить
        </Chip>
      </div>

      {saved && <p className="text-xs text-zinc-500 dark:text-zinc-400">{saved}</p>}

      <div
        ref={logBox}
        className="scroll selectable min-h-0 flex-1 overflow-y-auto rounded-2xl border border-zinc-200 bg-white p-4 font-mono text-xs leading-relaxed dark:border-zinc-800 dark:bg-[#0c0c0e]"
      >
        {shown.length === 0 && (
          <div className="flex h-full flex-col items-center justify-center gap-1 text-center">
            <span className="font-sans text-sm text-zinc-500 dark:text-zinc-400">
              {lines.length === 0 ? "Ядро не запущено" : "Ничего не найдено"}
            </span>
            <span className="max-w-sm font-sans text-xs text-zinc-400 dark:text-zinc-600">
              {lines.length === 0
                ? "Журнал наполнится, как только вы подключитесь на главном экране"
                : "Смягчите фильтр или измените запрос"}
            </span>
          </div>
        )}

        {shown.map((l) => (
          <div key={l.seq} className="whitespace-pre-wrap break-all">
            <span className="mr-2 text-zinc-400 dark:text-zinc-600">{l.time}</span>
            <span className={LEVEL_STYLE[l.level]}>{l.text}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

/** Чип фильтра из макета: 32px, скругление 8, подсветка активного. */
function Chip({
  children,
  onClick,
  active,
  busy,
}: {
  children: ReactNode;
  onClick: () => void;
  active?: boolean;
  busy?: boolean;
}) {
  return (
    <button
      onClick={onClick}
      disabled={busy}
      className={`flex h-8 shrink-0 items-center gap-2 rounded-lg border px-3.5 text-[13px] transition disabled:opacity-40 ${
        active
          ? "border-emerald-500 bg-emerald-50 text-emerald-700 dark:bg-[#0f1a15] dark:text-emerald-300"
          : "border-zinc-200 bg-white text-zinc-700 hover:bg-zinc-100 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800"
      }`}
    >
      {busy && <Spinner size={13} />}
      {children}
    </button>
  );
}
