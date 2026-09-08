import { useState } from "react";
import { backend, type Profile } from "../lib/api";
import { Button, Card } from "./ui";
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
}: {
  profile: Profile;
  onChange: (p: Profile) => void;
  onSave: () => void;
  onCancel: () => void;
}) {
  const [advanced, setAdvanced] = useState(false);
  const [wgPreview, setWgPreview] = useState("");
  const [wgPublicKey, setWgPublicKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [wgError, setWgError] = useState("");
  const api = backend();

  const client = (patch: Partial<Profile["client"]>) =>
    onChange({ ...profile, client: { ...profile.client, ...patch } });
  const opts = (patch: Partial<Profile["opts"]>) =>
    onChange({ ...profile, opts: { ...profile.opts, ...patch } });

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
        <Button variant="primary" onClick={onSave}>
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
            label="Ссылка на звонок VK Calls"
            hint="Создайте звонок и не завершайте его — по нему клиент получает TURN-данные"
          >
            <TextInput
              value={profile.client.vkLink}
              onChange={(vkLink) => client({ vkLink })}
              placeholder="https://vk.ru/call/join/…"
            />
          </Field>
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
                  {busy ? "Запрашиваю…" : "Запросить у сервера"}
                </Button>
              </div>
            )}
            {wgError && <p className="pt-2 text-sm text-red-600 dark:text-red-400">{wgError}</p>}
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
              <Field label="Пейсинг мимикрии, мс (-obf-timing)" hint="0 — выключен, максимум 60">
                <NumberInput
                  value={profile.opts.obfTimingMs}
                  min={0}
                  max={60}
                  onChange={(obfTimingMs) => opts({ obfTimingMs })}
                />
              </Field>
              <Field label="Потоков TURN (-n)">
                <NumberInput value={profile.client.threads} min={1} onChange={(threads) => client({ threads })} />
              </Field>
              <Field label="Потоков на учётку (-streams-per-cred)">
                <NumberInput
                  value={profile.client.streamsPerCred}
                  min={1}
                  onChange={(streamsPerCred) => client({ streamsPerCred })}
                />
              </Field>
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

          {profile.opts.proxyMode === "tcp" && (
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

            <div className="grid gap-3 pt-3 md:grid-cols-2">
              <Field label="Раздельное туннелирование" hint="На Windows делится по подсетям, а не по приложениям">
                <Picker
                  value={profile.client.splitTunnelMode}
                  onChange={(splitTunnelMode) => client({ splitTunnelMode })}
                  options={[
                    { value: "all", label: "Весь трафик в туннель" },
                    { value: "exclude", label: "Всё, кроме указанных подсетей" },
                    { value: "include", label: "Только указанные подсети" },
                  ]}
                />
              </Field>
              {profile.client.splitTunnelMode !== "all" && (
                <Field label="Подсети" hint="Через запятую: 10.0.0.0/8, 192.168.0.0/16">
                  <TextInput
                    value={profile.client.splitTunnelSubnets}
                    onChange={(splitTunnelSubnets) => client({ splitTunnelSubnets })}
                  />
                </Field>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-2 pt-4">
              <Button
                variant={profile.client.wireGuardConfig.trim() ? "default" : "primary"}
                disabled={busy}
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
                {busy ? "Запрашиваю…" : "Запросить у сервера"}
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
            {wgError && <p className="pt-2 text-sm text-red-600 dark:text-red-400">{wgError}</p>}
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
