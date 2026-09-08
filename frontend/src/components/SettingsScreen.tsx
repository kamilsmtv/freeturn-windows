import { useState, type ReactNode } from "react";
import { backend, type Settings } from "../lib/api";
import { Button, Select, Toggle } from "./ui";
import { TextInput } from "./Field";

/** Группа настроек: заголовок и строки — как разделы в Android-клиенте. */
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

function SettingRow({
  title,
  hint,
  children,
}: {
  title: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex items-center gap-4 px-5 py-3">
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-sm">{title}</span>
        {hint && <span className="text-xs text-zinc-500 dark:text-zinc-400">{hint}</span>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}

export function SettingsScreen({
  settings,
  patch,
  onSettings,
}: {
  settings: Settings;
  patch: (p: Partial<Settings>) => void;
  onSettings: (s: Settings) => void;
}) {
  const [subUrl, setSubUrl] = useState("");
  const [password, setPassword] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  // Имя действия вместо флага: колечко крутится в нажатой кнопке.
  const [busy, setBusy] = useState("");
  const api = backend();
  const subs = settings.subscriptions ?? [];

  const run = async (name: string, fn: () => Promise<string>) => {
    setBusy(name);
    setNote("");
    setError("");
    try {
      setNote(await fn());
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h1 className="px-8 pb-3 pt-6 text-xl font-semibold">Настройки</h1>

      <div className="scroll flex-1 overflow-y-auto px-8 pb-6">
        <div className="grid grid-cols-2 items-start gap-4">
          <div className="flex flex-col gap-4">
            <Group title="Приложение">
              <SettingRow title="Тема" hint="Следует за темой Windows">
                <Select
                  value={settings.theme}
                  onChange={(theme) => patch({ theme })}
                  options={[
                    { value: "system", label: "Системная" },
                    { value: "light", label: "Светлая" },
                    { value: "dark", label: "Тёмная" },
                  ]}
                />
              </SettingRow>
              <SettingRow
                title="Сворачивать в трей"
                hint="Крестик прячет окно, ядро продолжает работать. Выключение применяется после перезапуска."
              >
                <Toggle checked={settings.minimizeToTray} onChange={(v) => patch({ minimizeToTray: v })} />
              </SettingRow>
              <SettingRow title="Запускаться свёрнутым">
                <Toggle checked={settings.startMinimized} onChange={(v) => patch({ startMinimized: v })} />
              </SettingRow>
            </Group>

            <Group title="Автозапуск">
              <SettingRow
                title="Запускать при входе в систему"
                hint="Задача планировщика с правами администратора: иначе запуск упрётся в запрос UAC"
              >
                <Toggle
                  checked={settings.autostart}
                  onChange={async (v) => {
                    setError("");
                    try {
                      await api?.SetAutostart(v);
                      onSettings({ ...settings, autostart: v });
                    } catch (e) {
                      setError(String(e));
                    }
                  }}
                />
              </SettingRow>
              <SettingRow title="Подключать последний профиль" hint="Сразу после запуска приложения">
                <Toggle checked={settings.autoConnectLast} onChange={(v) => patch({ autoConnectLast: v })} />
              </SettingRow>
            </Group>
          </div>

          <div className="flex flex-col gap-4">
            <Group title="Ядро">
              <SettingRow title="Обновление" hint="Что делать, когда выходит новая версия">
                <Select
                  value={settings.coreUpdateMode}
                  onChange={(coreUpdateMode) => patch({ coreUpdateMode })}
                  options={[
                    { value: "ask", label: "Спрашивать" },
                    { value: "auto", label: "Автоматически" },
                    { value: "never", label: "Не проверять" },
                  ]}
                />
              </SettingRow>
              <SettingRow title="Интервал проверки" hint="Часы между обращениями к GitHub">
                <input
                  type="number"
                  min={1}
                  value={settings.coreUpdateHours}
                  onChange={(e) => patch({ coreUpdateHours: Number(e.target.value) })}
                  className="w-20 rounded-lg border border-zinc-300 bg-white px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-800"
                />
              </SettingRow>
              <SettingRow title="Токен GitHub" hint="Необязателен; поднимает лимит запросов к API">
                <input
                  type="password"
                  value={settings.githubToken}
                  onChange={(e) => patch({ githubToken: e.target.value })}
                  placeholder="ghp_…"
                  className="w-52 rounded-lg border border-zinc-300 bg-white px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-800"
                />
              </SettingRow>
              <SettingRow title="Проверять обновления приложения" hint="Только уведомление со ссылкой">
                <Toggle checked={settings.checkGuiUpdates} onChange={(v) => patch({ checkGuiUpdates: v })} />
              </SettingRow>
            </Group>

            <Group title="Подписки">
              <div className="flex gap-2 px-5 py-2">
                <TextInput value={subUrl} onChange={setSubUrl} placeholder="https://example.com/sub.md" />
                <Button
                  disabled={busy !== "" || !subUrl.trim()}
                  busy={busy === "sub-add"}
                  onClick={() =>
                    run("sub-add", async () => {
                      const p = await api!.ApplySubscription(subUrl.trim());
                      setSubUrl("");
                      const skipped = p.skipped ? `, пропущено ссылок: ${p.skipped}` : "";
                      return `Добавлено серверов: ${p.profiles.length}${skipped}`;
                    })
                  }
                >
                  Добавить
                </Button>
              </div>

              {subs.length === 0 && (
                <p className="px-5 pb-2 text-xs text-zinc-500 dark:text-zinc-400">Подписок нет</p>
              )}
              {subs.map((url) => (
                <SettingRow key={url} title={url}>
                  <div className="flex gap-2">
                    <Button
                      disabled={busy !== ""}
                      busy={busy === "sub-refresh:" + url}
                      onClick={() =>
                        run("sub-refresh:" + url, async () => {
                          const p = await api!.ApplySubscription(url);
                          return `Обновлено серверов: ${p.profiles.length}`;
                        })
                      }
                    >
                      Обновить
                    </Button>
                    <Button
                      variant="ghost"
                      disabled={busy !== ""}
                      busy={busy === "sub-remove:" + url}
                      onClick={() =>
                        run("sub-remove:" + url, async () => {
                          await api!.RemoveSubscription(url);
                          patch({ subscriptions: subs.filter((u) => u !== url) });
                          return "Подписка удалена; её профили остались в списке";
                        })
                      }
                    >
                      Удалить
                    </Button>
                  </div>
                </SettingRow>
              ))}

              <SettingRow title="Интервал обновления" hint="Часы между автоматическими обновлениями">
                <input
                  type="number"
                  min={1}
                  value={settings.subRefreshHours}
                  onChange={(e) => patch({ subRefreshHours: Number(e.target.value) })}
                  className="w-20 rounded-lg border border-zinc-300 bg-white px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-800"
                />
              </SettingRow>
            </Group>

            <Group title="Резервная копия">
              <p className="px-5 pb-1 text-xs text-zinc-500 dark:text-zinc-400">
                Формат совместим с Android-клиентом: внутри профили, ключи обфускации и доступы SSH. Без
                пароля файл не читается.
              </p>
              <div className="flex items-center gap-2 px-5 py-3">
                <TextInput type="password" value={password} onChange={setPassword} placeholder="Пароль бэкапа" />
                <Button
                  disabled={busy !== "" || !password}
                  busy={busy === "backup-save"}
                  onClick={() =>
                    run("backup-save", async () => {
                      const path = await api!.ExportBackup(password);
                      return path ? `Сохранено: ${path}` : "Сохранение отменено";
                    })
                  }
                >
                  Экспорт
                </Button>
                <Button
                  disabled={busy !== "" || !password}
                  busy={busy === "backup-load"}
                  onClick={() =>
                    run("backup-load", async () => {
                      const n = await api!.ImportBackup(password);
                      return n > 0 ? `Восстановлено профилей: ${n}` : "Восстановление отменено";
                    })
                  }
                >
                  Импорт
                </Button>
              </div>
            </Group>
          </div>
        </div>

        {note && <p className="pt-4 text-sm text-emerald-600">{note}</p>}
        {error && <p className="pt-4 text-sm text-red-600 dark:text-red-400">{error}</p>}
      </div>
    </div>
  );
}
