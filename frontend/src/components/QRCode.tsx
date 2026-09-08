import { useEffect, useState } from "react";
import { backend, type QRImage } from "../lib/api";
import { guard } from "../lib/effect";
import { Button } from "./ui";

/**
 * Место под код на экране постоянно: сам код меняет размер вместе с длиной
 * ссылки, и если бы он двигал соседей, при правке Client ID прыгал бы весь
 * диалог. Ориентир взят у Android-клиента - около 320 dp.
 */
const BOX_SIDE = 300;
const CODE_SIDE = 288;

/**
 * Ссылка QR-кодом - как в Android-клиенте: получателю достаточно навести
 * камеру, пересылать длинную строку через мессенджер не нужно.
 *
 * Картинку рисует бэкенд и отдаёт как data URI: так в интерфейсе не нужен
 * ещё один пакет, а сохранить в файл можно тем же кодом.
 */
export function QRCode({ text, name }: { text: string; name: string }) {
  const [img, setImg] = useState<QRImage | null>(null);
  const [note, setNote] = useState("");
  const [saved, setSaved] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const api = backend();

  useEffect(() => guard("QRCode/render", () => {
    setNote("");
    setSaved("");
    setError("");
    if (!api || !text) return;

    let alive = true;
    // Прежний код не убираем до готовности нового: иначе на каждое нажатие
    // клавиши блок исчезал бы и появлялся.
    api
      .QRCode(text)
      .then((v) => alive && setImg(v))
      // Слишком длинная ссылка - не ошибка, а причина показать текст вместо кода.
      .catch((e) => {
        if (alive) {
          setImg(null);
          setNote(String(e));
        }
      });
    return () => {
      alive = false;
    };
  }), [text]);

  if (note) {
    return <p className="text-xs text-zinc-500 dark:text-zinc-400">{note}: передайте её текстом.</p>;
  }
  if (!img) return null;

  // Модуль обязан занимать целое число пикселей: иначе границы плывут и
  // камера не разбирает плотный код гостевой ссылки. Поэтому берём ближайший
  // кратный размер снизу - код всегда помещается в отведённое место.
  const modules = Math.max(img.modules, 1);
  const side = modules * Math.max(1, Math.floor(CODE_SIDE / modules));

  return (
    <div className="flex flex-col items-center gap-2.5">
      {/* Код всегда чёрный на белом: сканеру важен контраст, а не тема окна. */}
      <div
        className="flex items-center justify-center rounded-2xl bg-white"
        style={{ width: BOX_SIDE, height: BOX_SIDE }}
      >
        <img src={img.uri} alt="QR-код ссылки" width={side} height={side} />
      </div>
      <Button
        busy={saving}
        onClick={async () => {
          setSaved("");
          setError("");
          setSaving(true);
          try {
            const path = await api?.SaveQRCode(text, name);
            if (path) setSaved(path);
          } catch (e) {
            setError(String(e));
          } finally {
            setSaving(false);
          }
        }}
      >
        Сохранить PNG
      </Button>
      {saved && <p className="max-w-full break-all text-xs text-zinc-500 dark:text-zinc-400">{saved}</p>}
      {error && <p className="max-w-full break-all text-xs text-red-600 dark:text-red-400">{error}</p>}
    </div>
  );
}
