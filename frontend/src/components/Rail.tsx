import type { ReactNode } from "react";
import type { CoreStatus } from "../lib/api";
import { Logo } from "./Logo";

export type Screen = "home" | "server" | "logs" | "updates" | "settings" | "about";

/** Значок раздела: рисуем контуром, чтобы он одинаково читался в обеих темах. */
function Icon({ name }: { name: Screen }) {
  const common = {
    width: 22,
    height: 22,
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.6,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
  };

  switch (name) {
    case "home":
      return (
        <svg {...common}>
          <path d="M12 3 3 10v10a1 1 0 0 0 1 1h5v-7h6v7h5a1 1 0 0 0 1-1V10z" />
        </svg>
      );
    case "server":
      return (
        <svg {...common}>
          <rect x="3" y="4" width="18" height="7" rx="2" />
          <rect x="3" y="13" width="18" height="7" rx="2" />
          <path d="M7 7.5h.01M7 16.5h.01" />
        </svg>
      );
    case "logs":
      return (
        <svg {...common}>
          <path d="M4 5h16M4 10h16M4 15h10M4 20h7" />
        </svg>
      );
    case "updates":
      return (
        <svg {...common}>
          <path d="M12 3v12M7 10l5 5 5-5" />
          <path d="M4 20h16" />
        </svg>
      );
    case "settings":
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="3.2" />
          <path d="M12 3v2.5M12 18.5V21M21 12h-2.5M5.5 12H3M18.4 5.6l-1.8 1.8M7.4 16.6l-1.8 1.8M18.4 18.4l-1.8-1.8M7.4 7.4 5.6 5.6" />
        </svg>
      );
    case "about":
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="9" />
          <path d="M12 16v-4M12 8h.01" />
        </svg>
      );
  }
}

const ITEMS: { id: Screen; label: string }[] = [
  { id: "home", label: "Главная" },
  { id: "server", label: "Сервер" },
  { id: "logs", label: "Журнал" },
  { id: "updates", label: "Обновления" },
  { id: "settings", label: "Настройки" },
];

/**
 * Навигация слева — то же, что нижние вкладки в Android-клиенте: разделы
 * всегда на виду, переход в один клик.
 */
export function Rail({
  screen,
  onScreen,
  core,
  children,
}: {
  screen: Screen;
  onScreen: (s: Screen) => void;
  core: CoreStatus | null;
  children?: ReactNode;
}) {
  const tone = core?.state === "failed" ? "failed" : core?.state === "running" ? "live" : "idle";

  return (
    <nav className="flex w-[88px] shrink-0 flex-col items-center gap-1 border-r border-zinc-200 bg-zinc-100 py-4 dark:border-zinc-800 dark:bg-[#0c0c0e]">
      <div title="Состояние подключения" className="mb-5 flex h-10 w-10 items-center justify-center">
        <Logo size={34} tone={tone} />
      </div>

      {ITEMS.map((item) => {
        const active = screen === item.id;
        return (
          <button
            key={item.id}
            onClick={() => onScreen(item.id)}
            className={`flex w-[72px] flex-col items-center gap-1.5 rounded-xl py-2.5 transition ${
              active
                ? "bg-white text-emerald-600 dark:bg-zinc-800 dark:text-emerald-500"
                : "text-zinc-500 hover:bg-white/60 dark:text-zinc-400 dark:hover:bg-zinc-900"
            }`}
          >
            <Icon name={item.id} />
            <span className="text-[11px]">{item.label}</span>
          </button>
        );
      })}

      <div className="flex-1" />
      {children}

      <button
        onClick={() => onScreen("about")}
        className={`flex w-[72px] flex-col items-center gap-1.5 rounded-xl py-2.5 transition ${
          screen === "about"
            ? "bg-white text-emerald-600 dark:bg-zinc-800 dark:text-emerald-500"
            : "text-zinc-500 hover:bg-white/60 dark:text-zinc-400 dark:hover:bg-zinc-900"
        }`}
      >
        <Icon name="about" />
        <span className="text-[11px]">О программе</span>
      </button>
    </nav>
  );
}
