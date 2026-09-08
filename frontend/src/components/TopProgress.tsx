/**
 * Общая полоска ожидания вверху окна.
 *
 * Держится одна на всё приложение: длинные операции - подключение, команды
 * серверу, скачивание ядра - идут в разных разделах, и отдельные спиннеры
 * пришлось бы искать глазами. Полоска же всегда на одном месте.
 */
export function TopProgress({ active, label }: { active: boolean; label: string }) {
  return (
    <div className="relative h-0.5 shrink-0 overflow-hidden bg-transparent" aria-hidden={!active}>
      {active && (
        <>
          <div className="absolute inset-0 bg-emerald-500/15" />
          <div className="progress-bar absolute inset-y-0 w-1/3 bg-emerald-500" title={label} />
        </>
      )}
    </div>
  );
}
