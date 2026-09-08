import type { ReactNode } from "react";

export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-xs font-medium text-zinc-500 dark:text-zinc-400">{label}</span>
      {children}
      {hint && <span className="text-xs text-zinc-400 dark:text-zinc-500">{hint}</span>}
    </label>
  );
}

/* Поле ввода из макета: радиус 8, тёмная подложка под рамкой. Высоту
   задают сами поля: у textarea она своя, а полагаться на то, чей класс
   окажется в стилях последним, нельзя. */
const inputClass =
  "w-full rounded-lg border border-zinc-300 bg-white px-3.5 text-sm outline-none focus:border-emerald-500 dark:border-zinc-600 dark:bg-[#09090b]";

const inputHeight = "h-[42px]";

export function TextInput({
  value,
  onChange,
  placeholder,
  type = "text",
  mono,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  type?: "text" | "password";
  mono?: boolean;
}) {
  return (
    <input
      type={type}
      value={value}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
      className={`${inputClass} ${inputHeight} ${mono ? "font-mono text-xs" : ""}`}
    />
  );
}

export function NumberInput({
  value,
  onChange,
  min,
  max,
}: {
  value: number;
  onChange: (v: number) => void;
  min?: number;
  max?: number;
}) {
  return (
    <input
      type="number"
      value={value}
      min={min}
      max={max}
      onChange={(e) => onChange(Number(e.target.value))}
      className={`${inputClass} ${inputHeight}`}
    />
  );
}

export function TextArea({
  value,
  onChange,
  placeholder,
  rows = 6,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  rows?: number;
}) {
  return (
    <textarea
      value={value}
      rows={rows}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
      className={`${inputClass} resize-y py-2.5 font-mono text-xs leading-relaxed`}
    />
  );
}

export function Picker<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
}) {
  return (
    <select value={value} onChange={(e) => onChange(e.target.value as T)} className={`${inputClass} ${inputHeight}`}>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}

export function CheckBox({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
}) {
  return (
    <label className="flex cursor-pointer items-center gap-2.5 text-[13px] text-zinc-600 dark:text-zinc-300">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="peer sr-only"
      />
      <span
        className={`flex h-[18px] w-[18px] shrink-0 items-center justify-center rounded-[5px] border transition ${
          checked
            ? "border-emerald-500 bg-emerald-500"
            : "border-zinc-300 bg-white dark:border-zinc-600 dark:bg-[#09090b]"
        }`}
      >
        {checked && (
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#052e24" strokeWidth="3.4" strokeLinecap="round" strokeLinejoin="round">
            <path d="M4 12.5 9.5 18 20 6.5" />
          </svg>
        )}
      </span>
      {label}
    </label>
  );
}
