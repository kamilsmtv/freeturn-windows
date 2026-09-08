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
} from "./lib/api";
import { guard } from "./lib/effect";
import { applyTheme } from "./lib/theme";
import { Button, Card, EmptyState, Row } from "./components/ui";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { Home } from "./components/Home";
import { LogView } from "./components/LogView";
import { ProfileEditor } from "./components/ProfileEditor";
import { Rail, type Screen } from "./components/Rail";
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
      home: <Home core={core} onEditProfile={setDraft} />,
      server: <ServerTab />,
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
      about: <About info={info} core={core} />,
    }[screen]
  );

  return (
    <div className="flex h-screen bg-zinc-100 text-zinc-900 dark:bg-zinc-950 dark:text-zinc-100">
      <Rail screen={screen} onScreen={(s) => { setDraft(null); setScreen(s); }} core={core} />

      <main className="flex min-w-0 flex-1 flex-col overflow-hidden">
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
function ServerTab() {
  const [snap, setSnap] = useState<Snapshot>({ list: [], activeId: "" });

  useEffect(() => guard("ServerTab/profiles", () => {
    const put = (s: Snapshot) => setSnap({ list: s?.list ?? [], activeId: s?.activeId ?? "" });
    backend()?.Profiles().then(put);
    return onEvent<Snapshot>(EVENTS.profiles, put);
  }), []);

  const active = snap.list.find((p) => p.id === snap.activeId) ?? null;
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

function About({ info, core }: { info: AppInfo | null; core: CoreStatus | null }) {
  if (!info) return null;
  const api = backend();

  return (
    <div className="scroll h-full overflow-y-auto px-8 py-6">
      <Card title="О программе">
        <Row label="Версия приложения">{info.version}</Row>
        <Row label="Версия ядра" hint={info.coreDir}>
          <span className="font-mono text-xs text-zinc-500">{core?.version || "неизвестна"}</span>
        </Row>
        <Row label="Конфиги и логи" hint={info.dataDir}>
          <Button onClick={() => api?.OpenDataDir()}>Открыть</Button>
        </Row>
        <Row label="Ядро" hint="free-turn-proxy, автор samosvalishe">
          <Button onClick={() => api?.OpenURL(`https://github.com/${info.coreRepo}`)}>GitHub</Button>
        </Row>
        <Row label="Android-клиент" hint="turn-proxy-android — источник модели данных и формата бэкапа">
          <Button onClick={() => api?.OpenURL("https://github.com/samosvalishe/turn-proxy-android")}>
            GitHub
          </Button>
        </Row>
        <p className="pt-3 text-xs text-zinc-500 dark:text-zinc-400">
          Проект создан в образовательных целях. Лицензия GPL-3.0.
        </p>
      </Card>
    </div>
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
