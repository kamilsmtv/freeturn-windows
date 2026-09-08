import type { CoreStatus, Profile, TunnelStatus } from "./api";

export type PhaseTone = "idle" | "busy" | "live" | "error";

export type Phase = {
  /** Короткая подпись под кнопкой. */
  text: string;
  tone: PhaseTone;
  /** Пояснение, что происходит сейчас; пусто, когда всё очевидно. */
  hint: string;
  /** Идёт переход: интерфейс показывает индикатор ожидания. */
  busy: boolean;
  /** Соединение действительно работает: можно показывать счётчики. */
  live: boolean;
};

/** Профиль работает через встроенный туннель, а не просто как прокси. */
export function isVPN(p: Profile | null): boolean {
  return p?.client.tunnelTransport === "wireguard";
}

/**
 * Фаза подключения одним значением.
 *
 * Состояния ядра недостаточно: в режиме VPN запущенное ядро - это ещё не
 * связь. Сначала оно должно договориться с TURN, и только потом поднимается
 * туннель. Пока туннеля нет, трафик никуда не идёт, и писать «Подключено»
 * значит обманывать.
 */
export function connectionPhase(
  core: CoreStatus | null,
  tunnel: TunnelStatus | null,
  active: Profile | null,
): Phase {
  const state = core?.state ?? "stopped";

  if (state === "failed") {
    return { text: "Ошибка", tone: "error", hint: core?.error ?? "", busy: false, live: false };
  }
  if (state === "stopping") {
    return { text: "Отключение…", tone: "busy", hint: "Снимаю туннель и маршруты", busy: true, live: false };
  }
  if (state === "stopped") {
    return { text: "Отключено", tone: "idle", hint: "", busy: false, live: false };
  }
  if (state === "starting") {
    return { text: "Запуск ядра…", tone: "busy", hint: "Ядро проходит проверку VK и ищет TURN-сервер", busy: true, live: false };
  }

  // Ядро работает. В режиме прокси это и есть готовность: оно слушает порт.
  if (!isVPN(active)) {
    return { text: "Подключено", tone: "live", hint: "Режим прокси: трафик системы не заворачивается", busy: false, live: true };
  }

  if (tunnel?.error) {
    return { text: "Туннель не поднялся", tone: "error", hint: tunnel.error, busy: false, live: false };
  }
  if (tunnel?.up) {
    return { text: "Подключено", tone: "live", hint: "Туннель поднят", busy: false, live: true };
  }
  return {
    text: "Поднимаю туннель…",
    tone: "busy",
    hint: "Ядро уже работает, ждём канал через TURN — обычно несколько секунд",
    busy: true,
    live: false,
  };
}
