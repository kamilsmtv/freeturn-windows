import { useEffect, useState } from "react";
import { backend, type QRImage } from "../lib/api";
import { guard } from "../lib/effect";
import { Button } from "./ui";

/** Ориентир размера на экране - как в Android-клиенте, 320 dp. */
const TARGET_SIDE = 320;

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
  const api = backend();

  useEffect(() => guard("QRCode/render", () => {
    setImg(null);
    setNote("");
    setSaved("");
    setError("");
    if (!api || !text) return;

    let alive = true;
    api
      .QRCode(text)
      .then((v) => alive && setImg(v))
      // Слишком длинная ссылка - не ошибка, а причина показать текст вместо кода.
      .catch((e) => alive && setNote(String(e)));
    return () => {
      alive = false;
    };
  }), [text]);

  if (note) {
    return <p className="text-xs text-zinc-500 dark:text-zinc-400">{note}: передайте её текстом.</p>;
  }
  if (!img) return null;

  // Модуль обязан занимать целое число пикселей: иначе границы плывут и
  // камера не разбирает плотный код гостевой ссылки. Поэтому размер не
  // подгоняется под 320 точно, а берётся ближайший кратный снизу; на очень
  // длинной ссылке двух пикселей на модуль хватает, и код выходит шире.
  const modules = Math.max(img.modules, 1);
  const side = modules * Math.max(2, Math.floor(TARGET_SIDE / modules));

  return (
    <div className="flex flex-col items-center gap-2.5">
      {/* Код всегда чёрный на белом: сканеру важен контраст, а не тема окна.
          Размер подобран так, чтобы диалог помещался в минимальное окно. */}
      <div className="rounded-2xl bg-white p-2.5">
        <img src={img.uri} alt="QR-код ссылки" width={side} height={side} />
      </div>
      <Button
        onClick={async () => {
          setSaved("");
          setError("");
          try {
            const path = await api?.SaveQRCode(text, name);
            if (path) setSaved(path);
          } catch (e) {
            setError(String(e));
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
