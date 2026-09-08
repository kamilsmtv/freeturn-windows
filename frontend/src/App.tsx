import { useEffect, useState } from "react";
import {
  backend,
  EVENTS,
  onEvent,
  WEBVIEW2_URL,
  type AppInfo,
  type CoreStatus,
  type Environment,
  type Profile,
  type Settings,
  type Snapshot,
  type TunnelStatus,
} from "./lib/api";
import { useBusyLabel, useBusyWhile } from "./lib/busy";
import { connectionPhase, runningProfile } from "./lib/phase";
import { guard } from "./lib/effect";
import { applyTheme } from "./lib/theme";
import { Card, EmptyState } from "./components/ui";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { About } from "./components/About";
import { Home } from "./components/Home";
import { LogView } from "./components/LogView";
import { ProfileEditor } from "./components/ProfileEditor";
import { Rail, type Screen } from "./components/Rail";
import { TopProgress } from "./components/TopProgress";
import { ServerPanel } from "./components/ServerPanel";
import { SettingsScreen } from "./components/SettingsScreen";
import { Updates } from "./components/Updates";

export default function App() {
  const [screen, setScreen] = useState<Screen>("home");
  const [env, setEnv] = useState<Environment | null>(null);
  const [info, setInfo] = useState<AppInfo | null>(null);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [core, setCore] = useState<CoreStatus | null>(null);
  const [draft, setDraft] = useState<Profile | null>(null);
  const [snap, setSnap] = useState<Snapshot>({ list: [], activeId: "" });
  const [tunnel, setTunnel] = useState<TunnelStatus | null>(null);

  useEffect(() => guard("App/init", () => {
    const api = backend();
    if (!api) return;
    api.Environment().then(setEnv);
    api.Info().then(setInfo);
    api.GetSettings().then(setSettings);
    api.CoreStatus().then(setCore);
    // Задачу планировщика мог удалить кто-то извне - спрашиваем систему.
    api.AutostartEnabled().then((on) =>
      setSettings((s) => (s && s.autostart !== on ? { ...s, autostart: on } : s)),
    );
    return onEvent<CoreStatus>(EVENTS.coreState, setCore);
  }), []);

  useEffect(() => applyTheme(settings?.theme ?? "system"), [settings?.theme]);

  // Профили и туннель нужны не только главному экрану: по ним считается
  // фаза подключения, а её показывает полоска в любом разделе.
  useEffect(() => guard("App/profiles", () => {
    const put = (s: Snapshot) => setSnap({ list: s?.list ?? [], activeId: s?.activeId ?? "" });
    backend()?.Profiles().then(put);
    return onEvent<Snapshot>(EVENTS.profiles, put);
  }), []);

  useEffect(() => guard("App/tunnel", () => {
    backend()?.TunnelStatus().then(setTunnel);
    return onEvent<TunnelStatus>(EVENTS.tunnel, setTunnel);
  }), []);

  const active = snap.list.find((p) => p.id === snap.activeId) ?? null;
  const phase = connectionPhase(core, tunnel, runningProfile(core, snap.list, active));

  // Пока туннель поднимается, состояние меняется без события: спрашиваем сами.
  useEffect(() => guard("App/tunnelPoll", () => {
    if (!phase.busy) return;
    const id = setInterval(() => backend()?.TunnelStatus().then(setTunnel), 1500);
    return () => clearInterval(id);
  }), [phase.busy]);

  // Переход виден из любого раздела: полоска показывает его и на экране
  // сервера, и в настройках.
  useBusyWhile("connection", phase.busy, phase.text);
  const busyLabel = useBusyLabel();

  const patch = (p: Partial<Settings>) => {
    if (!settings) return;
    const next = { ...settings, ...p };
    setSettings(next);
    backend()?.SaveSettings(next);
  };

  // Редактор профиля перекрывает раздел целиком: так же, как отдельный
  // экран профиля в Android-клиенте.
  const content = draft ? (
    <ProfileEditor
      profile={draft}
      onChange={setDraft}
      onCancel={() => setDraft(null)}
      onSave={async () => {
        await backend()?.SaveProfile(draft);
        setDraft(null);
      }}
    />
  ) : (
    {
      home: <Home core={core} snap={snap} phase={phase} onEditProfile={setDraft} />,
      server: <ServerTab active={active} />,
      logs: (
        <div className="h-full px-8 py-6">
          <LogView />
        </div>
      ),
      updates: (
        <div className="scroll h-full overflow-y-auto px-8 py-6">
          <Updates />
        </div>
      ),
      settings: settings ? (
        <SettingsScreen settings={settings} patch={patch} onSettings={setSettings} />
      ) : null,
      about: <About info={info} phase={phase} coreVersion={core?.version ?? ""} />,
    }[screen]
  );

  return (
    <div className="flex h-screen bg-zinc-100 text-zinc-900 dark:bg-zinc-950 dark:text-zinc-100">
      <Rail screen={screen} onScreen={(s) => { setDraft(null); setScreen(s); }} phase={phase} />

      <main className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <TopProgress active={busyLabel !== ""} label={busyLabel} />
        <EnvBanner env={env} />
        {/* Отдельный контейнер: без него баннер съедает высоту экранов,
            которые тянутся на 100% и считают её от всего main. */}
        <div className="flex min-h-0 flex-1 flex-col">
          <ErrorBoundary key={draft ? "editor" : screen}>{content}</ErrorBoundary>
        </div>
      </main>
    </div>
  );
}

/** Вкладка сервера работает с активным профилем. */
function ServerTab({ active }: { active: Profile | null }) {
  if (!active) {
    return (
      <div className="px-8 py-6">
        <Card>
          <div className="h-48">
            <EmptyState
              title="Профиль не выбран"
              hint="Управление сервером работает с активным профилем: выберите его на главном экране."
            />
          </div>
        </Card>
      </div>
    );
  }
  return (
    <ServerPanel profile={active} onProfile={() => {}} />
  );
}

function EnvBanner({ env }: { env: Environment | null }) {
  if (!env) return null;

  if (!env.webView2) {
    return (
      <Banner tone="red">
        Не найден компонент WebView2 — без него окно не отрисуется.{" "}
        <button className="underline" onClick={() => backend()?.OpenURL(WEBVIEW2_URL)}>
          Скачать WebView2
        </button>
      </Banner>
    );
  }
  if (!env.admin) {
    return (
      <Banner tone="amber">
        Приложение запущено без прав администратора: ядро не сможет добавить маршруты к TURN-серверам
        (флаг <code>-routes</code>). Перезапустите от имени администратора.
      </Banner>
    );
  }
  return null;
}

function Banner({ tone, children }: { tone: "red" | "amber"; children: React.ReactNode }) {
  const styles =
    tone === "red"
      ? "bg-red-100 text-red-900 dark:bg-red-950 dark:text-red-200"
      : "bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-200";
  return <div className={`px-6 py-2.5 text-sm ${styles}`}>{children}</div>;
}
