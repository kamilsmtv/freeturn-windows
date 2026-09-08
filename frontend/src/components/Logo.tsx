/**
 * Знак приложения: контурный ромб с ромбовидной сердцевиной — та же
 * геометрия, что у иконки в трее и в панели задач (design/icon).
 */
export function Logo({ size = 36, tone = "idle" }: { size?: number; tone?: "live" | "idle" | "failed" }) {
  const color =
    tone === "live"
      ? "text-emerald-500"
      : tone === "failed"
        ? "text-red-500"
        : "text-zinc-400 dark:text-zinc-600";

  // Толщина контура растёт на малых размерах: тонкий штрих там пропадает.
  const stroke = size <= 24 ? 22 : size <= 40 ? 18 : 16;
  const core = size <= 24 ? 46 : 40;

  return (
    <svg width={size} height={size} viewBox="0 0 256 256" fill="none" className={color}>
      <path
        d="M128 24 232 128 128 232 24 128Z"
        stroke="currentColor"
        strokeWidth={stroke}
        strokeLinejoin="round"
      />
      <path
        d={`M128 ${128 - core} ${128 + core} 128 128 ${128 + core} ${128 - core} 128Z`}
        fill="currentColor"
      />
    </svg>
  );
}
