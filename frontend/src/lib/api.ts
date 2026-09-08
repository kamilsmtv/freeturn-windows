/**
 * Тонкая обёртка над биндингами Wails. Обращаемся к window.go напрямую,
 * а не к сгенерированному wailsjs: так фронтенд собирается и без запуска
 * `wails generate`, а типы держим здесь, рядом с использованием.
 */

export type Environment = {
  admin: boolean;
  webView2: boolean;
  webView2Version: string;
  windows: boolean;
};

export type AppInfo = {
  version: string;
  dataDir: string;
  coreDir: string;
  logDir: string;
  coreRepo: string;
  guiRepo: string;
};

export type Settings = {
  theme: "system" | "light" | "dark";
  minimizeToTray: boolean;
  startMinimized: boolean;
  autostart: boolean;
  autoConnectLast: boolean;
  coreUpdateMode: "ask" | "auto" | "never";
  coreUpdateHours: number;
  checkGuiUpdates: boolean;
  githubToken: string;
  subRefreshHours: number;
  subscriptions: string[] | null;
  subLastRefresh: string;
  ownClientId: string;
  lastProfileId: string;
  keepLogLines: number;
  suppressAdminWarn: boolean;
  coreVersionChecked: string;
};

export type CoreState = "stopped" | "starting" | "running" | "stopping" | "failed";

export type CoreStatus = {
  state: CoreState;
  profileId: string;
  pid: number;
  version: string;
  startedAt: string;
  error: string;
};

export type LogLine = {
  seq: number;
  time: string;
  level: "info" | "warn" | "error" | "debug";
  text: string;
};

export type UpdateStatus = {
  installed: boolean;
  version: string;
  latestVersion: string;
  updateReady: boolean;
  changelog: string;
  releaseUrl: string;
  canRollback: boolean;
  rollbackTarget: string;
  checkedAt: string;
  error: string;
};

export type UpdateProgress = {
  stage: "download" | "verify" | "swap" | "done";
  downloaded: number;
  total: number;
};

export type GUIStatus = {
  current: string;
  latest: string;
  updateReady: boolean;
  changelog: string;
  releaseUrl: string;
  checkedAt: string;
  error: string;
};

export type KCP = {
  noDelay: number;
  interval: number;
  resend: number;
  nc: number;
  sndWnd: number;
  rcvWnd: number;
  mtu: number;
  ackNoDelay: boolean;
};

export type Profile = {
  id: string;
  name: string;
  ssh: {
    ip: string;
    port: number;
    username: string;
    password: string;
    authType: string;
    sshKey: string;
    hostFingerprint: string;
    rootMode: string;
    sudoPassword: string;
  };
  client: {
    serverAddress: string;
    vkLink: string;
    provider: string;
    threads: number;
    streamsPerCred: number;
    useUdp: boolean;
    manualCaptcha: boolean;
    localPort: string;
    debugMode: boolean;
    dnsMode: string;
    customDns: string;
    platform: string;
    magicSwitch: boolean;
    magicTurn: string;
    magicPort: string;
    routes: boolean;
    tunnelTransport: string;
    wireGuardConfig: string;
    wireGuardTunnelName: string;
    splitTunnelMode: string;
    splitTunnelSubnets: string;
    logsEnabled: boolean;
    clientId: string;
  };
  proxyListen: string;
  proxyConnect: string;
  opts: {
    obfProfile: string;
    obfKey: string;
    obfTimingMs: number;
    proxyMode: string;
    kcp: KCP;
  };
  subUrl?: string;
};

export type Snapshot = { list: Profile[]; activeId: string };

export type SubPreview = {
  name: string;
  refresh: string;
  skipped: number;
  profiles: Profile[];
};

type Backend = {
  Environment(): Promise<Environment>;
  Info(): Promise<AppInfo>;
  GetSettings(): Promise<Settings>;
  SaveSettings(s: Settings): Promise<void>;
  OpenURL(url: string): Promise<void>;
  OpenDataDir(): Promise<void>;
  CoreStatus(): Promise<CoreStatus>;
  CoreLog(): Promise<LogLine[]>;
  CoreClearLog(): Promise<void>;
  CoreStop(): Promise<void>;
  UpdateStatus(): Promise<UpdateStatus>;
  CheckCoreUpdate(): Promise<UpdateStatus>;
  InstallCore(): Promise<UpdateStatus>;
  RollbackCore(): Promise<UpdateStatus>;
  CheckGUIUpdate(): Promise<GUIStatus>;
  Profiles(): Promise<Snapshot>;
  NewProfile(name: string): Promise<Profile>;
  SaveProfile(p: Profile): Promise<Profile>;
  DeleteProfile(id: string): Promise<void>;
  DuplicateProfile(id: string): Promise<Profile>;
  SetActiveProfile(id: string): Promise<void>;
  GenerateObfKey(): Promise<string>;
  GenerateClientID(): Promise<string>;
  ImportLink(raw: string): Promise<Profile>;
  ExportLink(id: string, includeVKLink: boolean, clientId: string): Promise<string>;
  ExportLinkToFile(id: string, includeVKLink: boolean, clientId: string): Promise<string>;
  FetchSubscription(url: string): Promise<SubPreview>;
  ApplySubscription(url: string): Promise<SubPreview>;
  RemoveSubscription(url: string): Promise<void>;
  RefreshSubscriptions(): Promise<string[]>;
  ExportBackup(password: string): Promise<string>;
  ImportBackup(password: string): Promise<number>;
  CoreStart(p: Profile): Promise<void>;
  ExportLog(): Promise<string>;
  QRCode(text: string): Promise<string>;
  SaveQRCode(text: string, name: string): Promise<string>;
  PrepareWGConfig(profileId: string): Promise<string>;
  SaveWGConfig(profileId: string): Promise<string>;
  TrafficStats(): Promise<Traffic>;
  SetAutostart(enable: boolean): Promise<void>;
  AutostartEnabled(): Promise<boolean>;
  HideWindow(): Promise<void>;
  QuitApp(): Promise<void>;
  ConnectActive(): Promise<void>;
  VPSProbe(id: string): Promise<VPSResult>;
  VPSInstall(id: string, withWgPkg: boolean): Promise<VPSResult>;
  VPSStart(id: string): Promise<VPSResult>;
  VPSStop(id: string): Promise<VPSResult>;
  VPSLogs(id: string, lines: number): Promise<VPSResult>;
  VPSSetupWireGuard(id: string): Promise<VPSResult>;
  VPSFetchWGConfig(id: string): Promise<VPSResult>;
  VPSImportShareInfo(id: string): Promise<VPSResult>;
  VPSAddPeer(id: string, name: string): Promise<string>;
  TrustHostKey(id: string, fingerprint: string): Promise<void>;
  TunnelStatus(): Promise<TunnelStatus>;
  WintunInstalled(): Promise<boolean>;
  EnsureWintun(): Promise<void>;
  GenerateWGConfig(profileId: string): Promise<WGTemplate>;
};

export type WGTemplate = {
  config: string;
  publicKey: string;
  needsServerKey: boolean;
};

export type TunnelStatus = {
  enabled: boolean;
  up: boolean;
  adapter: string;
  error: string;
};

export type ProbeData = {
  installed: boolean;
  version: string;
  running: boolean;
  mode: string;
  obf: string;
  runtime: string;
  euid: number;
  wg: { present: boolean; port: number };
  virt: string;
  wg_kernel: boolean;
  conflicts: {
    warp: boolean;
    x3ui: boolean;
    wgeasy: boolean;
    tailscale: boolean;
    other_ifaces: string[] | null;
  };
};

export type VPSResult = {
  ok: boolean;
  fingerprint: string;
  error: string;
  probe?: ProbeData;
  text?: string;
  profile?: Profile;
};

export type Traffic = {
  available: boolean;
  adapter: string;
  rxBytes: number;
  txBytes: number;
  reason: string;
  rxRate: number;
  txRate: number;
};

type WailsRuntime = {
  EventsOn(event: string, cb: (...data: any[]) => void): () => void;
  LogError?(message: string): void;
};

declare global {
  interface Window {
    go?: { main?: { App?: Backend } };
    runtime?: WailsRuntime;
  }
}

/** Подписка на событие бэкенда; вне окна Wails - пустая отписка. */
export function onEvent<T>(event: string, cb: (data: T) => void): () => void {
  const rt = window.runtime;
  if (typeof rt?.EventsOn !== "function") return () => {};

  const off = rt.EventsOn(event, (data: T) => cb(data));
  // React вызывает возвращённое значение при размонтировании: если это не
  // функция, весь экран падает с "is not a function".
  return typeof off === "function" ? off : () => {};
}

export const EVENTS = {
  coreState: "core:state",
  coreLog: "core:log",
  updateStatus: "update:status",
  updateProgress: "update:progress",
  guiUpdate: "update:gui",
  profiles: "profiles:changed",
  vpsLog: "vps:log",
  tunnel: "tunnel:state",
} as const;

/** Бэкенд доступен только внутри окна Wails; в браузере (vite dev) его нет. */
export function backend(): Backend | null {
  return window.go?.main?.App ?? null;
}

export const WEBVIEW2_URL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703";
