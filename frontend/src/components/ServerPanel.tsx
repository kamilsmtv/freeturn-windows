import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  backend,
  EVENTS,
  onEvent,
  type Profile,
  type ProbeData,
  type VPSClient,
  type VPSResult,
} from "../lib/api";
import { Button, Spinner } from "./ui";
import { Field, NumberInput, Picker, TextArea, TextInput } from "./Field";
import { validClientName } from "../lib/format";
import { useBusyWhile } from "../lib/busy";
import { guard } from "../lib/effect";
import { QRCode } from "./QRCode";

/**
 * Управление сервером по SSH. Все команды идут через install.sh ядра:
 * скрипт скачивается приложением и передаётся на stdin bash, обратно
 * приходит один JSON-объект (протокол v3 самого install.sh).
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
  const [clients, setClients] = useState<VPSClient[] | null>(null);
  // Ссылка уже заведённого гостя: завести второй раз сервер не даёт, а
  // показать выданное раньше было нечем.
  const [peerLink, setPeerLink] = useState<{ name: string; url: string } | null>(null);
  // Форма доступа занимает пол-экрана, поэтому у подтверждённого сервера
  // она свёрнута: разворачиваем по клику, как в макете.
  const [sshOpen, setSshOpen] = useState(!profile.ssh.hostFingerprint);
  const logBox = useRef<HTMLDivElement>(null);
  const api = backend();

  useEffect(() => guard("ServerPanel/vpsLog", () =>
    onEvent<string>(EVENTS.vpsLog, (l) => setLog((prev) => [...prev.slice(-400), l])),
  ), []);
  // Смена активного профиля должна подхватываться формой доступа.
  //
  // Заодно сбрасываем всё, что относится к прежнему серверу: список
  // доступов и ссылка гостя от другого сервера - это не просто мусор на
  // экране, по ним можно раздать доступ не туда.
  useEffect(() => guard("ServerPanel/profile", () => {
    setDraft(profile);
    setSaved(true);
    setSshOpen(!profile.ssh.hostFingerprint);
    setProbe(null);
    setClients(null);
    setShare("");
    setPeerName("");
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
  //
  // Заодно подтягиваем список выданных доступов. Раньше он ждал нажатия
  // кнопки, и шаг выглядел пустым - человек не догадывался, что список
  // вообще существует, и спрашивал, где посмотреть, кому он раздал доступ.
  const refreshProbe = async () => {
    try {
      const r = await api?.VPSProbe(profile.id);
      if (r?.ok && r.probe) {
        setProbe(r.probe);
        if (r.probe.installed) {
          try {
            const list = await api?.VPSClients(profile.id);
            setClients(list?.clients ?? []);
          } catch {
            // Список - дополнение к состоянию, без него вкладка работает.
          }
        }
      }
    } catch {
      // Молчим намеренно: это фоновая проверка, а не действие пользователя.
    }
  };

  // Обновление списка по кнопке: после выдачи или отзыва доступа.
  const refreshClients = async () => {
    setError("");
    setBusy("clients");
    try {
      const r = await api?.VPSClients(profile.id);
      setClients(r?.clients ?? []);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy("");
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
  // Режим проброса tcp решает за пользователя: свой WireGuard скрипт
  // поднимает только для udp, и выбор тут ни на что не влияет.
  const external = draft.serverBackend === "external" || draft.opts.proxyMode === "tcp";

  const access = `${draft.ssh.username || "root"}@${draft.ssh.ip}:${draft.ssh.port || 22}`;
  const subtitle = [
    draft.ssh.ip.trim() || "адрес не задан",
    probe ? (probe.installed ? `ядро ${probe.version || "?"} ${probe.running ? "работает" : "остановлено"}` : "ядро не установлено") : "состояние неизвестно",
    probe?.method || "",
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
        <HeaderButton
          disabled={disabled}
          busy={busy === "probe"}
          onClick={() => run("probe", () => api!.VPSProbe(profile.id))}
          icon={
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="text-zinc-500 dark:text-zinc-400">
              <path d="M21 12a9 9 0 1 1-3-6.7" />
              <path d="M21 4v5h-5" />
            </svg>
          }
        >
          {busy === "probe" ? "Опрашиваю…" : "Проверить"}
        </HeaderButton>
        <HeaderButton
          disabled={disabled}
          busy={busy === "logs"}
          onClick={() => run("logs", () => api!.VPSLogs(profile.id, 120))}
          icon={
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="text-zinc-500 dark:text-zinc-400">
              <path d="M4 6h16M4 12h16M4 18h10" />
            </svg>
          }
        >
          {busy === "logs" ? "Читаю журнал…" : "Журнал сервера"}
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
            done
            title="Куда идёт трафик"
            hint={
              external
                ? "FreeTurn только передаёт трафик в ваш VPN и сам его не трогает"
                : "Установщик поднимет WireGuard (ft-wg0) и сам выпустит конфигурации"
            }
          >
            <div className="flex flex-col gap-3 pt-2 md:max-w-lg">
              {/* В режиме проброса tcp выбора нет: свой WireGuard скрипт
                  поднимает только для udp. Показывать при этом список,
                  который всё равно не слушается, - обман. */}
              {draft.opts.proxyMode === "tcp" ? (
                <Field
                  label="VPN на сервере"
                  hint="Режим туннеля tcp работает только поверх своего VPN — выбора нет"
                >
                  <div className="flex h-[42px] items-center rounded-lg border border-zinc-300 px-3.5 text-sm text-zinc-500 dark:border-zinc-700 dark:text-zinc-400">
                    Мой VPN (3x-ui, Xray, sing-box)
                  </div>
                </Field>
              ) : (
                <Field label="VPN на сервере">
                  <Picker
                    value={draft.serverBackend === "external" ? "external" : "new"}
                    onChange={(serverBackend) => patchProfile({ serverBackend })}
                    options={[
                      { value: "new", label: "Поднять WireGuard силами FreeTurn" },
                      { value: "external", label: "Мой VPN (3x-ui, Amnezia, wg-easy, Xray)" },
                    ]}
                  />
                </Field>
              )}

              {external ? (
                <>
                  <Field
                    label="Адрес своего VPN на сервере"
                    hint="Флаг -connect: у 3x-ui и Xray обычно 127.0.0.1:443, у WireGuard 127.0.0.1:51820"
                  >
                    <TextInput
                      value={draft.proxyConnect}
                      onChange={(proxyConnect) => patchProfile({ proxyConnect })}
                      placeholder="127.0.0.1:443"
                    />
                  </Field>
                  <p className="text-xs text-zinc-500 dark:text-zinc-400">
                    Клиентские конфигурации берите в панели своего VPN: FreeTurn своих ключей не
                    выдаёт никому, включая вас. На этой машине понадобится клиент того же
                    протокола — например v2rayN для 3x-ui.
                  </p>

                  {/* Скрипт не даёт сменить бэкенд, пока его WireGuard на месте. */}
                  {probe?.config?.backend === "new" && (
                    <div className="flex flex-col gap-2 rounded-xl border border-amber-300 bg-amber-50 p-3 dark:border-[#422006] dark:bg-[#1c1917]">
                      <span className="text-xs text-amber-900 dark:text-amber-300">
                        На сервере поднят WireGuard от FreeTurn — пока он есть, трафик в ваш VPN не
                        уйдёт. Удалить его вместе с выданными им конфигурациями?
                      </span>
                      <div>
                        <Button
                          disabled={disabled}
                          busy={busy === "dropwg"}
                          onClick={() => run("dropwg", () => api!.VPSRemoveWireGuard(profile.id))}
                        >
                          {busy === "dropwg" ? "Удаляю…" : "Удалить наш WireGuard с сервера"}
                        </Button>
                      </div>
                    </div>
                  )}
                </>
              ) : (
                <p className="text-xs text-zinc-500 dark:text-zinc-400">
                  Конфигурации выпустит сам сервер — и вам, и тем, с кем поделитесь. Уже
                  работающий на сервере VPN при этом не трогается.
                </p>
              )}

              <div>
                <Button variant={saved ? "default" : "primary"} disabled={!canSave} onClick={saveDraft}>
                  {saved ? "Сохранено" : "Сохранить"}
                </Button>
              </div>
            </div>
          </Step>

          <Step
            done={Boolean(probe?.installed)}
            title={probe?.installed ? "Сервер установлен" : "Установить и запустить"}
            hint={
              probe?.installed
                ? `${probe.version || "версия неизвестна"} · ${probe.running ? "служба работает" : "служба остановлена"}` +
                  `${probe.config?.backend === "new" ? " · WireGuard поднят скриптом" : ""}`
                : "Одна команда ставит ядро, поднимает WireGuard и отдаёт конфигурацию владельца в профиль"
            }
          >
            <div className="flex flex-wrap gap-2 pt-2">
              <Button
                variant="primary"
                disabled={disabled}
                onClick={() => run("apply", () => api!.VPSApply(profile.id))}
                busy={busy === "apply"}
              >
                {busy === "apply"
                  ? "Настраиваю…"
                  : probe?.installed
                    ? "Применить настройки заново"
                    : "Установить и запустить"}
              </Button>
              <Button
                disabled={disabled}
                busy={busy === "params"}
                onClick={() => run("params", () => api!.VPSImportParams(profile.id))}
              >
                {busy === "params" ? "Спрашиваю сервер…" : "Забрать параметры подключения"}
              </Button>
              {!external && (
                <Button
                  disabled={disabled}
                  busy={busy === "wg"}
                  onClick={() => run("wg", () => api!.VPSFetchWGConfig(profile.id))}
                >
                  {busy === "wg" ? "Забираю…" : "Забрать конфигурацию владельца"}
                </Button>
              )}
            </div>

          </Step>

          <Step
            done={false}
            title="Пригласить гостя"
            hint={
              external
                ? "Сервер заведёт доступ и выдаст Client ID, но своих ключей у него нет"
                : "Сервер заведёт доступ, выдаст Client ID и соберёт готовую ссылку"
            }
          >
            <div className="flex flex-col gap-2 pt-2">
              {/* При своём VPN ссылка неполная, и узнать это после отправки
                  гостю - худший момент из возможных. */}
              {external && (
                <p className="rounded-lg bg-amber-100 px-3 py-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
                  Трафик уходит в ваш VPN, поэтому в ссылке будут только адрес сервера, обфускация
                  и Client ID — без конфигурации WireGuard. Гостю отдельно нужен доступ к самому
                  VPN: например, ссылка из 3x-ui. Одной этой ссылки ему не хватит.
                </p>
              )}
              <div className="flex items-center gap-2">
                <TextInput
                  value={peerName}
                  onChange={setPeerName}
                  placeholder="Имя гостя, латиницей"
                />
                <Button
                  variant="primary"
                  disabled={disabled || !validClientName(peerName)}
                  busy={busy === "peer"}
                  onClick={async () => {
                    setError("");
                    setBusy("peer");
                    try {
                      setShare(await api!.VPSAddPeer(profile.id, peerName.trim()));
                    } catch (e) {
                      setError(String(e));
                    } finally {
                      setBusy("");
                    }
                  }}
                >
                  Создать ссылку
                </Button>
              </div>
              {peerName.trim() !== "" && !validClientName(peerName) && (
                <span className="text-xs text-amber-700 dark:text-amber-500">
                  Сервер принимает только латиницу, цифры, точку, дефис и подчёркивание — до 32
                  символов, первым идёт буква или цифра
                </span>
              )}
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

          <Step
            done={Boolean(clients && clients.length > 0)}
            title="Кому выдан доступ"
            hint={
              clients
                ? `Всего ${plural(clients.length)} · сервер помнит, кому выдана конфигурация и когда тот выходил на связь`
                : "Сервер помнит, кому выдана конфигурация и когда тот выходил на связь"
            }
          >
            <div className="flex flex-col gap-2 pt-2">
              {clients === null && (
                <span className="text-xs text-zinc-500 dark:text-zinc-400">
                  Список появится после проверки сервера
                </span>
              )}

              {clients?.length === 0 && (
                <span className="text-xs text-zinc-500 dark:text-zinc-400">
                  Сервер не выдал ни одного доступа
                </span>
              )}

              {clients?.map((c) => (
                <div
                  key={c.name}
                  className="flex items-center gap-2 rounded-lg border border-zinc-200 px-3 py-2 dark:border-zinc-800"
                >
                  {/* Имя и адрес - в столбик, кнопки - справа. Одной
                      строкой кнопки вытесняли имя до нуля: flex-1 отдаёт
                      место тому, что не сжимается, и truncate обрезал его
                      целиком. */}
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-[13px]">
                      {c.name}
                      {c.self && <span className="ml-2 text-xs text-emerald-600">это вы</span>}
                    </div>
                    {/* При своём VPN сервер не выдаёт ни адреса, ни ключей:
                        доступ там - это только Client ID в allowlist. */}
                    <div className="text-xs text-zinc-500 dark:text-zinc-400">
                      {c.pub ? (
                        <>
                          <span className="font-mono">{c.ip}</span> · {handshake(c.hs)}
                        </>
                      ) : (
                        "только Client ID"
                      )}
                    </div>
                  </div>
                  {!c.self && (
                    <div className="flex shrink-0 gap-2">
                      <Button
                        disabled={disabled}
                        busy={busy === "link-" + c.name}
                        onClick={async () => {
                          setError("");
                          setBusy("link-" + c.name);
                          try {
                            const url = await api!.VPSPeerLink(profile.id, c.name);
                            setPeerLink({ name: c.name, url });
                          } catch (e) {
                            setError(String(e));
                          } finally {
                            setBusy("");
                          }
                        }}
                      >
                        Ссылка
                      </Button>
                      <Button
                        disabled={disabled}
                        busy={busy === "drop-" + c.name}
                        onClick={async () => {
                          await run("drop-" + c.name, () => api!.VPSRemovePeer(profile.id, c.name));
                          setPeerLink(null);
                          await refreshClients();
                        }}
                      >
                        Отозвать
                      </Button>
                    </div>
                  )}
                </div>
              ))}

              <div className="pt-1">
                <Button disabled={disabled} busy={busy === "clients"} onClick={refreshClients}>
                  {busy === "clients" ? "Спрашиваю сервер…" : "Обновить список"}
                </Button>
              </div>

              {peerLink && (
                <div className="flex flex-col gap-2 rounded-xl border border-emerald-300 bg-emerald-50 p-3 dark:border-emerald-900 dark:bg-emerald-950/30">
                  <span className="text-[13px] font-medium">Ссылка для «{peerLink.name}»</span>
                  <QRCode text={peerLink.url} name={peerLink.name} />
                  <pre className="scroll selectable max-h-24 overflow-auto rounded-lg bg-zinc-950 p-2 font-mono text-xs text-zinc-300">
                    {peerLink.url}
                  </pre>
                  <div className="flex gap-2">
                    <Button onClick={() => navigator.clipboard.writeText(peerLink.url).catch(() => {})}>
                      Скопировать
                    </Button>
                    <Button variant="ghost" onClick={() => setPeerLink(null)}>
                      Скрыть
                    </Button>
                  </div>
                </div>
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
              <Button
                disabled={disabled}
                busy={busy === "restart"}
                onClick={() => run("restart", () => api!.VPSRestart(profile.id))}
              >
                {busy === "restart" ? "Перезапускаю…" : "Перезапустить"}
              </Button>
              <Button
                disabled={disabled}
                busy={busy === "stop"}
                onClick={() => run("stop", () => api!.VPSStop(profile.id))}
              >
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

          <section className="flex flex-col gap-2.5 rounded-2xl border border-zinc-200 bg-white p-[18px] dark:border-zinc-800 dark:bg-[#0c0c0e]">
            <div className="flex items-center gap-2">
              <h2 className="flex-1 text-[13px] font-semibold uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
                Ход выполнения
              </h2>
              <span className="text-xs text-zinc-400 dark:text-zinc-600">install.sh · протокол v3</span>
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
/** «3 доступа» вместо «3 доступ». */
function plural(n: number): string {
  const forms = ["доступ", "доступа", "доступов"];
  const mod100 = n % 100;
  const mod10 = n % 10;
  let form = forms[2];
  if (mod100 < 11 || mod100 > 14) {
    if (mod10 === 1) form = forms[0];
    else if (mod10 >= 2 && mod10 <= 4) form = forms[1];
  }
  return `${n} ${form}`;
}

/** Когда доступ последний раз выходил на связь; 0 — ни разу. */
function handshake(hs: number): string {
  if (!hs) return "не подключался";
  const mins = Math.round((Date.now() / 1000 - hs) / 60);
  if (mins < 1) return "только что";
  if (mins < 60) return `${mins} мин назад`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours} ч назад`;
  return `${Math.round(hours / 24)} дн назад`;
}

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
  icon,
  children,
  onClick,
  disabled,
  busy,
}: {
  icon: ReactNode;
  children: ReactNode;
  onClick: () => void;
  disabled?: boolean;
  busy?: boolean;
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled || busy}
      className="flex h-9 shrink-0 items-center gap-2 rounded-lg border border-zinc-200 bg-white px-4 text-[13px] transition hover:bg-zinc-100 disabled:pointer-events-none disabled:opacity-40 dark:border-zinc-800 dark:bg-zinc-900 dark:hover:bg-zinc-800"
    >
      {/* Пока идёт команда, значок уступает место колечку - кнопка не прыгает. */}
      {busy ? <Spinner /> : icon}
      {children}
    </button>
  );
}

