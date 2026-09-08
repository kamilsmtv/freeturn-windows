import { useEffect, useState } from "react";
import { backend } from "../lib/api";
import { guard } from "../lib/effect";
import { Button } from "./ui";

/**
 * Ссылка QR-кодом - как в Android-клиенте: получателю достаточно навести
 * камеру, пересылать длинную строку через мессенджер не нужно.
 *
 * Картинку рисует бэкенд и отдаёт как data URI: так в интерфейсе не нужен
 * ещё один пакет, а сохранить в файл можно тем же кодом.
 */
export function QRCode({ text, name }: { text: string; name: string }) {
  const [src, setSrc] = useState("");
  const [note, setNote] = useState("");
  const [saved, setSaved] = useState("");
  const api = backend();

  useEffect(() => guard("QRCode/render", () => {
    setSrc("");
    setNote("");
    setSaved("");
    if (!api || !text) return;

    let alive = true;
    api
      .QRCode(text)
      .then((uri) => alive && setSrc(uri))
      // Слишком длинная ссылка - не ошибка, а причина показать текст вместо кода.
      .catch((e) => alive && setNote(String(e)));
    return () => {
      alive = false;
    };
  }), [text]);

  if (note) {
    return <p className="text-xs text-zinc-500 dark:text-zinc-400">{note}: передайте её текстом.</p>;
  }
  if (!src) return null;

  return (
    <div className="flex flex-col items-center gap-2.5">
      {/* Код всегда чёрный на белом: сканеру важен контраст, а не тема окна. */}
      <div className="rounded-2xl bg-white p-3">
        <img
          src={src}
          alt="QR-код ссылки"
          width={200}
          height={200}
          // Без сглаживания модули остаются резкими при любом масштабе.
          style={{ imageRendering: "pixelated" }}
        />
      </div>
      <div className="flex items-center gap-2">
        <Button
          onClick={async () => {
            setSaved("");
            try {
              const path = await api?.SaveQRCode(text, name);
              if (path) setSaved(path);
            } catch (e) {
              setSaved(String(e));
            }
          }}
        >
          Сохранить PNG
        </Button>
      </div>
      {saved && <p className="max-w-full break-all text-xs text-zinc-500 dark:text-zinc-400">{saved}</p>}
    </div>
  );
}
