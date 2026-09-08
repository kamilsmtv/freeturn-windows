/** Форматирование объёма в тех же единицах, что использует ядро (KiB, MiB). */
export function bytes(n: number): string {
  if (n < 1024) return `${n} Б`;
  const units = ["КиБ", "МиБ", "ГиБ", "ТиБ"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`;
}

export function rate(bytesPerSecond: number): string {
  return `${bytes(bytesPerSecond)}/с`;
}

/** Длительность с момента запуска в виде «1 ч 05 мин». */
export function uptime(startedAt: string): string {
  if (!startedAt) return "";
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(startedAt).getTime()) / 1000));
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h} ч ${String(m).padStart(2, "0")} мин`;
  if (m > 0) return `${m} мин ${String(s).padStart(2, "0")} с`;
  return `${s} с`;
}
