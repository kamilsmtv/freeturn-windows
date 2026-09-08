import type { ReactNode } from "react";

export function Card({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <section className="rounded-2xl border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-[#131316]">
      {title && <h2 className="mb-3 text-sm font-semibold text-zinc-500 dark:text-zinc-400">{title}</h2>}
      {children}
    </section>
  );
}

/**
 * Колечко ожидания внутри кнопки: ровно там, где смотрит нажавший.
 * Цвет берётся у текста кнопки, поэтому годится для любого варианта.
 */
export function Spinner({ size = 14 }: { size?: number }) {
  return (
    <svg
      className="animate-spin"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth="3" opacity="0.25" />
      <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}

export function Button({
  children,
  onClick,
  variant = "default",
  disabled,
  busy,
}: {
  children: ReactNode;
  onClick?: () => void;
  variant?: "default" | "primary" | "ghost";
  disabled?: boolean;
  /** Кнопка ждёт ответа: показываем колечко и не даём нажать повторно. */
  busy?: boolean;
}) {
  // Размеры кнопок из макета: высота 36, радиус 8, текст 13.
  const base =
    "inline-flex h-9 shrink-0 items-center justify-center gap-2 rounded-lg px-4 text-[13px] font-medium transition disabled:opacity-40 disabled:pointer-events-none";
  const styles = {
    default:
      "border border-zinc-200 bg-white text-zinc-800 hover:bg-zinc-100 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-200 dark:hover:bg-zinc-800",
    primary: "bg-emerald-600 text-emerald-50 hover:bg-emerald-500",
    ghost: "text-zinc-600 hover:bg-zinc-200 dark:text-zinc-400 dark:hover:bg-zinc-800",
  }[variant];
  return (
    <button className={`${base} ${styles}`} onClick={onClick} disabled={disabled || busy}>
      {busy && <Spinner />}
      {children}
    </button>
  );
}

export function Row({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-zinc-100 py-2.5 last:border-0 dark:border-zinc-800">
      <div className="min-w-0">
        <div className="text-sm">{label}</div>
        {hint && <div className="text-xs text-zinc-500 dark:text-zinc-400">{hint}</div>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}

export function Toggle({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={`h-6 w-11 rounded-full p-0.5 transition ${
        checked ? "bg-emerald-600" : "bg-zinc-300 dark:bg-zinc-700"
      }`}
    >
      <span
        className={`block h-5 w-5 rounded-full bg-white transition ${checked ? "translate-x-5" : ""}`}
      />
    </button>
  );
}

export function Select<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value as T)}
      className="h-9 rounded-lg border border-zinc-300 bg-white px-2.5 text-[13px] dark:border-zinc-700 dark:bg-zinc-900"
    >
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}

export function EmptyState({ title, hint }: { title: string; hint: string }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-1 text-center">
      <div className="text-sm font-medium">{title}</div>
      <div className="max-w-sm text-xs text-zinc-500 dark:text-zinc-400">{hint}</div>
    </div>
  );
}
