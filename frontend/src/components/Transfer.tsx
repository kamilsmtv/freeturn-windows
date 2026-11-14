import { useEffect, useState } from "react";
import { backend, type Profile } from "../lib/api";
import { guard } from "../lib/effect";
import { validClientName } from "../lib/format";
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
  const [saving, setSaving] = useState(false);
  // Доступ заводит сервер: он выдаёт Client ID, кладёт его в allowlist и
  // отдаёт готовую ссылку с ключами. Ссылка, собранная из одного профиля,
  // гостя не авторизует - сервер его просто не знает.
  const [guest, setGuest] = useState("");
  const [serverLink, setServerLink] = useState("");
  const [creating, setCreating] = useState(false);
  // Отказ сервера держим отдельно от ошибок сборки ссылки: место ошибки
  // должно совпадать с местом кнопки, иначе её просто не видно - она
  // уезжает под QR-код, и человек думает, что ничего не произошло.
  const [serverError, setServerError] = useState("");
  const api = backend();

  // Показывать ли ссылку, в которой только настройки, когда доступ можно
  // завести на сервере: по ней гость не подключится, и это решение
  // принимают осознанно, а не сканируя первый попавшийся QR.
  const [forceSettingsOnly, setForceSettingsOnly] = useState(false);

  const canAsk = profile.ssh.ip.trim() !== "";
  const manualId = clientId.trim() !== "";
  // Без Client ID в ссылке Android подставит id своего устройства, а его
  // нет в списке допущенных - сервер оборвёт каждый поток на рукопожатии.
  const settingsOnly = !serverLink && !manualId;
  const hidden = canAsk && settingsOnly && !forceSettingsOnly;
  const shown = hidden ? "" : serverLink || url;

  // Ссылку пересобираем на каждое изменение переключателей: иначе на экране
  // (и в QR-коде) остаётся прежняя, и получатель сканирует не то, что видит
  // отправитель.
  useEffect(() => guard("ShareDialog/build", () => {
    if (!api) return;

    let alive = true;
    // Небольшая задержка: Client ID набирают по символу, а каждая пересборка
    // перерисовывает и QR-код.
    const timer = setTimeout(() => {
      setError("");
      api
        .ExportLink(profile.id, includeVK, clientId)
        .then((v) => alive && setUrl(v))
        .catch((e) => alive && setError(String(e)));
    }, 200);
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }), [profile.id, includeVK, clientId]);

  return (
    <Modal title={`Поделиться профилем «${profile.name}»`} onClose={onClose}>
      <div className="flex flex-col gap-3">
        {canAsk ? (
          <div className="flex flex-col gap-2 rounded-xl border border-emerald-300 bg-emerald-50 p-3 dark:border-emerald-900 dark:bg-emerald-950/30">
            <span className="text-[13px] font-medium">Завести доступ на сервере</span>
            <span className="text-xs text-zinc-600 dark:text-zinc-400">
              Сервер выдаст гостю свой Client ID, внесёт его в список допущенных и вложит в
              ссылку конфигурацию. Без этого шага гость не подключится: сервер его не знает.
            </span>
            <div className="flex items-center gap-2 pt-1">
              <TextInput value={guest} onChange={setGuest} placeholder="Имя гостя, латиницей" />
              <Button
                variant="primary"
                busy={creating}
                disabled={creating || !validClientName(guest)}
                onClick={async () => {
                  setServerError("");
                  setCreating(true);
                  try {
                    setServerLink((await api?.VPSAddPeer(profile.id, guest.trim())) ?? "");
                  } catch (e) {
                    setServerError(String(e));
                  } finally {
                    setCreating(false);
                  }
                }}
              >
                Создать
              </Button>
            </div>
            {guest.trim() !== "" && !validClientName(guest) && (
              <span className="text-xs text-amber-700 dark:text-amber-500">
                Сервер принимает латиницу, цифры, точку, дефис и подчёркивание — до 32 символов
              </span>
            )}
            {serverError && (
              <p className="whitespace-pre-wrap rounded-lg bg-red-100 px-3 py-2 text-xs text-red-900 dark:bg-red-950 dark:text-red-200">
                {serverError}
              </p>
            )}
          </div>
        ) : (
          <p className="rounded-xl bg-amber-100 px-3 py-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
            В профиле нет доступа по SSH, поэтому завести гостя на сервере отсюда нельзя. Ссылка
            ниже несёт только настройки: владелец сервера должен добавить Client ID гостя в
            список допущенных, иначе подключение не поднимется.
          </p>
        )}

        {serverLink && (
          <p className="text-xs text-emerald-700 dark:text-emerald-400">
            Доступ заведён — ссылка ниже уже содержит Client ID гостя и его конфигурацию.
          </p>
        )}

        <CheckBox
          checked={includeVK}
          onChange={setIncludeVK}
          label="Вложить свою ссылку на звонок VK"
        />
        <p className="-mt-2 text-xs text-zinc-500">
          Обычно получатель создаёт свой звонок: ссылка уникальна для каждого клиента.
        </p>
        {includeVK && profile.client.vkLink.trim() === "" && (
          <p className="-mt-1 text-xs text-amber-600 dark:text-amber-400">
            В профиле не заполнена ссылка на звонок, вкладывать нечего — получателю
            придётся вписать свою.
          </p>
        )}
        {includeVK && serverLink && (
          <p className="-mt-1 text-xs text-amber-600 dark:text-amber-400">
            К ссылке от сервера звонок не подставляется — передайте её гостю отдельно.
          </p>
        )}
        <Field
          label="Client ID для получателя"
          hint="Только для ручного пути: владелец сервера должен сам внести его в список допущенных"
        >
          <div className="flex items-center gap-2">
            <TextInput value={clientId} onChange={setClientId} mono placeholder="необязательно" />
            <Button onClick={async () => setClientId((await api?.GenerateClientID()) ?? "")}>Создать</Button>
          </div>
        </Field>

        <div className="flex items-center gap-2">
          <Button
            busy={saving}
            disabled={hidden}
            onClick={async () => {
              setError("");
              setSaving(true);
              try {
                // В файл уходит то же, что на экране: гостевая ссылка от
                // сервера, а не заново собранная из профиля.
                const path = serverLink
                  ? await api?.SaveLinkToFile(guest.trim() || profile.name, serverLink)
                  : await api?.ExportLinkToFile(profile.id, includeVK, clientId);
                if (path) setSaved(path);
              } catch (e) {
                setError(String(e));
              } finally {
                setSaving(false);
              }
            }}
          >
            Сохранить в файл
          </Button>
        </div>

        {hidden && (
          <div className="flex flex-col gap-1.5 rounded-xl border border-dashed border-zinc-300 p-4 text-center dark:border-zinc-700">
            <span className="text-[13px] text-zinc-700 dark:text-zinc-300">
              QR-код появится, когда сервер заведёт доступ — кнопка «Создать» выше
            </span>
            <span className="text-xs text-zinc-500">
              Ссылка без Client ID несёт только настройки: телефон подставит свой id, сервер его не знает
              и не пустит.
            </span>
            <button
              onClick={() => setForceSettingsOnly(true)}
              className="mx-auto text-xs text-zinc-500 underline transition hover:text-zinc-700 dark:hover:text-zinc-300"
            >
              Всё равно показать ссылку только с настройками
            </button>
          </div>
        )}

        {shown && settingsOnly && canAsk && (
          <p className="rounded-xl bg-amber-100 px-3 py-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
            <b>Только настройки — подключиться по этой ссылке не выйдет.</b> Клиент подставит id своего
            устройства, а сервер пускает только Client ID из списка допущенных.
          </p>
        )}
        {shown && manualId && !serverLink && (
          <p className="rounded-xl bg-amber-100 px-3 py-2 text-xs text-amber-900 dark:bg-amber-950 dark:text-amber-200">
            Client ID <span className="font-mono">{clientId.trim()}</span> нужно вручную внести в список
            допущенных на сервере, иначе получатель не подключится.
          </p>
        )}

        {shown && (
          <>
            <QRCode text={shown} name={guest.trim() || profile.name} />
            <Field label="Ссылка">
              <TextArea value={shown} onChange={() => {}} rows={4} />
            </Field>
            <Button onClick={() => navigator.clipboard.writeText(shown)}>Скопировать</Button>
          </>
        )}
        {saved && <p className="text-sm text-emerald-600">Сохранено: {saved}</p>}
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
      </div>
    </Modal>
  );
}
