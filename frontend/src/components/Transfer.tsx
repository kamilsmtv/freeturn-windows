import { useState } from "react";
import { backend, type Profile } from "../lib/api";
import { Button } from "./ui";
import { CheckBox, Field, TextArea, TextInput } from "./Field";
import { QRCode } from "./QRCode";

function Modal({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-10 flex items-center justify-center bg-black/40 p-6" onClick={onClose}>
      {/* Диалог должен помещаться в минимальное окно (900x600): с QR-кодом
          содержимое выше экрана, поэтому прокручиваем сам диалог. */}
      <div
        className="scroll max-h-full w-full max-w-lg overflow-y-auto rounded-xl bg-white p-5 shadow-xl dark:bg-zinc-900"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 text-sm font-semibold">{title}</h2>
        {children}
      </div>
    </div>
  );
}

export function ImportDialog({ onClose }: { onClose: () => void }) {
  const [raw, setRaw] = useState("");
  const [error, setError] = useState("");
  const api = backend();

  return (
    <Modal title="Импорт профиля из ссылки" onClose={onClose}>
      <Field label="Ссылка freeturn://" hint="Вставьте строку целиком">
        <TextArea value={raw} onChange={setRaw} rows={4} placeholder="freeturn://…" />
      </Field>
      {error && <p className="pt-2 text-sm text-red-600 dark:text-red-400">{error}</p>}
      <div className="flex justify-end gap-2 pt-4">
        <Button variant="ghost" onClick={onClose}>
          Отмена
        </Button>
        <Button
          variant="primary"
          onClick={async () => {
            setError("");
            try {
              await api?.ImportLink(raw);
              onClose();
            } catch (e) {
              setError(String(e));
            }
          }}
        >
          Импортировать
        </Button>
      </div>
    </Modal>
  );
}

export function ShareDialog({ profile, onClose }: { profile: Profile; onClose: () => void }) {
  const [includeVK, setIncludeVK] = useState(false);
  const [clientId, setClientId] = useState("");
  const [url, setUrl] = useState("");
  const [saved, setSaved] = useState("");
  const [error, setError] = useState("");
  const api = backend();

  const build = async () => {
    setError("");
    try {
      setUrl((await api?.ExportLink(profile.id, includeVK, clientId)) ?? "");
    } catch (e) {
      setError(String(e));
    }
  };

  return (
    <Modal title={`Поделиться профилем «${profile.name}»`} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <CheckBox
          checked={includeVK}
          onChange={setIncludeVK}
          label="Вложить свою ссылку на звонок VK"
        />
        <p className="-mt-2 text-xs text-zinc-500">
          Обычно получатель создаёт свой звонок: ссылка уникальна для каждого клиента.
        </p>
        <Field label="Client ID для получателя" hint="Владелец сервера добавляет его в allowlist">
          <div className="flex items-center gap-2">
            <TextInput value={clientId} onChange={setClientId} mono placeholder="необязательно" />
            <Button onClick={async () => setClientId((await api?.GenerateClientID()) ?? "")}>Создать</Button>
          </div>
        </Field>

        <div className="flex items-center gap-2">
          <Button onClick={build}>Собрать ссылку</Button>
          <Button
            onClick={async () => {
              setError("");
              try {
                const path = await api?.ExportLinkToFile(profile.id, includeVK, clientId);
                if (path) setSaved(path);
              } catch (e) {
                setError(String(e));
              }
            }}
          >
            Сохранить в файл
          </Button>
        </div>

        {url && (
          <>
            <QRCode text={url} name={profile.name} />
            <Field label="Ссылка">
              <TextArea value={url} onChange={() => {}} rows={4} />
            </Field>
            <Button onClick={() => navigator.clipboard.writeText(url)}>Скопировать</Button>
          </>
        )}
        {saved && <p className="text-sm text-emerald-600">Сохранено: {saved}</p>}
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
      </div>
    </Modal>
  );
}
