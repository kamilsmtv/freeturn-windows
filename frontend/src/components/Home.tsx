import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  backend,
  type CoreStatus,
  type Profile,
  type Snapshot,
  type Traffic,
} from "../lib/api";
import { guard } from "../lib/effect";
import { isVPN, runningProfile, type Phase } from "../lib/phase";
import { bytes, uptime } from "../lib/format";
import { ImportDialog, ShareDialog } from "./Transfer";

/** Порог, с которого список перестаёт читаться взглядом и нужен поиск. */
const SEARCH_FROM = 6;

/**
 * Главный экран: одна большая кнопка, под ней состояние, внизу — список
 * профилей. Устроен как в Android-клиенте: список всегда на виду и
 * разворачивается на весь экран, когда профилей много.
 */
export function Home({
  core,
  snap,
  phase,
  onEditProfile,
}: {
  core: CoreStatus | null;
  snap: Snapshot;
  phase: Phase;
  onEditProfile: (p: Profile) => void;
}) {
  const [traffic, setTraffic] = useState<Traffic | null>(null);
  const [query, setQuery] = useState("");
  const [expanded, setExpanded] = useState(false);
  const [importing, setImporting] = useState(false);
  const [sharing, setSharing] = useState<Profile | null>(null);
  const [error, setError] = useState("");
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const [, tick] = useState(0);

  const api = backend();
  const running = core?.state === "running" || core?.state === "starting";

  useEffect(() => guard("Home/traffic", () => {
    if (!running) {
      setTraffic(null);
      return;
    }
    const read = () => {
      api?.TrafficStats().then(setTraffic);
      tick((v) => v + 1);
    };
    read();
    const id = setInterval(read, 2000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [running]);

  const run = async (fn: () => Promise<unknown>) => {
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(String(e));
    }
  };

  const active = snap.list.find((p) => p.id === snap.activeId) ?? null;
  // Активный профиль можно переключить, не отключаясь: подпись под кнопкой
  // должна называть тот, с которым ядро работает сейчас.
  const shown = runningProfile(core, snap.list, active);

  // Активный профиль всегда первым: до него не должно быть прокрутки.
  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    return snap.list
      .filter((p) => !q || p.name.toLowerCase().includes(q) || p.client.serverAddress.toLowerCase().includes(q))
      .sort((a, b) => (a.id === snap.activeId ? -1 : b.id === snap.activeId ? 1 : 0));
  }, [snap, query]);

  // Во время остановки кнопка ничего не делает: запустить ядро заново,
  // пока прежнее ещё снимает маршруты, - верный способ получить два ядра.
  const switching = core?.state === "stopping";
  const toggleConnection = () => {
    if (switching) return;
    if (running) return run(() => api!.CoreStop());
    if (active) return run(() => api!.CoreStart(active));
  };

  const heroLabel = running ? "Отключить" : "Подключить";
  const statusText = phase.text;
  // Ошибку фазы показывает подпись под кнопкой - здесь только то, что
  // относится к самому окну (не удалось запустить, не сохранился профиль).
  const problem = error || (phase.tone === "error" ? phase.hint : "");

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {expanded ? (
        <button
          onClick={() => setExpanded(false)}
          className="flex items-center gap-3.5 px-8 py-4 text-left"
        >
          <span
            onClick={(e) => {
              e.stopPropagation();
              toggleConnection();
            }}
            className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-full border-2 ${
              phase.live
                ? "border-emerald-500 bg-emerald-500/10 text-emerald-500"
                : phase.busy
                  ? "border-amber-500 bg-amber-500/10 text-amber-500"
                  : "border-zinc-300 text-zinc-400 dark:border-zinc-700 dark:text-zinc-500"
            }`}
          >
            <PowerIcon size={20} />
          </span>
          <span className="flex min-w-0 flex-col">
            <span className="text-[15px] font-semibold">{statusText}</span>
            <span className="truncate text-xs text-zinc-500 dark:text-zinc-400">
              {shown ? shown.name : "Профиль не выбран"}
            </span>
          </span>
        </button>
      ) : (
        <div className="flex flex-col items-center justify-center gap-5 px-10 pb-8 pt-10">
          <button
            onClick={toggleConnection}
            disabled={!active || switching}
            className={`flex h-52 w-52 flex-col items-center justify-center gap-2.5 rounded-full border-2 transition disabled:opacity-50 ${
              phase.live
                ? "hero-live text-emerald-700 dark:text-emerald-400"
                : phase.busy
                  ? "hero-busy text-amber-700 dark:text-amber-300"
                  : "border-zinc-300 bg-white text-zinc-500 hover:border-zinc-400 dark:border-zinc-800 dark:bg-[#131316] dark:text-zinc-500 dark:hover:border-zinc-700"
            }`}
          >
            <PowerIcon size={52} />
            <span
              className={`text-[15px] font-semibold tracking-wide ${
                phase.live
                  ? "text-emerald-900 dark:text-emerald-100"
                  : phase.busy
                    ? "text-amber-900 dark:text-amber-100"
                    : "text-zinc-600 dark:text-zinc-300"
              }`}
            >
              {heroLabel}
            </span>
          </button>

          <div className="flex flex-col items-center gap-2">
            <div className="flex items-center gap-2">
              <span
                className={`h-2 w-2 rounded-full ${
                  phase.live
                    ? "bg-emerald-500"
                    : phase.tone === "error"
                      ? "bg-red-500"
                      : phase.busy
                        ? "dot-busy bg-amber-500"
                        : "bg-zinc-400"
                }`}
              />
              <span className="text-xl font-semibold">{statusText}</span>
            </div>
            <span className="max-w-md text-center text-[13px] text-zinc-500 dark:text-zinc-400">
              {/* При ошибке причина уже показана красной плашкой ниже - здесь
                  дублировать её незачем. */}
              {shown
                ? phase.hint && phase.tone !== "error"
                  ? `${shown.name} · ${phase.hint}`
                  : shown.name
                : "Добавьте профиль, чтобы подключиться"}
            </span>
          </div>

          {phase.live && (
            <div className="flex items-center rounded-full border border-zinc-200 bg-white px-1.5 py-2 dark:border-zinc-800 dark:bg-zinc-900">
              {core?.startedAt && (
                <>
                  <Stat value={uptime(core.startedAt)} tone="clock" />
                  <span className="h-[18px] w-px bg-zinc-200 dark:bg-zinc-800" />
                </>
              )}
              {traffic?.available ? (
                <>
                  <Stat value={bytes(traffic.rxBytes)} tone="emerald" />
                  <span className="h-[18px] w-px bg-zinc-200 dark:bg-zinc-800" />
                  <Stat value={bytes(traffic.txBytes)} tone="sky" />
                </>
              ) : (
                <span className="px-4 text-[13px] text-zinc-500 dark:text-zinc-400">
                  {traffic?.reason || "Счётчики недоступны"}
                </span>
              )}
            </div>
          )}
        </div>
      )}

      <div className="flex min-h-0 flex-1 flex-col gap-3 rounded-t-[20px] border-t border-zinc-200 bg-zinc-50 px-8 pb-6 pt-5 dark:border-zinc-800 dark:bg-[#111113]">
        <div className="flex items-center gap-3">
          <span className="text-[13px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
            Профили
          </span>
          <span className="text-xs text-zinc-400 dark:text-zinc-600">
            {query ? `показано ${visible.length} из ${snap.list.length}` : plural(snap.list.length)}
          </span>
          <span className="h-px flex-1 bg-zinc-200 dark:bg-zinc-800" />

          {/* Поле остаётся, пока в нём что-то есть: иначе список может
              остаться отфильтрованным без единого способа это отменить. */}
          {(snap.list.length >= SEARCH_FROM || query !== "") && (
            <div className="flex h-8 items-center gap-2 rounded-lg border border-zinc-200 bg-white px-3 dark:border-zinc-800 dark:bg-zinc-900">
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" className="text-zinc-400">
                <circle cx="11" cy="11" r="6.5" />
                <path d="M16 16l4 4" />
              </svg>
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Поиск"
                className="w-[150px] bg-transparent text-[13px] outline-none"
              />
            </div>
          )}

          {snap.list.length > 0 && (
            <Chip onClick={() => setExpanded((v) => !v)}>
              <svg
                width="15"
                height="15"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
                className={`text-zinc-500 transition dark:text-zinc-400 ${expanded ? "rotate-180" : ""}`}
              >
                <path d="M6 15l6-6 6 6" />
              </svg>
              {expanded ? "Свернуть" : "Развернуть"}
            </Chip>
          )}

          <Chip onClick={() => setImporting(true)}>
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="text-zinc-500 dark:text-zinc-400">
              <path d="M12 4v11M7 10l5 5 5-5M5 20h14" />
            </svg>
            Импорт ссылки
          </Chip>

          <Chip onClick={() => run(async () => onEditProfile((await api!.NewProfile("Новый сервер"))!))}>
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" className="text-zinc-500 dark:text-zinc-400">
              <path d="M12 5v14M5 12h14" />
            </svg>
            Добавить
          </Chip>
        </div>

        <div className="scroll fade-bottom flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto pr-1">
          {snap.list.length === 0 && (
            <div className="flex flex-col items-center gap-1 py-12 text-center">
              <span className="text-sm font-medium">Профилей пока нет</span>
              <span className="max-w-sm text-xs text-zinc-500 dark:text-zinc-400">
                Добавьте сервер вручную, вставьте ссылку freeturn:// или подключите подписку в настройках.
              </span>
            </div>
          )}

          {snap.list.length > 0 && visible.length === 0 && (
            <div className="flex flex-col items-center gap-1 py-10 text-center">
              <span className="text-[13px] text-zinc-500 dark:text-zinc-400">Ничего не найдено</span>
              <span className="text-xs text-zinc-400 dark:text-zinc-600">
                Проверьте название или адрес сервера
              </span>
            </div>
          )}

          {visible.map((p) => {
            const isActive = p.id === snap.activeId;
            const live = isActive && running;
            const vpn = isVPN(p);
            return (
              <div
                key={p.id}
                className={`flex items-center gap-3.5 rounded-xl border bg-white px-4 py-3.5 dark:bg-zinc-900 ${
                  isActive ? "border-emerald-500" : "border-zinc-200 dark:border-zinc-800"
                }`}
              >
                <button
                  onClick={() => run(() => api!.SetActiveProfile(p.id))}
                  className="flex min-w-0 flex-1 items-center gap-3.5 text-left"
                >
                  <span
                    className={`h-2.5 w-2.5 shrink-0 rounded-full ${
                      live ? "bg-emerald-500" : isActive ? "bg-emerald-500/40" : "bg-zinc-300 dark:bg-zinc-700"
                    }`}
                  />
                  <span className="flex min-w-0 flex-col gap-0.5">
                    <span className="truncate text-sm font-semibold">{p.name}</span>
                    <span className="truncate text-xs text-zinc-500 dark:text-zinc-400">
                      {p.client.serverAddress || "адрес не задан"}
                      {p.subUrl ? " · из подписки" : ""}
                    </span>
                  </span>
                </button>

                <span
                  className={`rounded-full px-2.5 py-1 text-[11px] ${
                    vpn
                      ? "bg-emerald-100 text-emerald-800 dark:bg-[#064e3b] dark:text-emerald-100"
                      : "bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400"
                  }`}
                >
                  {vpn ? "VPN" : "прокси"}
                </span>
                <span className="rounded-full bg-zinc-100 px-2.5 py-1 text-[11px] text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400">
                  {p.opts.obfProfile === "none" ? "без обфускации" : p.opts.obfProfile}
                </span>

                <RowMenu
                  open={menuFor === p.id}
                  onOpen={() => setMenuFor(menuFor === p.id ? null : p.id)}
                  items={[
                    { label: "Изменить", run: () => onEditProfile(p) },
                    { label: "Поделиться", run: () => setSharing(p) },
                    { label: "Клонировать", run: () => run(() => api!.DuplicateProfile(p.id)) },
                    { label: "Удалить", danger: true, run: () => run(() => api!.DeleteProfile(p.id)) },
                  ]}
                />
              </div>
            );
          })}
        </div>

        {problem && (
          <p className="rounded-xl bg-red-100 px-4 py-2.5 text-sm text-red-900 dark:bg-red-950 dark:text-red-200">
            {problem}
          </p>
        )}

        {phase.live && !problem && !isVPN(active) && (
          <p className="rounded-xl bg-amber-100 px-4 py-2.5 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
            Профиль работает в режиме прокси: ядро слушает{" "}
            <span className="font-mono">{active?.client.localPort || "127.0.0.1:9000"}</span> и трафик системы
            не заворачивает. Чтобы весь трафик пошёл через сервер, включите режим VPN в профиле.
          </p>
        )}
      </div>

      {importing && <ImportDialog onClose={() => setImporting(false)} />}
      {sharing && <ShareDialog profile={sharing} onClose={() => setSharing(null)} />}
    </div>
  );
}

/** Кнопка-чип из макета: высота 32, скруглении 8, поверхность списка. */
function Chip({ children, onClick }: { children: ReactNode; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="flex h-8 shrink-0 items-center gap-1.5 rounded-lg border border-zinc-200 bg-white px-3 text-[13px] text-zinc-700 transition hover:bg-zinc-100 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-200 dark:hover:bg-zinc-800"
    >
      {children}
    </button>
  );
}

function RowMenu({
  open,
  onOpen,
  items,
}: {
  open: boolean;
  onOpen: () => void;
  items: { label: string; run: () => void; danger?: boolean }[];
}) {
  return (
    <div className="relative shrink-0">
      <button
        onClick={onOpen}
        title="Действия"
        className="flex h-8 w-8 items-center justify-center rounded-lg text-zinc-400 transition hover:bg-zinc-100 dark:hover:bg-zinc-800"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round">
          <circle cx="12" cy="5" r="1.4" />
          <circle cx="12" cy="12" r="1.4" />
          <circle cx="12" cy="19" r="1.4" />
        </svg>
      </button>

      {open && (
        <>
          {/* Клик мимо закрывает меню - иначе оно живёт своей жизнью. */}
          <div className="fixed inset-0 z-10" onClick={onOpen} />
          <div className="absolute right-0 z-20 mt-1 flex w-44 flex-col rounded-xl border border-zinc-200 bg-white py-1 shadow-xl dark:border-zinc-700 dark:bg-zinc-800">
            {items.map((item) => (
              <button
                key={item.label}
                onClick={() => {
                  onOpen();
                  item.run();
                }}
                className={`px-3 py-2 text-left text-[13px] transition hover:bg-zinc-100 dark:hover:bg-zinc-700 ${
                  item.danger ? "text-red-600 dark:text-red-400" : ""
                }`}
              >
                {item.label}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function Stat({ value, tone }: { value: string; tone: "clock" | "emerald" | "sky" }) {
  const color =
    tone === "clock" ? "text-zinc-400" : tone === "emerald" ? "text-emerald-500" : "text-sky-400";
  return (
    <span className="flex items-center gap-2 px-4">
      <svg
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
        className={color}
      >
        {tone === "clock" ? (
          <>
            <circle cx="12" cy="12" r="9" />
            <path d="M12 7v5l3 2" />
          </>
        ) : tone === "emerald" ? (
          <path d="M12 19V5M6 11l6-6 6 6" />
        ) : (
          <path d="M12 5v14M6 13l6 6 6-6" />
        )}
      </svg>
      <span className="text-[13px] text-zinc-800 dark:text-zinc-200">{value}</span>
    </span>
  );
}

function PowerIcon({ size }: { size: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round">
      <path d="M12 3v9" />
      <path d="M18.4 6.6a9 9 0 1 1-12.8 0" />
    </svg>
  );
}

function plural(n: number) {
  const tail = n % 100 >= 11 && n % 100 <= 14 ? 5 : n % 10;
  if (tail === 1) return `${n} профиль`;
  if (tail >= 2 && tail <= 4) return `${n} профиля`;
  return `${n} профилей`;
}
