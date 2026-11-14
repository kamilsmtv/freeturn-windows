import { useEffect, useRef, useState } from "react";
import { backend, type Traffic } from "../lib/api";
import { guard } from "../lib/effect";
import { bytes, rate, uptime } from "../lib/format";

/** Как часто мерить пинг без участия пользователя. */
const PING_EVERY = 60_000;

export type PingState = {
  /** Последнее удачное значение, мс. */
  ms: number | null;
  /** Когда получено последнее значение или отказ. */
  at: number | null;
  failed: boolean;
  /** Проверочный узел вне туннеля из-за раздельного туннелирования. */
  outside: boolean;
  measuring: boolean;
};

const idle: PingState = { ms: null, at: null, failed: false, outside: false, measuring: false };

/**
 * usePing меряет задержку через туннель раз в минуту, пока enabled.
 * Возвращает состояние и функцию внеочередного замера (кнопка ↻).
 */
export function usePing(enabled: boolean): [PingState, () => void] {
  const [state, setState] = useState<PingState>(idle);
  const busy = useRef(false);
  // gen - номер сеанса замеров. Ответ, пришедший после отключения или
  // уже в следующем подключении, относится к прошлому туннелю.
  const gen = useRef(0);

  const measure = () => {
    const api = backend();
    if (!api || busy.current || !enabled) return;
    const mine = gen.current;
    busy.current = true;
    setState((s) => ({ ...s, measuring: true }));
    api
      .Ping()
      .then((r) => {
        if (gen.current !== mine) return;
        const at = Date.now();
        setState((s) =>
          r.ok
            ? { ms: r.ms, at, failed: false, outside: false, measuring: false }
            : r.outside
              ? { ms: null, at, failed: false, outside: true, measuring: false }
              : { ...s, at, failed: true, outside: false, measuring: false },
        );
      })
      .catch(() => {
        if (gen.current === mine) setState((s) => ({ ...s, failed: true, measuring: false }));
      })
      .finally(() => {
        // Флаг снимает только свой сеанс: новый сеанс сбросил его сам и
        // мог уже начать замер.
        if (gen.current === mine) busy.current = false;
      });
  };

  useEffect(() => guard("StatsCard/ping", () => {
    gen.current++;
    busy.current = false;
    if (!enabled) {
      setState(idle);
      return;
    }
    measure();
    const id = setInterval(measure, PING_EVERY);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [enabled]);

  return [state, measure];
}

/** Цвет точки: до 150 мс хорошо, до 400 терпимо, дальше плохо. */
function pingDot(p: PingState): string {
  if (p.outside) return "bg-zinc-400 dark:bg-zinc-600";
  if (p.failed || p.ms === null) return "bg-red-500";
  if (p.ms < 150) return "bg-emerald-500";
  if (p.ms < 400) return "bg-amber-500";
  return "bg-red-500";
}

function ago(at: number | null): string {
  if (!at) return "";
  const s = Math.max(0, Math.round((Date.now() - at) / 1000));
  return s < 60 ? `${s} с назад` : `${Math.floor(s / 60)} мин назад`;
}

function clock(startedAt: string): string {
  const d = new Date(startedAt);
  if (Number.isNaN(d.getTime())) return "";
  return `с ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

/**
 * Карточка статистики под статусом: пинг, скорость, трафик и время в сети.
 * Макет - design/Stats.dc.html и design/StatsStates.dc.html.
 */
export function StatsCard({
  vpn,
  ping,
  onPing,
  traffic,
  startedAt,
}: {
  vpn: boolean;
  ping: PingState;
  onPing: () => void;
  traffic: Traffic | null;
  startedAt?: string;
}) {
  return (
    <div className="flex w-[680px] max-w-full items-stretch rounded-2xl border border-zinc-200 bg-white py-4 dark:border-zinc-800 dark:bg-[#131316]">
      <Cell
        label="Пинг"
        action={
          vpn && (
            <button
              onClick={onPing}
              disabled={ping.measuring}
              title="Измерить заново"
              className="-my-[3px] -mr-1 flex h-[22px] w-[22px] items-center justify-center rounded-md text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-700 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
            >
              {ping.measuring ? <Spin /> : <RefreshIcon />}
            </button>
          )
        }
      >
        {!vpn ? (
          <Two main={<span className="text-zinc-500 dark:text-zinc-400">—</span>} sub="только в режиме VPN" />
        ) : ping.outside ? (
          <Two
            main={<span className="text-zinc-500 dark:text-zinc-400">—</span>}
            sub="проверочный узел мимо туннеля"
          />
        ) : ping.measuring && (ping.ms !== null || ping.at === null) ? (
          <Two
            main={<span className="text-zinc-500 dark:text-zinc-400">измеряю…</span>}
            sub={ping.ms !== null ? `прошлый раз ${ping.ms} мс` : ""}
          />
        ) : (
          <Two
            main={
              <span className="flex items-center gap-2">
                <span className={`h-2 w-2 rounded-full ${pingDot(ping)}`} />
                {ping.failed ? "нет ответа" : `${ping.ms ?? "—"} мс`}
              </span>
            }
            sub={ping.failed ? "туннель не отвечает" : ago(ping.at)}
          />
        )}
      </Cell>

      {traffic?.available ? (
        <>
          <Cell label="Скорость">
            <Pair down={rate(traffic.rxRate)} up={rate(traffic.txRate)} />
          </Cell>
          <Cell label="Трафик">
            <Pair down={bytes(traffic.rxBytes)} up={bytes(traffic.txBytes)} />
          </Cell>
        </>
      ) : (
        <Cell label="Скорость и трафик" wide>
          <span className="whitespace-normal text-[13px] leading-snug text-zinc-500 dark:text-zinc-400">
            {traffic?.reason || "Счётчики недоступны"}
          </span>
        </Cell>
      )}

      <Cell label="В сети" last>
        <Two main={startedAt ? uptime(startedAt) : "—"} sub={startedAt ? clock(startedAt) : ""} />
      </Cell>
    </div>
  );
}

/**
 * Строка цифр для свёрнутой шапки, когда список профилей развёрнут.
 */
export function StatsStrip({ vpn, ping, traffic, startedAt }: {
  vpn: boolean;
  ping: PingState;
  traffic: Traffic | null;
  startedAt?: string;
}) {
  return (
    <span className="flex shrink-0 items-center gap-3.5 text-xs tabular-nums text-zinc-500 dark:text-zinc-400">
      {vpn && !ping.outside && (ping.ms !== null || ping.failed) && (
        <span className="flex items-center gap-1.5">
          <span className={`h-1.5 w-1.5 rounded-full ${pingDot(ping)}`} />
          {ping.failed ? "нет ответа" : `${ping.ms} мс`}
        </span>
      )}
      {traffic?.available && (
        <>
          <span className="flex items-center gap-1"><Arrow dir="down" size={12} />{rate(traffic.rxRate)}</span>
          <span className="flex items-center gap-1"><Arrow dir="up" size={12} />{rate(traffic.txRate)}</span>
        </>
      )}
      {startedAt && <span>{uptime(startedAt)}</span>}
    </span>
  );
}

function Cell({ label, action, children, last, wide }: {
  label: string;
  action?: React.ReactNode;
  children: React.ReactNode;
  last?: boolean;
  wide?: boolean;
}) {
  return (
    <div
      className={`flex min-w-0 flex-col gap-2 whitespace-nowrap px-5 ${wide ? "flex-[2_1_0]" : "flex-[1_1_0]"} ${
        last ? "" : "border-r border-zinc-200 dark:border-zinc-800"
      }`}
    >
      <div className="flex h-4 items-center">
        <span className="flex-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-zinc-500">{label}</span>
        {action}
      </div>
      {children}
    </div>
  );
}

function Two({ main, sub }: { main: React.ReactNode; sub: string }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-[15px] font-semibold tabular-nums text-zinc-900 dark:text-zinc-50">{main}</span>
      <span className="h-4 truncate text-xs text-zinc-500">{sub}</span>
    </div>
  );
}

function Pair({ down, up }: { down: string; up: string }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="flex items-center gap-1.5">
        <Arrow dir="down" />
        <span className="text-[15px] font-semibold tabular-nums text-zinc-900 dark:text-zinc-50">{down}</span>
      </span>
      <span className="flex items-center gap-1.5">
        <Arrow dir="up" />
        <span className="text-[13px] tabular-nums text-zinc-600 dark:text-zinc-400">{up}</span>
      </span>
    </div>
  );
}

function Arrow({ dir, size = 14 }: { dir: "up" | "down"; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.9"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={dir === "down" ? "text-emerald-500" : "text-sky-400"}
    >
      {dir === "down" ? <path d="M12 5v14M6 13l6 6 6-6" /> : <path d="M12 19V5M6 11l6-6 6 6" />}
    </svg>
  );
}

function RefreshIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 12a9 9 0 1 1-3-6.7" />
      <path d="M21 4v5h-5" />
    </svg>
  );
}

function Spin() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" strokeWidth="2.4" strokeLinecap="round" className="animate-spin">
      <circle cx="12" cy="12" r="9" className="stroke-zinc-300 dark:stroke-zinc-700" />
      <path d="M12 3a9 9 0 0 1 9 9" className="stroke-emerald-500" />
    </svg>
  );
}
