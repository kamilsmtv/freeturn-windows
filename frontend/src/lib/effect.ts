/**
 * React вызывает всё, что вернул эффект, если это не undefined — без
 * проверки на функцию. Любое случайно возвращённое значение роняет экран
 * с "is not a function" при размонтировании, причём стек указывает на
 * компонент, а не на конкретный эффект.
 *
 * guard возвращает React только функцию, а о постороннем значении пишет в
 * журнал окна (%APPDATA%\FreeTurn\logs\gui.log), чтобы виновник был виден.
 */
export function guard(name: string, run: () => unknown): () => void {
  const result = run();
  if (typeof result === "function") return result as () => void;

  if (result !== undefined && result !== null) {
    const message = `эффект ${name} вернул ${typeof result} вместо функции очистки`;
    window.runtime?.LogError?.(message);
    console.warn(message, result);
  }
  return () => {};
}
