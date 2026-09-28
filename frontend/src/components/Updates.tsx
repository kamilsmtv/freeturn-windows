import { useEffect, useState } from "react";
import {
  backend,
  EVENTS,
  onEvent,
  type DownloadLink,
  type GUIStatus,
  type UpdateProgress,
  type UpdateStatus,
} from "../lib/api";
import { Button, Card, Row } from "./ui";
import { useBusyWhile } from "../lib/busy";
import { guard } from "../lib/effect";
import { QRCode } from "./QRCode";

const STAGE_LABEL: Record<UpdateProgress["stage"], string> = {
  download: "Скачивание",
  verify: "Проверка контрольной суммы",
  swap: "Замена бинаря",
  done: "Готово",
};

function mib(bytes: number) {
  return (bytes / 1024 / 1024).toFixed(1);
}

// Wintun - компонент сетевого адаптера. Он живёт отдельно от ядра и
// качается с чужого сайта, который у многих открывается только через VPN,
// так что без ручного пути тут замкнутый круг.
function Wintun() {
  const [installed, setInstalled] = useState<boolean | null>(null);
  const [source, setSource] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");

  const api = backend();
  const refresh = () => api?.WintunInstalled().then(setInstalled);

  useEffect(() => guard("Updates/wintun", () => {
    void refresh();
    api?.WintunSourceURL().then(setSource);
  }), []);

  const run = async (name: string, fn: () => Promise<void>) => {
    setBusy(name);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy("");
      void refresh();
    }
  };

  useBusyWhile("wintun", busy !== "", "Компонент сетевого адаптера");

  return (
    <Card title="Компонент сетевого адаптера (wintun)">
      <Row label="Состояние" hint="Нужен только в режиме VPN">
        <span className="text-sm">{installed === null ? "—" : installed ? "Установлен" : "Не установлен"}</span>
      </Row>
      <Row label="Действия">
        <div className="flex gap-2">
          <Button
            variant="primary"
            onClick={() => run("download", () => api!.EnsureWintun())}
            disabled={busy !== "" || !api || installed === true}
            busy={busy === "download"}
          >
            Скачать
          </Button>
          <Button
            onClick={() => run("file", () => api!.ImportWintun())}
            disabled={busy !== "" || !api}
            busy={busy === "file"}
          >
            Выбрать файл
          </Button>
        </div>
      </Row>

      {error && <p className="whitespace-pre-wrap pt-3 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {installed === false && source && (
        <p className="pt-3 text-xs text-zinc-500">
          Сайт компонента может быть недоступен без VPN. Скачайте архив на другой машине и укажите его
          кнопкой «Выбрать файл» - подойдёт и сам архив, и распакованная wintun.dll, контрольную сумму
          приложение проверит само.{" "}
          <button className="selectable underline" onClick={() => api?.OpenURL(source)}>
            {source}
          </button>
        </p>
      )}
    </Card>
  );
}

/**
 * Ссылки на всё, что приложение качает само.
 *
 * Карточка нужна там, где сети нет: открыть её из самого приложения не
 * выйдет, поэтому главное здесь - не кнопка «Открыть», а сам адрес,
 * который можно переписать, скопировать на флешку или снять QR-кодом
 * телефоном, у которого сеть есть.
 */
function Manual() {
  const [links, setLinks] = useState<DownloadLink[]>([]);
  const [copied, setCopied] = useState("");
  const [qr, setQr] = useState("");

  const api = backend();

  useEffect(() => guard("Updates/manual", () => {
    api?.Downloads().then(setLinks);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), []);

  if (links.length === 0) return null;

  const copy = (url: string) => {
    navigator.clipboard.writeText(url).then(
      () => {
        setCopied(url);
        setTimeout(() => setCopied(""), 1500);
      },
      () => {},
    );
  };

  return (
    <Card title="Если скачать не удаётся">
      <p className="text-sm text-zinc-500 dark:text-zinc-400">
        Сайты бывают недоступны без VPN, а поднять VPN без этих же файлов нельзя. Откройте ссылку
        там, где сеть есть — на телефоне или другой машине, — принесите файл и укажите его кнопкой
        «Выбрать файл» выше. Раскладывать файлы по папкам руками не нужно, да и не выйдет: рядом с
        FreeTurn.exe приложение ничего не ищет, а по своим каталогам разложит само — проверив
        контрольную сумму.
      </p>

      <div className="mt-3 flex flex-col gap-3">
        {links.map((l) => (
          <div
            key={l.url}
            className="rounded-xl border border-zinc-200 p-3 dark:border-zinc-800"
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="text-sm font-medium">
                {l.title}
                {l.optional && (
                  <span className="ml-2 text-xs font-normal text-zinc-400">не обязателен</span>
                )}
              </span>
              <div className="flex gap-2">
                <Button onClick={() => copy(l.url)}>
                  {copied === l.url ? "Скопировано" : "Копировать"}
                </Button>
                <Button onClick={() => setQr(qr === l.url ? "" : l.url)}>
                  {qr === l.url ? "Скрыть QR" : "QR"}
                </Button>
                <Button variant="ghost" onClick={() => api?.OpenURL(l.url)}>
                  Открыть
                </Button>
              </div>
            </div>

            <p className="pt-1 text-xs text-zinc-500 dark:text-zinc-400">{l.hint}</p>
            <p className="selectable break-all pt-1 font-mono text-xs text-zinc-500 dark:text-zinc-400">
              {l.url}
            </p>
            {l.dir && (
              <p className="pt-1 text-xs text-zinc-400 dark:text-zinc-500">
                Попадёт в <span className="selectable break-all font-mono">{l.dir}</span>
              </p>
            )}

            {qr === l.url && (
              <div className="pt-3">
                <QRCode text={l.url} name={l.title} />
              </div>
            )}
          </div>
        ))}
      </div>
    </Card>
  );
}

export function Updates() {
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  const [progress, setProgress] = useState<UpdateProgress | null>(null);
  const [gui, setGui] = useState<GUIStatus | null>(null);
  // Занятость помечаем именем действия: колечко должно крутиться ровно
  // в той кнопке, которую нажали.
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [asset, setAsset] = useState("");

  useEffect(() => guard("Updates/subscribe", () => {
    backend()?.UpdateStatus().then(setStatus);
    backend()?.CoreAssetName().then(setAsset);
    const off = [
      onEvent<UpdateStatus>(EVENTS.updateStatus, setStatus),
      onEvent<UpdateProgress>(EVENTS.updateProgress, setProgress),
      onEvent<GUIStatus>(EVENTS.guiUpdate, setGui),
    ];
    return () => off.forEach((f) => f());
  }), []);

  const run = async (name: string, fn: () => Promise<UpdateStatus>) => {
    setBusy(name);
    setError("");
    try {
      setStatus(await fn());
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy("");
      setProgress(null);
    }
  };

  const api = backend();

  // Скачивание и проверка версий идут заметное время - показываем полоской.
  useBusyWhile("update", busy !== "", "Обновление ядра");

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
            <Button
              onClick={() => run("check", () => api!.CheckCoreUpdate())}
              disabled={busy !== "" || !api}
              busy={busy === "check"}
            >
              Проверить
            </Button>
            <Button
              variant="primary"
              onClick={() => run("install", () => api!.InstallCore())}
              disabled={busy !== "" || !api || (status?.installed === true && !status?.updateReady)}
              busy={busy === "install"}
            >
              {status?.installed ? "Обновить" : "Скачать ядро"}
            </Button>
            <Button
              onClick={() => run("file", () => api!.ImportCore())}
              disabled={busy !== "" || !api}
              busy={busy === "file"}
            >
              Выбрать файл
            </Button>
            <Button
              onClick={() => run("rollback", () => api!.RollbackCore())}
              disabled={busy !== "" || !api || !status?.canRollback}
              busy={busy === "rollback"}
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
          <p className="whitespace-pre-wrap pt-3 text-sm text-red-600 dark:text-red-400">{error || status?.error}</p>
        )}

        {(!status?.installed || error !== "" || (status?.error ?? "") !== "") && (
          <p className="pt-3 text-xs text-zinc-500">
            Если GitHub недоступен, скачайте {asset || "бинарь ядра"} на другой машине и укажите кнопкой
            «Выбрать файл». Положите рядом checksums.txt из того же релиза - файл будет сверен по
            контрольной сумме. Адреса - в карточке «Если скачать не удаётся» ниже.
          </p>
        )}
      </Card>

      <Wintun />

      <Manual />

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
                setBusy("gui");
                try {
                  setGui((await api?.CheckGUIUpdate()) ?? null);
                } finally {
                  setBusy("");
                }
              }}
              disabled={busy !== "" || !api}
              busy={busy === "gui"}
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
