import { useEffect, useState } from "react";
import {
  backend,
  EVENTS,
  onEvent,
  type GUIStatus,
  type UpdateProgress,
  type UpdateStatus,
} from "../lib/api";
import { Button, Card, Row } from "./ui";
import { guard } from "../lib/effect";

const STAGE_LABEL: Record<UpdateProgress["stage"], string> = {
  download: "Скачивание",
  verify: "Проверка контрольной суммы",
  swap: "Замена бинаря",
  done: "Готово",
};

function mib(bytes: number) {
  return (bytes / 1024 / 1024).toFixed(1);
}

export function Updates() {
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  const [progress, setProgress] = useState<UpdateProgress | null>(null);
  const [gui, setGui] = useState<GUIStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => guard("Updates/subscribe", () => {
    backend()?.UpdateStatus().then(setStatus);
    const off = [
      onEvent<UpdateStatus>(EVENTS.updateStatus, setStatus),
      onEvent<UpdateProgress>(EVENTS.updateProgress, setProgress),
      onEvent<GUIStatus>(EVENTS.guiUpdate, setGui),
    ];
    return () => off.forEach((f) => f());
  }), []);

  const run = async (fn: () => Promise<UpdateStatus>) => {
    setBusy(true);
    setError("");
    try {
      setStatus(await fn());
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
      setProgress(null);
    }
  };

  const api = backend();

  return (
    <div className="flex flex-col gap-4">
      <Card title="Ядро free-turn-proxy">
        <Row label="Установленная версия" hint={status?.installed ? "" : "Ядро ещё не скачано"}>
          <span className="font-mono text-sm">{status?.version || "—"}</span>
        </Row>
        <Row label="Последний релиз" hint={status?.checkedAt ? `Проверено: ${new Date(status.checkedAt).toLocaleString("ru")}` : ""}>
          <span className="font-mono text-sm">{status?.latestVersion || "—"}</span>
        </Row>
        <Row label="Действия">
          <div className="flex gap-2">
            <Button onClick={() => run(() => api!.CheckCoreUpdate())} disabled={busy || !api}>
              Проверить
            </Button>
            <Button
              variant="primary"
              onClick={() => run(() => api!.InstallCore())}
              disabled={busy || !api || (status?.installed === true && !status?.updateReady)}
            >
              {status?.installed ? "Обновить" : "Скачать ядро"}
            </Button>
            <Button
              onClick={() => run(() => api!.RollbackCore())}
              disabled={busy || !api || !status?.canRollback}
            >
              Откатить{status?.rollbackTarget ? ` на ${status.rollbackTarget}` : ""}
            </Button>
          </div>
        </Row>

        {progress && (
          <div className="pt-3">
            <div className="mb-1 flex justify-between text-xs text-zinc-500">
              <span>{STAGE_LABEL[progress.stage]}</span>
              {progress.total > 0 && (
                <span>
                  {mib(progress.downloaded)} / {mib(progress.total)} МиБ
                </span>
              )}
            </div>
            <div className="h-1.5 overflow-hidden rounded-full bg-zinc-200 dark:bg-zinc-800">
              <div
                className="h-full bg-emerald-600 transition-all"
                style={{
                  width: progress.total > 0 ? `${(progress.downloaded / progress.total) * 100}%` : "100%",
                }}
              />
            </div>
          </div>
        )}

        {(error || status?.error) && (
          <p className="pt-3 text-sm text-red-600 dark:text-red-400">{error || status?.error}</p>
        )}
      </Card>

      {status?.updateReady && status.changelog && (
        <Card title={`Что нового в ${status.latestVersion}`}>
          <pre className="selectable max-h-64 overflow-auto whitespace-pre-wrap text-sm text-zinc-700 dark:text-zinc-300">
            {status.changelog}
          </pre>
          {status.releaseUrl && (
            <div className="pt-3">
              <Button variant="ghost" onClick={() => api?.OpenURL(status.releaseUrl)}>
                Открыть страницу релиза
              </Button>
            </div>
          )}
        </Card>
      )}

      <Card title="Обновление приложения">
        <Row label="Версия FreeTurn" hint="Приложение обновляется вручную: самозамены .exe нет">
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm">{gui?.current || "—"}</span>
            <Button
              onClick={async () => {
                setBusy(true);
                try {
                  setGui((await api?.CheckGUIUpdate()) ?? null);
                } finally {
                  setBusy(false);
                }
              }}
              disabled={busy || !api}
            >
              Проверить
            </Button>
          </div>
        </Row>
        {gui?.updateReady && (
          <p className="pt-3 text-sm">
            Доступна версия {gui.latest}.{" "}
            <button className="underline" onClick={() => api?.OpenURL(gui.releaseUrl)}>
              Открыть страницу релиза
            </button>
          </p>
        )}
        {gui?.error && <p className="pt-3 text-sm text-red-600 dark:text-red-400">{gui.error}</p>}
      </Card>
    </div>
  );
}
