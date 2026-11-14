import { useEffect, useState } from "react";
import { backend, type Profile, type SplitShared } from "../lib/api";
import { Button, Card, Toggle } from "./ui";
import { CheckBox, Field, NumberInput, Picker, TextArea, TextInput } from "./Field";

/**
 * Экран профиля: основные поля сверху, всё остальное - в «Дополнительно».
 * Дефолты взяты из quickstart ядра, поэтому обычный сценарий закрывается
 * тремя полями: адрес сервера, ссылка на звонок и ключ обфускации.
 */
export function ProfileEditor({
  profile,
  onChange,
  onSave,
  onCancel,
  onOpenServer,
}: {
  profile: Profile;
  onChange: (p: Profile) => void;
  onSave: () => void;
  onCancel: () => void;
  // onOpenServer уводит на вкладку сервера, сохранив правки: без доступа
  // по SSH половина кнопок профиля всё равно не работает.
  onOpenServer: () => void;
}) {
  const [advanced, setAdvanced] = useState(false);
  const [wgPreview, setWgPreview] = useState("");
  const [wgPublicKey, setWgPublicKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [wgError, setWgError] = useState("");
  const api = backend();

  // Общие правила раздельного туннелирования живут вне профиля, но
  // правятся здесь же: держим их черновик рядом с черновиком профиля и
  // сохраняем вместе с ним - «Отмена» откатывает и то и другое.
  const [shared, setShared] = useState<SplitShared | null>(null);
  const [sharedDraft, setSharedDraft] = useState({ mode: "all", subnets: "" });
  const [sharedError, setSharedError] = useState("");
  const [wasShared] = useState(profile.client.splitTunnelShared);
  useEffect(() => {
    api
      ?.GetSplitShared()
      .then((s) => {
        setShared(s);
        if (s.exists) setSharedDraft({ mode: s.mode, subnets: s.subnets });
      })
      // Без ответа считаем, что набора нет: иначе галочка не работала бы вовсе.
      .catch(() => setShared({ exists: false, mode: "all", subnets: "", users: 0, total: 0 }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const useShared = profile.client.splitTunnelShared;
  const split = useShared
    ? { mode: sharedDraft.mode, subnets: sharedDraft.subnets }
    : { mode: profile.client.splitTunnelMode, subnets: profile.client.splitTunnelSubnets };
  const setSplit = (patch: { mode?: string; subnets?: string }) => {
    if (useShared) {
      setSharedDraft((d) => ({ ...d, ...patch }));
      return;
    }
    client({
      ...(patch.mode !== undefined && { splitTunnelMode: patch.mode }),
      ...(patch.subnets !== undefined && { splitTunnelSubnets: patch.subnets }),
    });
  };
  // Как в Android: включение на профиле без набора заводит набор из его
  // правил, выключение оставляет профилю копию общих - чтобы маршруты не
  // поменялись от одного щелчка.
  const toggleShared = (on: boolean) => {
    // До ответа бэкенда неизвестно, есть ли набор: включение завело бы
    // его из правил профиля поверх настоящего.
    if (!shared) return;
    if (on) {
      if (!shared?.exists) {
        setSharedDraft({ mode: profile.client.splitTunnelMode, subnets: profile.client.splitTunnelSubnets });
      }
      client({ splitTunnelShared: true });
    } else {
      client({ splitTunnelShared: false, splitTunnelMode: sharedDraft.mode, splitTunnelSubnets: sharedDraft.subnets });
    }
  };
  const sharedUsers = (shared?.users ?? 0) - (wasShared ? 1 : 0) + (useShared ? 1 : 0);
  const sharedTotal = Math.max(shared?.total ?? 0, sharedUsers);

  // commit сохраняет общие правила раньше профиля: если подсети не
  // разобрались, профиль не должен сохраниться ссылкой на битый набор.
  const commit = (next: () => void) => async () => {
    setSharedError("");
    if (useShared) {
      try {
        await api?.SetSplitShared(sharedDraft.mode, sharedDraft.subnets);
      } catch (e) {
        setSharedError(String(e));
        setAdvanced(true);
        return;
      }
    }
    next();
  };

  const client = (patch: Partial<Profile["client"]>) =>
    onChange({ ...profile, client: { ...profile.client, ...patch } });
  const opts = (patch: Partial<Profile["opts"]>) =>
    onChange({ ...profile, opts: { ...profile.opts, ...patch } });

  // В режиме direct реле не участвует, и всё, что его касается, ядро
  // игнорирует - показывать такие поля значит обещать несбыточное.
  const direct = profile.client.provider === "direct";
  const tcp = profile.opts.proxyMode === "tcp";

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <header className="flex items-center gap-3.5 border-b border-zinc-200 px-8 py-5 dark:border-zinc-900">
        <button
          onClick={onCancel}
          title="Назад"
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-zinc-500 transition hover:bg-zinc-200 dark:text-zinc-400 dark:hover:bg-zinc-800"
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <path d="M15 5l-7 7 7 7" />
          </svg>
        </button>
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-lg font-semibold">{profile.name || "Новый профиль"}</h1>
          <p className="text-xs text-zinc-500 dark:text-zinc-400">Профиль подключения</p>
        </div>
        <Button onClick={onCancel}>Отмена</Button>
        <Button variant="primary" onClick={commit(onSave)}>
          Сохранить
        </Button>
      </header>

      <div className="scroll flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-8 py-5">
      <Card title="Основное">
        <div className="grid gap-3 md:grid-cols-2">
          <Field label="Название">
            <TextInput value={profile.name} onChange={(name) => onChange({ ...profile, name })} />
          </Field>
          <Field label="Адрес сервера (peer)" hint="host:port, например 1.2.3.4:56000">
            <TextInput
              value={profile.client.serverAddress}
              onChange={(serverAddress) => client({ serverAddress })}
              placeholder="1.2.3.4:56000"
            />
          </Field>
          <Field
            label="Как идти до сервера"
            hint={
              direct
                ? "Напрямую на адрес сервера. Не сработает, если оператор его блокирует"
                : "Потоки идут через TURN-серверы звонков — обходит блокировку адреса"
            }
          >
            <Picker
              value={profile.client.provider || "vk"}
              onChange={(provider) => client({ provider })}
              options={[
                { value: "vk", label: "Через реле звонков" },
                { value: "direct", label: "Напрямую (без реле)" },
              ]}
            />
          </Field>
          {!direct && (
            <Field
              label="Ссылка на звонок VK Calls"
              hint="Создайте звонок и не завершайте его — по нему клиент получает TURN-данные"
            >
              <TextInput
                value={profile.client.vkLink}
                onChange={(vkLink) => client({ vkLink })}
                placeholder="https://vk.ru/call/join/…"
              />
            </Field>
          )}
          <Field label="Локальный адрес" hint="Сюда подключается WireGuard или Xray">
            <TextInput value={profile.client.localPort} onChange={(localPort) => client({ localPort })} />
          </Field>
        </div>
      </Card>

      <Card title="Обфускация">
        <div className="grid gap-3 md:grid-cols-2">
          <Field label="Профиль" hint="Должен совпадать с сервером">
            <Picker
              value={profile.opts.obfProfile}
              onChange={(obfProfile) => opts({ obfProfile })}
              options={[
                { value: "none", label: "Без обфускации" },
                { value: "rtpopus", label: "rtpopus" },
                { value: "rtpopus2", label: "rtpopus2" },
                { value: "rtpopus3", label: "rtpopus3 (рекомендуется)" },
              ]}
            />
          </Field>
          <Field label="Ключ" hint="64 hex-символа, общий с сервером">
            <div className="flex items-center gap-2">
              <TextInput value={profile.opts.obfKey} onChange={(obfKey) => opts({ obfKey })} mono />
              <Button onClick={async () => opts({ obfKey: (await api?.GenerateObfKey()) ?? "" })}>
                Создать
              </Button>
            </div>
          </Field>
        </div>
      </Card>

      <Card title="Режим работы">
        <div className="flex gap-2.5">
          <button
            onClick={() => client({ tunnelTransport: "wireguard" })}
            className={`flex flex-1 items-center gap-3 rounded-xl border px-4 py-3.5 text-left transition ${
              profile.client.tunnelTransport === "wireguard"
                ? "border-emerald-500 bg-emerald-50 dark:bg-[#0f1a15]"
                : "border-zinc-200 bg-white hover:border-zinc-300 dark:border-zinc-800 dark:bg-zinc-900"
            }`}
          >
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.7"
              strokeLinecap="round"
              strokeLinejoin="round"
              className={
                profile.client.tunnelTransport === "wireguard"
                  ? "text-emerald-600 dark:text-emerald-500"
                  : "text-zinc-400"
              }
            >
              <path d="M12 3 4 6v6c0 4.5 3.2 7.9 8 9 4.8-1.1 8-4.5 8-9V6z" />
            </svg>
            <span className="flex flex-col gap-0.5">
              <span className="text-sm font-medium">VPN — весь трафик</span>
              <span className="text-xs text-zinc-500 dark:text-zinc-400">
                Туннель поднимает само приложение
              </span>
            </span>
          </button>

          <button
            onClick={() => client({ tunnelTransport: "none" })}
            className={`flex flex-1 items-center gap-3 rounded-xl border px-4 py-3.5 text-left transition ${
              profile.client.tunnelTransport !== "wireguard"
                ? "border-emerald-500 bg-emerald-50 dark:bg-[#0f1a15]"
                : "border-zinc-200 bg-white hover:border-zinc-300 dark:border-zinc-800 dark:bg-zinc-900"
            }`}
          >
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.7"
              strokeLinecap="round"
              strokeLinejoin="round"
              className={
                profile.client.tunnelTransport !== "wireguard"
                  ? "text-emerald-600 dark:text-emerald-500"
                  : "text-zinc-400"
              }
            >
              <path d="M4 12h16M14 6l6 6-6 6" />
            </svg>
            <span className="flex flex-col gap-0.5">
              <span className="text-sm font-medium">Прокси — локальный порт</span>
              <span className="text-xs text-zinc-500 dark:text-zinc-400">
                Для Xray, sing-box или своего клиента
              </span>
            </span>
          </button>
        </div>

        {/* Режим прокси часто выбирают ради связки с 3x-ui, и тут важно
            сказать то, о чём иначе узнают опытным путём: своего туннеля не
            будет, клиент нужен отдельный. Android-клиент пишет то же самое. */}
        {profile.client.tunnelTransport !== "wireguard" && (
          <div className="mt-3 flex flex-col gap-1 rounded-xl border border-zinc-200 px-4 py-3 text-xs text-zinc-500 dark:border-zinc-800 dark:text-zinc-400">
            <span>
              {tcp
                ? "TCP-форвардер: локальный порт слушает Xray или sing-box, свой туннель не поднимается. Конфигурацию возьмите в своей панели (например 3x-ui) и замените в ней адрес сервера на локальный адрес выше."
                : "UDP-релей: локальный порт принимает трафик WireGuard. Свой туннель приложение не поднимает — клиент запускаете сами."}
            </span>
            <span>
              Если VPN на этой машине держит другая программа, исключите из её туннеля адрес
              сервера — иначе трафик зациклится.
            </span>
          </div>
        )}

        {profile.client.tunnelTransport === "wireguard" && (
          <div className="mt-3">
            {profile.client.wireGuardConfig.trim() ? (
              <div className="flex items-center gap-3 rounded-xl border border-emerald-300 bg-emerald-50 px-4 py-3 dark:border-emerald-900 dark:bg-emerald-950/30">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="text-emerald-600 dark:text-emerald-500">
                  <path d="M4 12.5 9.5 18 20 6.5" />
                </svg>
                <span className="flex-1 text-[13px]">Конфигурация WireGuard есть — туннель поднимет приложение</span>
                <Button
                  disabled={busy}
                  busy={busy}
                  onClick={async () => {
                    setWgError("");
                    setWgPreview("");
                    setBusy(true);
                    try {
                      const r = await api?.VPSFetchWGConfig(profile.id);
                      if (r?.ok && r.profile) onChange(r.profile);
                      else if (r && !r.ok) setWgError(r.error);
                    } catch (e) {
                      setWgError(String(e));
                    } finally {
                      setBusy(false);
                    }
                  }}
                >
                  Запросить заново
                </Button>
              </div>
            ) : (
              <div className="flex items-center gap-3 rounded-xl border border-amber-300 bg-amber-50 px-4 py-3 dark:border-amber-900 dark:bg-amber-950/30">
                <span className="flex-1 text-[13px] text-amber-900 dark:text-amber-200">
                  Для VPN нужна конфигурация клиента — её выдаёт сервер
                </span>
                <Button
                  variant="primary"
                  disabled={busy}
                  busy={busy}
                  onClick={async () => {
                    setWgError("");
                    setBusy(true);
                    try {
                      const r = await api?.VPSFetchWGConfig(profile.id);
                      if (r?.ok && r.profile) onChange(r.profile);
                      else if (r && !r.ok) setWgError(r.error);
                    } catch (e) {
                      setWgError(String(e));
                    } finally {
                      setBusy(false);
                    }
                  }}
                >
                  Запросить у сервера
                </Button>
              </div>
            )}
            {wgError && (
              <div className="flex flex-col gap-2 pt-2">
                <p className="text-sm text-red-600 dark:text-red-400">{wgError}</p>
                {needsSSH(wgError) && (
                  <div>
                    <Button variant="primary" onClick={commit(onOpenServer)}>
                      Заполнить доступ по SSH
                    </Button>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </Card>

      <button
        onClick={() => setAdvanced((v) => !v)}
        className="self-start text-sm text-zinc-500 underline hover:text-zinc-700 dark:hover:text-zinc-300"
      >
        {advanced ? "Скрыть дополнительные параметры" : "Дополнительно"}
      </button>

      {advanced && (
        <>
          <Card title="Режим и транспорт">
            <div className="grid gap-3 md:grid-cols-3">
              <Field label="Режим туннеля (-mode)" hint="udp — WireGuard, tcp — Xray/sing-box">
                <Picker
                  value={profile.opts.proxyMode}
                  onChange={(proxyMode) => opts({ proxyMode })}
                  options={[
                    { value: "udp", label: "udp" },
                    { value: "tcp", label: "tcp" },
                  ]}
                />
              </Field>
              {!direct && (
                <Field label="Транспорт до TURN (-transport)">
                  <Picker
                    value={profile.client.useUdp ? "udp" : "tcp"}
                    onChange={(v) => client({ useUdp: v === "udp" })}
                    options={[
                      { value: "tcp", label: "tcp (по умолчанию)" },
                      { value: "udp", label: "udp" },
                    ]}
                  />
                </Field>
              )}
              <Field label="Пейсинг мимикрии, мс (-obf-timing)" hint="0 — выключен, максимум 60">
                <NumberInput
                  value={profile.opts.obfTimingMs}
                  min={0}
                  max={60}
                  onChange={(obfTimingMs) => opts({ obfTimingMs })}
                />
              </Field>
              <Field
                label={direct ? "Потоков (-n)" : "Потоков TURN (-n)"}
                hint={direct ? "Дефолт ядра для direct — 1" : "Дефолт ядра — 12"}
              >
                <NumberInput value={profile.client.threads} min={1} onChange={(threads) => client({ threads })} />
              </Field>
              {!direct && (
                <Field label="Потоков на учётку (-streams-per-cred)" hint="Дефолт ядра — 12">
                  <NumberInput
                    value={profile.client.streamsPerCred}
                    min={1}
                    onChange={(streamsPerCred) => client({ streamsPerCred })}
                  />
                </Field>
              )}
              <Field label="Client ID" hint="Владелец сервера добавляет его в allowlist">
                <div className="flex items-center gap-2">
                  <TextInput value={profile.client.clientId} onChange={(clientId) => client({ clientId })} mono />
                  <Button onClick={async () => client({ clientId: (await api?.GenerateClientID()) ?? "" })}>
                    Создать
                  </Button>
                </div>
              </Field>
            </div>
          </Card>

          <Card title="DNS и маршруты">
            <div className="grid gap-3 md:grid-cols-2">
              <Field label="Режим DNS (-dns-mode)">
                <Picker
                  value={profile.client.dnsMode}
                  onChange={(dnsMode) => client({ dnsMode })}
                  options={[
                    { value: "auto", label: "auto" },
                    { value: "plain", label: "plain (UDP/53)" },
                    { value: "doh", label: "doh" },
                  ]}
                />
              </Field>
              <Field label="Свои DNS (-dns-servers)" hint="Через запятую: 1.1.1.1, 8.8.8.8">
                <TextInput value={profile.client.customDns} onChange={(customDns) => client({ customDns })} />
              </Field>
            </div>
            <div className="flex flex-col gap-2 pt-3">
              <CheckBox
                checked={profile.client.routes}
                onChange={(routes) => client({ routes })}
                label="Автоматические маршруты к TURN (-routes) — требует прав администратора"
              />
              <CheckBox
                checked={profile.client.manualCaptcha}
                onChange={(manualCaptcha) => client({ manualCaptcha })}
                label="Ручная captcha VK (-manual-captcha)"
              />
              <CheckBox
                checked={profile.client.debugMode}
                onChange={(debugMode) => client({ debugMode })}
                label="Подробный журнал (-debug)"
              />
              <CheckBox
                checked={profile.client.magicSwitch}
                onChange={(magicSwitch) => client({ magicSwitch })}
                label="Задать TURN-сервер вручную (-turn / -port)"
              />
            </div>
            {profile.client.magicSwitch && (
              <div className="grid gap-3 pt-3 md:grid-cols-2">
                <Field label="IP TURN-сервера">
                  <TextInput value={profile.client.magicTurn} onChange={(magicTurn) => client({ magicTurn })} />
                </Field>
                <Field label="Порт TURN-сервера">
                  <TextInput value={profile.client.magicPort} onChange={(magicPort) => client({ magicPort })} />
                </Field>
              </div>
            )}
          </Card>

          {tcp && (
            <Card title="Объединение сессий (-bond)">
              <div className="flex items-start justify-between gap-4">
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium">Одно TCP-соединение на все сессии</span>
                  <span className="text-xs text-zinc-500 dark:text-zinc-400">
                    Меньше соединений наружу — меньше поводов попасть под лимиты. Нужен сервер
                    версии 4.0 и новее: со старым подключение не поднимется.
                  </span>
                </div>
                <Toggle checked={profile.client.bond} onChange={(bond) => client({ bond })} />
              </div>
            </Card>
          )}

          {tcp && (
            <Card title="KCP (только режим tcp)">
              <div className="grid gap-3 md:grid-cols-4">
                {(
                  [
                    ["interval", "Интервал, мс"],
                    ["sndWnd", "Окно отправки"],
                    ["rcvWnd", "Окно приёма"],
                    ["mtu", "MTU (300…1350)"],
                    ["resend", "Быстрый ресенд"],
                    ["noDelay", "nodelay (0/1)"],
                    ["nc", "nc (0/1)"],
                  ] as const
                ).map(([key, label]) => (
                  <Field key={key} label={label}>
                    <NumberInput
                      value={profile.opts.kcp[key]}
                      onChange={(v) => opts({ kcp: { ...profile.opts.kcp, [key]: v } })}
                    />
                  </Field>
                ))}
              </div>
              <div className="pt-3">
                <CheckBox
                  checked={profile.opts.kcp.ackNoDelay}
                  onChange={(ackNoDelay) => opts({ kcp: { ...profile.opts.kcp, ackNoDelay } })}
                  label="Отправлять ACK сразу (-kcp-acknodelay)"
                />
              </div>
            </Card>
          )}

          <Card title="WireGuard">
            <div className="grid gap-3 md:grid-cols-2">
              <Field label="Режим работы">
                <Picker
                  value={profile.client.tunnelTransport}
                  onChange={(tunnelTransport) => client({ tunnelTransport })}
                  options={[
                    { value: "none", label: "Прокси (ядро слушает локальный порт)" },
                    { value: "wireguard", label: "VPN — встроенный туннель WireGuard" },
                  ]}
                />
              </Field>
              <Field label="Имя туннеля" hint="Под этим именем приложение создаст сетевой адаптер">
                <TextInput
                  value={profile.client.wireGuardTunnelName}
                  onChange={(wireGuardTunnelName) => client({ wireGuardTunnelName })}
                />
              </Field>
            </div>

            {profile.client.tunnelTransport === "wireguard" && !profile.client.wireGuardConfig.trim() && (
              <p className="pt-3 text-sm text-amber-600 dark:text-amber-400">
                Для VPN нужна конфигурация клиента. Обычно её выдаёт сервер: ключи, адрес и параметры
                обфускации он генерирует сам — нажмите «Запросить у сервера». Заготовка вручную нужна,
                только если сервер поднят не через это приложение.
              </p>
            )}

            <div className="pt-3">
              <Field
                label="Конфигурация от сервера"
                hint="Вставьте .conf как есть: Endpoint, MTU и AllowedIPs приложение подставит само при подъёме туннеля"
              >
                <TextArea
                  value={profile.client.wireGuardConfig}
                  onChange={(wireGuardConfig) => client({ wireGuardConfig })}
                  placeholder="[Interface]…"
                />
              </Field>
            </div>

            <div className="flex flex-col gap-1 pt-3">
              <CheckBox
                checked={useShared}
                onChange={toggleShared}
                label="Общие для всех профилей"
              />
              <p className="pl-[28px] text-xs text-zinc-500 dark:text-zinc-400">
                {useShared
                  ? `Используют ${sharedUsers} из ${sharedTotal} профилей. Правки затронут их все`
                  : "Один набор правил для всех профилей, где это включено"}
              </p>
            </div>

            <div className="grid gap-3 pt-3 md:grid-cols-2">
              <Field label="Раздельное туннелирование" hint="На Windows делится по подсетям, а не по приложениям">
                <Picker
                  value={split.mode}
                  onChange={(mode) => setSplit({ mode })}
                  options={[
                    { value: "all", label: "Весь трафик в туннель" },
                    { value: "exclude", label: "Всё, кроме указанных подсетей" },
                    { value: "include", label: "Только указанные подсети" },
                  ]}
                />
              </Field>
              {split.mode !== "all" && (
                <Field label="Подсети" hint="Через запятую: 10.0.0.0/8, 192.168.0.0/16">
                  <TextInput value={split.subnets} onChange={(subnets) => setSplit({ subnets })} />
                </Field>
              )}
            </div>
            {sharedError && <p className="pt-2 text-sm text-red-600 dark:text-red-400">{sharedError}</p>}

            <div className="flex flex-wrap items-center gap-2 pt-4">
              <Button
                variant={profile.client.wireGuardConfig.trim() ? "default" : "primary"}
                disabled={busy}
                busy={busy}
                onClick={async () => {
                  setWgError("");
                  setWgPreview("");
                  setWgPublicKey("");
                  setBusy(true);
                  try {
                    const r = await api?.VPSFetchWGConfig(profile.id);
                    if (!r) return;
                    if (r.fingerprint) {
                      setWgError(
                        "Сначала подтвердите ключ сервера на вкладке «Сервер»: " + r.fingerprint,
                      );
                      return;
                    }
                    if (!r.ok) {
                      setWgError(r.error);
                      return;
                    }
                    if (r.profile) onChange(r.profile);
                    setWgPreview(r.text ?? "");
                  } catch (e) {
                    setWgError(String(e));
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                Запросить у сервера
              </Button>
              <Button
                disabled={busy}
                onClick={async () => {
                  setWgError("");
                  setWgPreview("");
                  setWgPublicKey("");
                  try {
                    const t = await api?.GenerateWGConfig(profile.id);
                    if (!t) return;
                    client({ wireGuardConfig: t.config });
                    setWgPublicKey(t.publicKey);
                  } catch (e) {
                    setWgError(String(e));
                  }
                }}
              >
                Заготовка вручную
              </Button>
              <Button
                onClick={async () => {
                  setWgError("");
                  setWgPreview("");
                  try {
                    setWgPreview((await api?.PrepareWGConfig(profile.id)) ?? "");
                  } catch (e) {
                    setWgError(String(e));
                  }
                }}
              >
                Показать готовый конфиг
              </Button>
              <Button
                onClick={async () => {
                  setWgError("");
                  try {
                    const path = await api?.SaveWGConfig(profile.id);
                    if (path) setWgPreview(`Сохранено: ${path}`);
                  } catch (e) {
                    setWgError(String(e));
                  }
                }}
              >
                Сохранить .conf
              </Button>
              <span className="text-xs text-zinc-500">
                Запрос у сервера требует заполненного доступа по SSH на вкладке «Сервер»
              </span>
            </div>
            {wgPreview && (
              <pre className="selectable mt-3 max-h-64 overflow-auto rounded-lg bg-zinc-950 p-3 font-mono text-xs text-zinc-300">
                {wgPreview}
              </pre>
            )}
            {wgPublicKey && (
              <div className="mt-3 rounded-lg border border-emerald-400 bg-emerald-50 p-3 text-sm dark:border-emerald-800 dark:bg-emerald-950">
                <div className="font-medium text-emerald-900 dark:text-emerald-200">
                  Конфигурация создана — осталось два шага
                </div>
                <ol className="mt-2 list-decimal space-y-1 pl-5 text-xs text-emerald-900 dark:text-emerald-300">
                  <li>
                    Передайте владельцу сервера публичный ключ этого клиента:
                    <code className="selectable ml-1 break-all font-mono">{wgPublicKey}</code>
                  </li>
                  <li>Впишите публичный ключ сервера вместо заглушки в поле выше.</li>
                </ol>
                <div className="pt-2">
                  <Button onClick={() => navigator.clipboard.writeText(wgPublicKey)}>
                    Скопировать публичный ключ
                  </Button>
                </div>
              </div>
            )}
            {wgError && (
              <div className="flex flex-col gap-2 pt-2">
                <p className="text-sm text-red-600 dark:text-red-400">{wgError}</p>
                {needsSSH(wgError) && (
                  <div>
                    <Button variant="primary" onClick={commit(onOpenServer)}>
                      Заполнить доступ по SSH
                    </Button>
                  </div>
                )}
              </div>
            )}
          </Card>

          <Card title="Раздача в локальную сеть">
            <div className="flex items-start justify-between gap-4">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium">SOCKS5 для соседей по сети</span>
                <span className="text-xs text-zinc-500 dark:text-zinc-400">
                  Телефон или второй компьютер ходят через этот туннель, указав адрес и порт в
                  настройках прокси. Работает только в режиме VPN.
                </span>
              </div>
              <Toggle
                checked={profile.client.socksHotspot}
                onChange={(socksHotspot) => client({ socksHotspot })}
              />
            </div>

            {profile.client.socksHotspot && (
              <>
                <div className="grid gap-3 pt-3 md:grid-cols-2">
                  <Field label="Адрес раздачи" hint="0.0.0.0 — слушать на всех интерфейсах">
                    <TextInput
                      value={profile.client.socksListen}
                      placeholder="0.0.0.0:1080"
                      onChange={(socksListen) => client({ socksListen })}
                    />
                  </Field>
                  <Field label="Логин">
                    <TextInput
                      value={profile.client.socksUser}
                      onChange={(socksUser) => client({ socksUser })}
                    />
                  </Field>
                  <Field label="Пароль">
                    <TextInput
                      type="password"
                      value={profile.client.socksPassword}
                      onChange={(socksPassword) => client({ socksPassword })}
                    />
                  </Field>
                </div>

                {profile.client.tunnelTransport !== "wireguard" && (
                  <p className="mt-3 rounded-lg bg-amber-100 px-3 py-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
                    Сейчас профиль работает в режиме прокси — раздавать нечего. Включите режим VPN
                    в карточке «WireGuard» выше.
                  </p>
                )}

                <p className="mt-3 text-xs text-zinc-500 dark:text-zinc-400">
                  Логин и пароль обязательны: открытый SOCKS5 в общей сети найдут и начнут
                  использовать посторонние. Порт приложение открывает в брандмауэре само, но только
                  для частной и доменной сети. Если Windows считает вашу сеть общедоступной,
                  соединения не дойдут, пока в её свойствах не выбрать «Частная».
                </p>
              </>
            )}
          </Card>

          <Card title="Сервер">
            <p className="text-sm text-zinc-500 dark:text-zinc-400">
              Доступ по SSH, установка и запуск ядра на VPS — на вкладке «Сервер». Она работает с
              активным профилем.
            </p>
          </Card>
        </>
      )}

      </div>
    </div>
  );
}

/**
 * Ошибка про доступ по SSH — единственная, которую нельзя исправить в
 * самом профиле: поля живут на вкладке сервера. Показываем к ней кнопку,
 * иначе человек упирается в текст и не понимает, куда идти.
 */
function needsSSH(error: string): boolean {
  return error.includes("SSH") || error.includes("ssh");
}
