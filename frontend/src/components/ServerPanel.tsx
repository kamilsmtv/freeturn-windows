import { useEffect, useRef, useState, type ReactNode } from "react";
import { backend, EVENTS, onEvent, type Profile, type ProbeData, type VPSResult } from "../lib/api";
import { Button } from "./ui";
import { Field, NumberInput, Picker, TextArea, TextInput } from "./Field";
import { useBusyWhile } from "../lib/busy";
import { guard } from "../lib/effect";
import { QRCode } from "./QRCode";

/**
 * Управление сервером по SSH. Все команды идут через install.sh ядра:
 * скрипт скачивается приложением и передаётся на stdin bash, обратно
 * приходит один JSON-объект (протокол v2 самого install.sh).
 *
 * Экран собран как в макете: слева — «Настройка» с шагами сверху вниз,
 * справа — состояние сервера, предупреждения и ход выполнения команд.
 */
export function ServerPanel({ profile, onProfile }: { profile: Profile; onProfile: (p: Profile) => void }) {
  const [busy, setBusy] = useState("");
  const [probe, setProbe] = useState<ProbeData | null>(null);
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [fingerprint, setFingerprint] = useState("");
  const [log, setLog] = useState<string[]>([]);
  const [draft, setDraft] = useState<Profile>(profile);
  const [saved, setSaved] = useState(true);
  const [share, setShare] = useState("");
  const [peerName, setPeerName] = useState("");
  // Форма доступа занимает пол-экрана, поэтому у подтверждённого сервера
  // она свёрнута: разворачиваем по клику, как в макете.
  const [sshOpen, setSshOpen] = useState(!profile.ssh.hostFingerprint);
  const logBox = useRef<HTMLDivElement>(null);
  const api = backend();

  useEffect(() => guard("ServerPanel/vpsLog", () =>
    onEvent<string>(EVENTS.vpsLog, (l) => setLog((prev) => [...prev.slice(-400), l])),
  ), []);
  // Смена активного профиля должна подхватываться формой доступа.
  useEffect(() => guard("ServerPanel/profile", () => {
    setDraft(profile);
    setSaved(true);
    setSshOpen(!profile.ssh.hostFingerprint);
  }), [profile.id]);
  // Команды меняют профиль на бэкенде (порт бэкенда, конфиг WireGuard):
  // подхватываем новую версию, пока в форме нет несохранённых правок.
  useEffect(() => guard("ServerPanel/sync", () => {
    if (saved) {
      setDraft(profile);
    }
  }), [profile, saved]);

  // Прокручиваем сам блок журнала, а не страницу: scrollIntoView тянет за
  // собой все родительские контейнеры и уводит экран от нажатой кнопки.
  useEffect(() => guard("ServerPanel/scroll", () => {
    const box = logBox.current;
    if (box) box.scrollTop = box.scrollHeight;
  }), [log.length]);

  // Молчаливый опрос: обновляет состояние и шаги, но не трогает
  // сообщения об ошибках - иначе он затирал бы результат самой команды.
  const refreshProbe = async () => {
    try {
      const r = await api?.VPSProbe(profile.id);
      if (r?.ok && r.probe) setProbe(r.probe);
    } catch {
      // Молчим намеренно: это фоновая проверка, а не действие пользователя.
    }
  };

  // При открытии вкладки состояние сервера должно быть уже известно -
  // иначе все шаги выглядят непройденными. Спрашиваем только сервер с
  // подтверждённым ключом: иначе всплыло бы окно про отпечаток.
  useEffect(() => guard("ServerPanel/autoProbe", () => {
    if (profile.ssh.ip.trim() !== "" && profile.ssh.hostFingerprint !== "") {
      void refreshProbe();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [profile.id]);

  const run = async (name: string, fn: () => Promise<VPSResult>) => {
    setBusy(name);
    setNote("");
    setError("");
    setFingerprint("");
    try {
      const r = await fn();
      if (r.fingerprint) {
        setFingerprint(r.fingerprint);
        setError(r.error);
        return;
      }
      if (!r.ok) {
        setError(r.error || "не удалось выполнить команду");
        return;
      }
      if (r.probe) setProbe(r.probe);
      if (r.profile) onProfile(r.profile);
      if (r.text) setNote(r.text);
      // Команда меняет состояние сервера: шаги должны это показать сразу.
      if (name !== "probe" && name !== "logs") {
        void refreshProbe();
      }
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy("");
    }
  };

  const patchSSH = (patch: Partial<Profile["ssh"]>) => {
    setDraft({ ...draft, ssh: { ...draft.ssh, ...patch } });
    setSaved(false);
  };
  const patchProfile = (patch: Partial<Profile>) => {
    setDraft({ ...draft, ...patch });
    setSaved(false);
  };

  // Возврат к свёрнутому виду: несохранённые правки при этом отбрасываем,
  // иначе они остались бы жить невидимыми и молча попали в следующую команду.
  const closeSSH = () => {
    setDraft(profile);
    setSaved(true);
    setSshOpen(false);
  };

  const saveDraft = async () => {
    setError("");
    try {
      const p = await api!.SaveProfile(draft);
      onProfile(p);
      setSaved(true);
      // Подтверждённый доступ снова прячем: форма занимает пол-экрана.
      if (p.ssh.hostFingerprint !== "") {
        setSshOpen(false);
      }
      setNote("Доступ сохранён");
    } catch (e) {
      setError(String(e));
    }
  };

  // Пока команда выполняется, вверху окна идёт полоска ожидания.
  useBusyWhile("vps", busy !== "", "Команда серверу");

  const idle = busy === "" && api !== null;
  // Команды работают с сохранённым профилем: несохранённые правки бэкенд не увидит.
  const ready = saved && draft.ssh.ip.trim() !== "";
  // Кнопка сохранения не должна зависеть от ready - иначе сохранить
  // несохранённое становится невозможно.
  const canSave = idle && !saved;
  const disabled = !idle || !ready;

  const access = `${draft.ssh.username || "root"}@${draft.ssh.ip}:${draft.ssh.port || 22}`;
  const subtitle = [
    draft.ssh.ip.trim() || "адрес не задан",
    probe ? (probe.installed ? `ядро ${probe.version || "?"} ${probe.running ? "работает" : "остановлено"}` : "ядро не установлено") : "состояние неизвестно",
    probe?.runtime || "",
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <header className="flex items-center gap-4 border-b border-zinc-200 px-8 pb-[18px] pt-6 dark:border-zinc-900">
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-xl font-semibold">Сервер {profile.name}</h1>
          <p className="truncate text-[13px] text-zinc-500 dark:text-zinc-400">{subtitle}</p>
        </div>
        <HeaderButton disabled={disabled} onClick={() => run("probe", () => api!.VPSProbe(profile.id))}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="text-zinc-500 dark:text-zinc-400">
            <path d="M21 12a9 9 0 1 1-3-6.7" />
            <path d="M21 4v5h-5" />
          </svg>
          {busy === "probe" ? "Опрашиваю…" : "Проверить"}
        </HeaderButton>
        <HeaderButton disabled={disabled} onClick={() => run("logs", () => api!.VPSLogs(profile.id, 120))}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="text-zinc-500 dark:text-zinc-400">
            <path d="M4 6h16M4 12h16M4 18h10" />
          </svg>
          Журнал сервера
        </HeaderButton>
      </header>

      <div className="scroll grid min-h-0 flex-1 items-start gap-4 grid-cols-2 overflow-y-auto px-8 pb-6 pt-5">
        <section className="flex flex-col gap-3 rounded-2xl border border-zinc-200 bg-white p-5 dark:border-zinc-800 dark:bg-[#131316]">
          <h2 className="text-[13px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
            Настройка
          </h2>

          <Step
            done={Boolean(draft.ssh.hostFingerprint) && saved}
            title={draft.ssh.hostFingerprint ? "Доступ по SSH подтверждён" : "Доступ по SSH"}
            hint={draft.ssh.ip.trim() ? access : "IP, пользователь и способ входа — команды идут через SSH"}
            action={
              draft.ssh.hostFingerprint ? (
                sshOpen ? (
                  <Button variant="ghost" onClick={closeSSH}>
                    {saved ? "Свернуть" : "Отменить"}
                  </Button>
                ) : (
                  <Button variant="ghost" onClick={() => setSshOpen(true)}>
                    Изменить
                  </Button>
                )
              ) : null
            }
          >
            {(!draft.ssh.hostFingerprint || sshOpen) && (
              <div className="flex flex-col gap-3 pt-1">
                <div className="grid gap-3">
                  <Field label="Адрес сервера" hint="IP или домен VPS">
                    <TextInput value={draft.ssh.ip} onChange={(ip) => patchSSH({ ip })} placeholder="203.0.113.10" />
                  </Field>
                  <Field label="Порт SSH">
                    <NumberInput
                      value={draft.ssh.port || 22}
                      min={1}
                      max={65535}
                      onChange={(port) => patchSSH({ port })}
                    />
                  </Field>
                  <Field label="Пользователь">
                    <TextInput value={draft.ssh.username} onChange={(username) => patchSSH({ username })} />
                  </Field>
                  <Field label="Способ входа">
                    <Picker
                      value={draft.ssh.authType}
                      onChange={(authType) => patchSSH({ authType })}
                      options={[
                        { value: "PASSWORD", label: "Пароль" },
                        { value: "SSH_KEY", label: "Приватный ключ" },
                      ]}
                    />
                  </Field>
                  {draft.ssh.authType === "PASSWORD" && (
                    <Field label="Пароль">
                      <TextInput
                        type="password"
                        value={draft.ssh.password}
                        onChange={(password) => patchSSH({ password })}
                      />
                    </Field>
                  )}
                  <Field label="Способ повышения прав" hint="Как выполнять команды от root">
                    <Picker
                      value={draft.ssh.rootMode}
                      onChange={(rootMode) => patchSSH({ rootMode })}
                      options={[
                        { value: "ROOT", label: "Вход сразу под root" },
                        { value: "SUDO_NOPASS", label: "sudo без пароля" },
                        { value: "SUDO_PASS", label: "sudo с паролем" },
                      ]}
                    />
                  </Field>
                  {draft.ssh.rootMode === "SUDO_PASS" && (
                    <Field label="Пароль sudo" hint="Пусто — будет взят пароль SSH">
                      <TextInput
                        type="password"
                        value={draft.ssh.sudoPassword}
                        onChange={(sudoPassword) => patchSSH({ sudoPassword })}
                      />
                    </Field>
                  )}
                  <Field label="Адрес прослушивания сервера" hint="Флаг -listen серверной части">
                    <TextInput value={draft.proxyListen} onChange={(proxyListen) => patchProfile({ proxyListen })} />
                  </Field>
                  <Field label="Локальный бэкенд на сервере" hint="Флаг -connect: WireGuard 51820, Xray 443">
                    <TextInput value={draft.proxyConnect} onChange={(proxyConnect) => patchProfile({ proxyConnect })} />
                  </Field>
                </div>

                {draft.ssh.authType === "SSH_KEY" && (
                  <Field label="Приватный ключ" hint="Вставьте содержимое файла целиком, включая строки BEGIN и END">
                    <TextArea
                      value={draft.ssh.sshKey}
                      onChange={(sshKey) => patchSSH({ sshKey })}
                      rows={5}
                      placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                    />
                  </Field>
                )}

                <div>
                  <Button variant={saved ? "default" : "primary"} disabled={!canSave} onClick={saveDraft}>
                    {saved ? "Сохранено" : "Сохранить доступ"}
                  </Button>
                </div>
              </div>
            )}
          </Step>

          <Step
            done={Boolean(probe?.installed)}
            title={probe?.installed ? "Ядро установлено" : "Установить ядро"}
            hint={
              probe?.installed
                ? `${probe.version || "версия неизвестна"} · ${probe.running ? "служба работает" : "служба остановлена"}`
                : "install.sh соберёт и поставит серверную часть free-turn-proxy"
            }
          >
            <div className="pt-2">
              <Button
                variant="primary"
                disabled={disabled}
                onClick={() => run("install", () => api!.VPSInstall(profile.id, true))}
              >
                {busy === "install" ? "Устанавливаю…" : probe?.installed ? "Переустановить" : "Установить ядро"}
              </Button>
            </div>
          </Step>

          <Step
            done={Boolean(probe?.wg.present)}
            title={probe?.wg.present ? "WireGuard настроен" : "Настроить WireGuard"}
            hint={
              probe?.wg.present
                ? `порт ${probe.wg.port}${probe.wg_kernel ? ", модуль ядра" : ""} · конфигурация клиента в профиле`
                : "Сервер заведёт интерфейс и отдаст конфигурацию клиента; если утилит wg нет, приложение поставит wireguard-tools"
            }
          >
            <div className="flex flex-wrap gap-2 pt-1">
              <Button disabled={disabled} onClick={() => run("share", () => api!.VPSImportShareInfo(profile.id))}>
                {busy === "share" ? "Спрашиваю сервер…" : "Забрать параметры подключения"}
              </Button>
              <Button disabled={disabled} onClick={() => run("wg", () => api!.VPSSetupWireGuard(profile.id))}>
                {busy === "wg" ? "Настраиваю…" : "Настроить WireGuard и забрать конфиг"}
              </Button>
            </div>
          </Step>

          <Step
            done={false}
            title="Пригласить гостя"
            hint="Сервер заведёт пира и отдаст готовую ссылку"
          >
            <div className="flex flex-col gap-2 pt-2">
              <div className="flex items-center gap-2">
                <TextInput value={peerName} onChange={setPeerName} placeholder="Имя гостя" />
                <Button
                  variant="primary"
                  disabled={disabled || !peerName.trim()}
                  onClick={async () => {
                    setError("");
                    try {
                      setShare(await api!.VPSAddPeer(profile.id, peerName.trim()));
                    } catch (e) {
                      setError(String(e));
                    }
                  }}
                >
                  Создать ссылку
                </Button>
              </div>
              {share && (
                <>
                  <pre className="scroll selectable max-h-28 overflow-auto rounded-lg bg-zinc-950 p-3 font-mono text-xs text-zinc-300">
                    {share}
                  </pre>
                  {/* Гостю удобнее навести камеру, чем переносить длинную строку. */}
                  <QRCode text={share} name={peerName.trim() || profile.name} />
                  <div>
                    <Button onClick={() => navigator.clipboard.writeText(share)}>Скопировать ссылку</Button>
                  </div>
                </>
              )}
            </div>
          </Step>
        </section>

        <div className="flex flex-col gap-4">
          <section className="flex flex-col gap-2.5 rounded-2xl border border-zinc-200 bg-white p-5 dark:border-zinc-800 dark:bg-[#131316]">
            <h2 className="text-[13px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
              Состояние
            </h2>
            <div className="flex items-center gap-2.5">
              <span
                className={`h-2 w-2 shrink-0 rounded-full ${
                  probe?.running ? "bg-emerald-500" : probe ? "bg-zinc-400 dark:bg-zinc-600" : "bg-zinc-300 dark:bg-zinc-700"
                }`}
              />
              <span className="flex-1 text-sm">
                {probe
                  ? probe.running
                    ? "Серверная часть работает"
                    : probe.installed
                      ? "Ядро установлено, но остановлено"
                      : "Ядро не установлено"
                  : "Нажмите «Проверить» — приложение опросит VPS"}
              </span>
              <span className="text-xs text-zinc-500 dark:text-zinc-400">
                {draft.proxyListen} → {draft.proxyConnect}
              </span>
            </div>
            <div className="flex items-center gap-2.5 pt-1">
              <Button disabled={disabled} onClick={() => run("start", () => api!.VPSStart(profile.id))}>
                {busy === "start" ? "Запускаю…" : probe?.running ? "Перезапустить" : "Запустить"}
              </Button>
              <Button disabled={disabled} onClick={() => run("stop", () => api!.VPSStop(profile.id))}>
                {busy === "stop" ? "Останавливаю…" : "Остановить"}
              </Button>
            </div>
          </section>

          {note && <Notice tone="ok">{note}</Notice>}
          {error && !fingerprint && <Notice tone="error">{error}</Notice>}
          {!ready && (
            <Notice tone="warn">
              {draft.ssh.ip.trim() === ""
                ? "Заполните адрес сервера и доступ по SSH — без них команды не выполнить."
                : "Сохраните доступ: команды выполняются с сохранённым профилем."}
            </Notice>
          )}

          {fingerprint && (
            <div className="flex flex-col gap-2 rounded-xl border border-amber-300 bg-amber-50 p-4 dark:border-[#422006] dark:bg-[#1c1917]">
              <span className="text-[13px] text-amber-900 dark:text-amber-300">
                Сервер предъявил неизвестный ключ
              </span>
              <code className="selectable break-all font-mono text-xs text-amber-800 dark:text-amber-400">
                {fingerprint}
              </code>
              <span className="text-xs text-amber-800 dark:text-zinc-400">
                Сверьте отпечаток с тем, что показывает сам сервер: он защищает от подмены соединения.
              </span>
              <div className="pt-1">
                <Button
                  variant="primary"
                  onClick={async () => {
                    await api?.TrustHostKey(profile.id, fingerprint);
                    setFingerprint("");
                    setNote("Ключ сервера сохранён — повторите команду");
                  }}
                >
                  Доверять этому ключу
                </Button>
              </div>
            </div>
          )}

          {probe && conflicts(probe).length > 0 && (
            <div className="flex items-start gap-2.5 rounded-xl border border-amber-300 bg-amber-50 px-4 py-3.5 dark:border-[#422006] dark:bg-[#1c1917]">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" className="mt-px shrink-0 text-amber-500">
                <path d="M12 9v4M12 17h.01" />
                <path d="M10.3 3.9 2.6 17.2A1.9 1.9 0 0 0 4.3 20h15.4a1.9 1.9 0 0 0 1.7-2.8L13.7 3.9a1.9 1.9 0 0 0-3.4 0z" />
              </svg>
              <div className="flex flex-col gap-0.5">
                <span className="text-[13px] text-amber-900 dark:text-amber-300">
                  На сервере найдено: {conflicts(probe).join(", ")}
                </span>
                <span className="text-xs text-amber-800 dark:text-zinc-400">
                  Может занимать порты и свои интерфейсы — если появятся странности с портами, начните с этого
                </span>
              </div>
            </div>
          )}

          <section className="flex flex-col gap-2.5 rounded-2xl border border-zinc-200 bg-white p-[18px] dark:border-zinc-800 dark:bg-[#0c0c0e]">
            <div className="flex items-center gap-2">
              <h2 className="flex-1 text-[13px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
                Ход выполнения
              </h2>
              <span className="text-xs text-zinc-400 dark:text-zinc-600">install.sh · протокол v2</span>
            </div>
            <div
              ref={logBox}
              className="scroll selectable flex h-56 flex-col gap-1.5 overflow-y-auto font-mono text-[11.5px] leading-relaxed text-zinc-500 dark:text-zinc-400"
            >
              {log.length === 0 ? (
                <span className="font-sans text-xs text-zinc-400 dark:text-zinc-600">
                  Здесь появится вывод команд install.sh
                </span>
              ) : (
                log.map((l, i) => (
                  <span key={i} className="whitespace-pre-wrap break-all">
                    {l}
                  </span>
                ))
              )}
            </div>
            {log.length > 0 && (
              <div>
                <Button variant="ghost" onClick={() => setLog([])}>
                  Очистить
                </Button>
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}

/** Шаг настройки: галочка и зелёный фон, когда он уже пройден. */
function Step({
  done,
  title,
  hint,
  action,
  children,
}: {
  done: boolean;
  title: string;
  hint?: string;
  action?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <section
      className={`flex items-start gap-3 rounded-xl border px-3.5 py-3 ${
        done
          ? "border-emerald-200 bg-emerald-50 dark:border-[#14532d] dark:bg-[#0f1a15]"
          : "border-zinc-200 bg-zinc-50 dark:border-zinc-800 dark:bg-zinc-900"
      }`}
    >
      <span
        className={`mt-0.5 flex h-[22px] w-[22px] shrink-0 items-center justify-center rounded-full ${
          done ? "bg-emerald-500" : "border-2 border-zinc-300 dark:border-zinc-600"
        }`}
      >
        {done && (
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="#052e24" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
            <path d="M4 12.5 9.5 18 20 6.5" />
          </svg>
        )}
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex items-start gap-2">
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium">{title}</h3>
            {hint && <p className="text-xs text-zinc-500 dark:text-zinc-400">{hint}</p>}
          </div>
          {action}
        </div>
        {children}
      </div>
    </section>
  );
}

function Notice({ tone, children }: { tone: "ok" | "warn" | "error"; children: ReactNode }) {
  const styles = {
    ok: "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-[#14532d] dark:bg-[#0f1a15] dark:text-emerald-300",
    warn: "border-amber-300 bg-amber-50 text-amber-800 dark:border-[#422006] dark:bg-[#1c1917] dark:text-amber-300",
    error: "border-red-300 bg-red-50 text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300",
  }[tone];
  return <p className={`rounded-xl border px-4 py-3 text-[13px] ${styles}`}>{children}</p>;
}

/** Кнопка в шапке экрана: 36px, иконка слева — как в макете. */
function HeaderButton({
  children,
  onClick,
  disabled,
}: {
  children: ReactNode;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className="flex h-9 shrink-0 items-center gap-2 rounded-lg border border-zinc-200 bg-white px-4 text-[13px] transition hover:bg-zinc-100 disabled:pointer-events-none disabled:opacity-40 dark:border-zinc-800 dark:bg-zinc-900 dark:hover:bg-zinc-800"
    >
      {children}
    </button>
  );
}

function conflicts(p: ProbeData): string[] {
  const out: string[] = [];
  if (p.conflicts.warp) out.push("Cloudflare WARP");
  if (p.conflicts.x3ui) out.push("3x-ui");
  if (p.conflicts.wgeasy) out.push("wg-easy");
  if (p.conflicts.tailscale) out.push("Tailscale");
  for (const i of p.conflicts.other_ifaces ?? []) out.push(`интерфейс ${i}`);
  return out;
}
