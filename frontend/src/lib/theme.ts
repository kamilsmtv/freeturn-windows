export type Theme = "system" | "light" | "dark";

/**
 * Вешает класс `dark` на <html> и подписывается на смену системной темы,
 * пока выбран режим "system". Возвращает функцию отписки.
 */
export function applyTheme(theme: Theme): () => void {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  const paint = () => {
    const dark = theme === "dark" || (theme === "system" && media.matches);
    document.documentElement.classList.toggle("dark", dark);
  };
  paint();
  if (theme !== "system") return () => {};
  media.addEventListener("change", paint);
  return () => media.removeEventListener("change", paint);
}
