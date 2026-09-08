import type { ReactNode } from "react";
import { backend, type AppInfo } from "../lib/api";
import type { Phase } from "../lib/phase";
import { Button } from "./ui";
import { Logo } from "./Logo";

/** Группа как в настройках: заголовок и строки. */
function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col rounded-2xl border border-zinc-200 bg-white py-1.5 dark:border-zinc-800 dark:bg-[#131316]">
      <h2 className="px-5 pb-2 pt-3 text-xs font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-500">
        {title}
      </h2>
      {children}
    </section>
  );
}

function InfoRow({ title, hint, children }: { title: string; hint?: string; children?: ReactNode }) {
  return (
    <div className="flex items-center gap-4 px-5 py-3">
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-sm">{title}</span>
        {hint && <span className="break-words text-xs text-zinc-500 dark:text-zinc-400">{hint}</span>}
      </div>
      {children && <div className="shrink-0">{children}</div>}
    </div>
  );
}

/**
 * Экран «О программе»: версии, где что лежит, куда идти за исходниками и
 * чей чужой труд внутри. Ссылки открываются во внешнем браузере - окно
 * приложения для этого не годится.
 */
export function About({
  info,
  phase,
  coreVersion,
}: {
  info: AppInfo | null;
  phase: Phase;
  /** Версия скачанного ядра; пусто - ещё не скачано. */
  coreVersion: string;
}) {
  if (!info) return null;

  const api = backend();
  const open = (url: string) => () => api?.OpenURL(url);
  const guiRepo = `https://github.com/${info.guiRepo}`;
  const coreRepo = `https://github.com/${info.coreRepo}`;

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h1 className="px-8 pb-3 pt-6 text-xl font-semibold">О программе</h1>

      <div className="scroll flex-1 overflow-y-auto px-8 pb-6">
        <div className="grid grid-cols-2 items-start gap-4">
          <div className="flex flex-col gap-4">
            <section className="flex items-center gap-4 rounded-2xl border border-zinc-200 bg-white p-5 dark:border-zinc-800 dark:bg-[#131316]">
              <Logo size={48} tone={phase.live ? "live" : "idle"} />
              <div className="min-w-0">
                <div className="text-lg font-semibold">FreeTurn для Windows</div>
                <div className="text-[13px] text-zinc-500 dark:text-zinc-400">
                  Версия {info.version} · клиент ядра free-turn-proxy
                </div>
              </div>
            </section>

            <Group title="Исходный код">
              <InfoRow title="Репозиторий приложения" hint={info.guiRepo}>
                <Button onClick={open(guiRepo)}>Открыть</Button>
              </InfoRow>
              <InfoRow title="Сообщить об ошибке" hint="Issues на GitHub">
                <Button onClick={open(guiRepo + "/issues")}>Открыть</Button>
              </InfoRow>
              <InfoRow title="Лицензия" hint="GPL-3.0">
                <Button onClick={open(guiRepo + "/blob/master/LICENSE")}>Текст</Button>
              </InfoRow>
            </Group>

            <Group title="Версии">
              <InfoRow title="Приложение">
                <span className="font-mono text-xs text-zinc-500 dark:text-zinc-400">{info.version}</span>
              </InfoRow>
              <InfoRow title="Ядро" hint={coreVersion ? "" : "Скачается при первом подключении"}>
                <span className="font-mono text-xs text-zinc-500 dark:text-zinc-400">
                  {coreVersion || "не скачано"}
                </span>
              </InfoRow>
            </Group>
          </div>

          <div className="flex flex-col gap-4">
            <Group title="Файлы на диске">
              <InfoRow title="Настройки и журналы" hint={info.dataDir}>
                <Button onClick={() => api?.OpenDataDir()}>Открыть</Button>
              </InfoRow>
              <InfoRow title="Ядро и его прошлая версия" hint={info.coreDir} />
              <InfoRow title="Журнал интерфейса" hint={info.logDir} />
            </Group>

            <Group title="Чужой труд внутри">
              <InfoRow title="free-turn-proxy" hint="Ядро проекта, автор samosvalishe">
                <Button onClick={open(coreRepo)}>GitHub</Button>
              </InfoRow>
              <InfoRow title="turn-proxy-android" hint="Модель профилей, формат ссылок и бэкапа">
                <Button onClick={open("https://github.com/samosvalishe/turn-proxy-android")}>GitHub</Button>
              </InfoRow>
              <InfoRow title="amneziawg-go" hint="WireGuard/AmneziaWG для встроенного туннеля">
                <Button onClick={open("https://github.com/amnezia-vpn/amneziawg-go")}>GitHub</Button>
              </InfoRow>
              <InfoRow title="Wintun" hint="Драйвер адаптера, WireGuard LLC">
                <Button onClick={open("https://www.wintun.net/")}>Сайт</Button>
              </InfoRow>
            </Group>

            <p className="px-1 text-xs text-zinc-500 dark:text-zinc-400">
              Проект создан в образовательных и исследовательских целях. Пользуйтесь им в
              соответствии с законами вашей страны.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
