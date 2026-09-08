import { useEffect, useSyncExternalStore } from "react";

/**
 * Общий признак «идёт работа» для полоски ожидания вверху окна.
 *
 * Разделы живут независимо, а полоска одна, поэтому состояние держим вне
 * дерева компонентов: каждый раздел отмечает своим ключом, что он занят,
 * и снимает отметку при размонтировании.
 */
const flags = new Map<string, string>();
const listeners = new Set<() => void>();

function notify() {
  for (const l of listeners) l();
}

export function setBusy(key: string, active: boolean, label = "") {
  if (active) {
    if (flags.get(key) === label) return;
    flags.set(key, label);
  } else {
    if (!flags.delete(key)) return;
  }
  notify();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Подпись первой занятой операции; пусто - работы нет. */
export function busyLabel(): string {
  for (const label of flags.values()) return label;
  return "";
}

/** Подписка на общий признак ожидания. */
export function useBusyLabel(): string {
  return useSyncExternalStore(subscribe, busyLabel, () => "");
}

/** Держит отметку занятости, пока active истинно, и снимает её при уходе. */
export function useBusyWhile(key: string, active: boolean, label: string) {
  useEffect(() => {
    setBusy(key, active, label);
    return () => setBusy(key, false);
  }, [key, active, label]);
}
