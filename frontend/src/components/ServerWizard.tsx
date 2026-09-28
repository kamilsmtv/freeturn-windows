import { useEffect, useRef, useState } from "react";
import { backend, EVENTS, onEvent, type ProbeData, type Profile, type VPSResult } from "../lib/api";
import { Button, Card, Spinner } from "./ui";
import { Field, Picker, TextArea, TextInput } from "./Field";
import { useBusyWhile } from "../lib/busy";
import { guard } from "../lib/effect";

/**
 * Мастер добавления сервера: доступ по SSH, установка, подключение.
 *
 * Появился из-за того, что разложенные по двум вкладкам профиль и сервер
 * читались как две несвязанные сущности: с вкладки сервера нельзя было
 * начать, а профиль требовал доступ по SSH, которого в нём нет. Здесь тот
 * же путь идёт сверху вниз одним экраном, а профиль остаётся для тонкой
 * настройки — как и был.
 *
 * Команды сервера работают с сохранённым профилем, поэтому заготовку
 * сохраняем на первом же шаге. Брошенный на полпути профиль удаляем, если
 * дело не дошло до установки: пустые записи в списке никому не нужны.
 */
export function ServerWizard({
  onDone,
  onCancel,
  onManual,
}: {
  onDone: (p: Profile) => void;
  onCancel: () => void;
  onManual: () => void;
}) {
  const [step, setStep] = useState(1);
  const [draft, setDraft] = useState<Profile | null>(null);
  const [savedId, setSavedId] = useState("");
  const [applied, setApplied] = useState(false);
  const [probe, setProbe] = useState<ProbeData | null>(null);

  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [note, setNote] = useState("");
  const [fingerprint, setFingerprint] = useState("");
  const [log, setLog] = useState<string[]>([]);
  const logBox = useRef<HTMLDivElement>(null);

  const api = backend();

  // Уйти из мастера можно не только «Отменой»: человек кликает по разделу
  // слева, и компонент просто исчезает. Заготовку, до установки так и не
  // дошедшую, убираем за собой - иначе в списке копятся пустые профили.
  const leftovers = useRef({ id: "", applied: false });
  leftovers.current = { id: savedId, applied };
  useEffect(() => () => {
    const { id, applied: done } = leftovers.current;
    if (id && !done) void backend()?.DeleteProfile(id);
  }, []);

  useEffect(() => guard("Wizard/init", () => {
    void api?.NewProfile("Мой сервер")?.then(setDraft);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), []);

  useEffect(() => guard("Wizard/log", () =>
    onEvent<string>(EVENTS.vpsLog, (l) => setLog((prev) => [...prev.slice(-200), l])),
  ), []);

  useEffect(() => guard("Wizard/scroll", () => {
    const box = logBox.current;
    if (box) box.scrollTop = box.scrollHeight;
  }), [log.length]);

  useBusyWhile("wizard", busy !== "", "Настройка сервера");

  if (!draft) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner size={20} />
      </div>
    );
  }

  const patch = (p: Partial<Profile>) => setDraft({ ...draft, ...p });
  const patchSSH = (p: Partial<Profile["ssh"]>) => setDraft({ ...draft, ssh: { ...draft.ssh, ...p } });
  const patchClient = (p: Partial<Profile["client"]>) =>
    setDraft({ ...draft, client: { ...draft.client, ...p } });
  const patchOpts = (p: Partial<Profile["opts"]>) =>
    setDraft({ ...draft, opts: { ...draft.opts, ...p } });

  const external = draft.serverBackend === "external";

  // Сохраняем перед каждой командой: сервер работает с тем, что на диске,
  // а не с тем, что человек только что набрал в форме.
  const save = async (): Promise<Profile | null> => {
    if (!api) return null;
    const saved = await api.SaveProfile(savedId ? { ...draft, id: savedId } : draft);
    setSavedId(saved.id);
    setDraft(saved);
    return saved;
  };

  const run = async (name: string, fn: (id: string) => Promise<VPSResult>) => {
    setBusy(name);
    setError("");
    setNote("");
    setFingerprint("");
    try {
      const saved = await save();
      if (!saved) return null;
      const r = await fn(saved.id);
      if (r.fingerprint) {
        setFingerprint(r.fingerprint);
        return null;
      }
      if (!r.ok) {
        setError(r.error || "не удалось выполнить команду");
        return null;
      }
      if (r.probe) setProbe(r.probe);
      if (r.profile) setDraft(r.profile);
      if (r.text) setNote(r.text);
      return r;
    } catch (e) {
      setError(String(e));
      return null;
    } finally {
      setBusy("");
    }
  };

  const cancel = async () => {
    // Установленный сервер и выданную им конфигурацию терять нельзя, а
    // пустую заготовку держать в списке незачем.
    if (savedId && !applied) await api?.DeleteProfile(savedId);
    onCancel();
  };

  const finish = async () => {
    const saved = await save();
    if (!saved) return;
    await api?.SetActiveProfile(saved.id);
    onDone(saved);
  };

  const sshReady = draft.ssh.ip.trim() !== "" && draft.ssh.username.trim() !== "";
  const vkReady =
    draft.client.provider === "direct" || draft.client.vkLink.trim() !== "";

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <header className="flex items-center gap-3.5 border-b border-zinc-200 px-8 py-5 dark:border-zinc-900">
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-xl font-semibold">Новый сервер</h1>
          <p className="text-[13px] text-zinc-500 dark:text-zinc-400">
            Шаг {step} из 3 — {["доступ по SSH", "установка", "подключение"][step - 1]}
          </p>
        </div>
        <Button onClick={cancel}>Отмена</Button>
      </header>

      <div className="scroll flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-8 py-5">
        <Steps step={step} />

        {step === 1 && (
          <Card title="Доступ к серверу">
            <p className="pb-3 text-sm text-zinc-500 dark:text-zinc-400">
              Нужен VPS с root-доступом. Приложение зайдёт на него по SSH и поставит серверную
              часть само — вручную на сервере делать ничего не придётся.
            </p>
            <div className="grid gap-3 md:grid-cols-2">
              <Field label="Название">
                <TextInput value={draft.name} onChange={(name) => patch({ name })} />
              </Field>
              <Field label="Адрес VPS" hint="IP или домен — тот, по которому вы заходите по SSH">
                <TextInput
                  value={draft.ssh.ip}
                  onChange={(ip) => patchSSH({ ip })}
                  placeholder="203.0.113.10"
                />
              </Field>
              <Field label="Пользователь">
                <TextInput value={draft.ssh.username} onChange={(username) => patchSSH({ username })} />
              </Field>
              <Field label="Порт SSH">
                <TextInput
                  value={String(draft.ssh.port || 22)}
                  onChange={(v) => patchSSH({ port: Number(v) || 22 })}
                />
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
              {draft.ssh.authType === "PASSWORD" ? (
                <Field label="Пароль">
                  <TextInput
                    type="password"
                    value={draft.ssh.password}
                    onChange={(password) => patchSSH({ password })}
                  />
                </Field>
              ) : (
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
              )}
            </div>

            {draft.ssh.authType === "SSH_KEY" && (
              <div className="pt-3">
                <Field label="Приватный ключ" hint="Содержимое файла целиком, включая строки BEGIN и END">
                  <TextArea
                    value={draft.ssh.sshKey}
                    onChange={(sshKey) => patchSSH({ sshKey })}
                    rows={4}
                    placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                  />
                </Field>
              </div>
            )}

            {draft.ssh.authType === "PASSWORD" && (
              <div className="grid gap-3 pt-3 md:grid-cols-2">
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
              </div>
            )}

            <div className="flex items-center gap-3 pt-4">
              <Button
                variant="primary"
                disabled={!sshReady || busy !== ""}
                busy={busy === "probe"}
                onClick={async () => {
                  const r = await run("probe", (id) => api!.VPSProbe(id));
                  if (r) setStep(2);
                }}
              >
                {busy === "probe" ? "Проверяю…" : "Проверить доступ"}
              </Button>
              <button className="text-sm text-zinc-500 underline hover:text-zinc-700" onClick={onManual}>
                У меня уже есть данные — заполнить профиль вручную
              </button>
            </div>
          </Card>
        )}

        {step === 2 && (
          <Card title="Что поставить на сервер">
            {probe?.installed && (
              <p className="mb-3 rounded-lg bg-amber-100 px-3 py-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
                На сервере уже стоит FreeTurn {probe.version || ""}. Можно забрать его параметры и
                не трогать настройку — или применить заново с полями ниже.
              </p>
            )}

            <div className="grid gap-3 md:grid-cols-2">
              <Field label="VPN на сервере">
                <Picker
                  value={external ? "external" : "new"}
                  onChange={(serverBackend) => patch({ serverBackend })}
                  options={[
                    { value: "new", label: "Поднять WireGuard силами FreeTurn" },
                    { value: "external", label: "Мой VPN (3x-ui, Amnezia, wg-easy)" },
                  ]}
                />
              </Field>
              {external ? (
                <Field label="Адрес своего VPN" hint="У 3x-ui и Xray обычно 127.0.0.1:443">
                  <TextInput
                    value={draft.proxyConnect}
                    onChange={(proxyConnect) => patch({ proxyConnect })}
                    placeholder="127.0.0.1:443"
                  />
                </Field>
              ) : (
                <Field label="Порт сервера" hint="Его будет слушать FreeTurn на VPS">
                  <TextInput
                    value={draft.proxyListen}
                    onChange={(proxyListen) => patch({ proxyListen })}
                  />
                </Field>
              )}
              <Field label="Обфускация" hint="Ключ сервер создаст сам и вернёт в профиль">
                <Picker
                  value={draft.opts.obfProfile}
                  onChange={(obfProfile) => patchOpts({ obfProfile })}
                  options={[
                    { value: "rtpopus3", label: "rtpopus3 (рекомендуется)" },
                    { value: "rtpopus2", label: "rtpopus2" },
                    { value: "rtpopus", label: "rtpopus" },
                    { value: "none", label: "Без обфускации" },
                  ]}
                />
              </Field>
              {external && (
                <Field label="Порт сервера" hint="Его будет слушать FreeTurn на VPS">
                  <TextInput
                    value={draft.proxyListen}
                    onChange={(proxyListen) => patch({ proxyListen })}
                  />
                </Field>
              )}
            </div>

            {external && (
              <p className="pt-3 text-xs text-zinc-500 dark:text-zinc-400">
                FreeTurn станет транспортом для вашего VPN и своих ключей не выдаст: конфигурацию
                клиента возьмите в его панели, а на этой машине понадобится клиент того же
                протокола — например v2rayN для 3x-ui.
              </p>
            )}

            <div className="flex flex-wrap items-center gap-2 pt-4">
              <Button
                variant="primary"
                disabled={busy !== ""}
                busy={busy === "apply"}
                onClick={async () => {
                  const r = await run("apply", (id) => api!.VPSApply(id));
                  if (r) {
                    setApplied(true);
                    setStep(3);
                  }
                }}
              >
                {busy === "apply" ? "Устанавливаю…" : "Установить и запустить"}
              </Button>
              {probe?.installed && (
                <Button
                  disabled={busy !== ""}
                  busy={busy === "params"}
                  onClick={async () => {
                    const r = await run("params", (id) => api!.VPSImportParams(id));
                    if (r) {
                      setApplied(true);
                      setStep(3);
                    }
                  }}
                >
                  Забрать параметры и не ставить
                </Button>
              )}
              <Button disabled={busy !== ""} onClick={() => setStep(1)}>
                Назад
              </Button>
            </div>
          </Card>
        )}

        {step === 3 && (
          <Card title="Подключение">
            <div className="grid gap-3 md:grid-cols-2">
              <Field label="Адрес сервера (peer)" hint="Заполнил сам сервер">
                <TextInput
                  value={draft.client.serverAddress}
                  onChange={(serverAddress) => patchClient({ serverAddress })}
                />
              </Field>
              <Field label="Как идти до сервера">
                <Picker
                  value={draft.client.provider || "vk"}
                  onChange={(provider) => patchClient({ provider })}
                  options={[
                    { value: "vk", label: "Через реле звонков" },
                    { value: "direct", label: "Напрямую (без реле)" },
                  ]}
                />
              </Field>
              {draft.client.provider !== "direct" && (
                <Field
                  label="Ссылка на звонок VK Calls"
                  hint="Создайте звонок и не завершайте его — по нему клиент получает TURN-данные"
                >
                  <TextInput
                    value={draft.client.vkLink}
                    onChange={(vkLink) => patchClient({ vkLink })}
                    placeholder="https://vk.ru/call/join/…"
                  />
                </Field>
              )}
            </div>

            <div className="mt-3 flex flex-col gap-1 rounded-xl border border-zinc-200 px-4 py-3 text-xs text-zinc-500 dark:border-zinc-800 dark:text-zinc-400">
              <span>
                Обфускация: {draft.opts.obfProfile}
                {draft.opts.obfKey ? " — ключ получен с сервера" : " — ключа нет"}
              </span>
              <span>
                {draft.client.wireGuardConfig.trim()
                  ? "Конфигурация WireGuard получена — туннель поднимет само приложение"
                  : "Конфигурации WireGuard нет: трафик уходит в ваш VPN, клиент запускаете сами"}
              </span>
            </div>

            <div className="flex items-center gap-2 pt-4">
              <Button variant="primary" disabled={!vkReady || busy !== ""} onClick={finish}>
                Готово
              </Button>
              <Button disabled={busy !== ""} onClick={() => setStep(2)}>
                Назад
              </Button>
              {!vkReady && (
                <span className="text-xs text-zinc-500">
                  Без ссылки на звонок подключение не поднимется — либо выберите «Напрямую»
                </span>
              )}
            </div>
          </Card>
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
                  if (savedId) await api?.TrustHostKey(savedId, fingerprint);
                  setFingerprint("");
                  setNote("Ключ сервера сохранён — повторите проверку");
                }}
              >
                Доверять этому ключу
              </Button>
            </div>
          </div>
        )}

        {error && (
          <p className="whitespace-pre-wrap rounded-xl bg-red-100 px-4 py-3 text-sm text-red-900 dark:bg-red-950 dark:text-red-200">
            {error}
          </p>
        )}
        {note && !error && (
          <p className="whitespace-pre-wrap rounded-xl bg-emerald-100 px-4 py-3 text-sm text-emerald-900 dark:bg-emerald-950 dark:text-emerald-200">
            {note}
          </p>
        )}

        {log.length > 0 && (
          <Card title="Ход выполнения">
            <div
              ref={logBox}
              className="scroll max-h-52 overflow-y-auto rounded-lg bg-zinc-950 p-3 font-mono text-xs text-zinc-300"
            >
              {log.map((l, i) => (
                <div key={i} className="whitespace-pre-wrap">
                  {l}
                </div>
              ))}
            </div>
          </Card>
        )}
      </div>
    </div>
  );
}

/** Полоска шагов: видно, сколько осталось. */
function Steps({ step }: { step: number }) {
  const titles = ["Доступ", "Установка", "Подключение"];
  return (
    <div className="flex items-center gap-2">
      {titles.map((t, i) => (
        <div key={t} className="flex flex-1 items-center gap-2">
          <span
            className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs ${
              i + 1 <= step
                ? "bg-emerald-600 text-white"
                : "bg-zinc-200 text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400"
            }`}
          >
            {i + 1}
          </span>
          <span
            className={`text-[13px] ${
              i + 1 === step ? "font-medium" : "text-zinc-500 dark:text-zinc-400"
            }`}
          >
            {t}
          </span>
          {i < titles.length - 1 && <span className="h-px flex-1 bg-zinc-200 dark:bg-zinc-800" />}
        </div>
      ))}
    </div>
  );
}
