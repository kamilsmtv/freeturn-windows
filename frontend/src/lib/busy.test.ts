import { beforeEach, describe, expect, it } from "vitest";
import { busyLabel, setBusy } from "./busy";

describe("общий признак ожидания", () => {
  beforeEach(() => {
    setBusy("a", false);
    setBusy("b", false);
  });

  it("пусто, пока никто не занят", () => {
    expect(busyLabel()).toBe("");
  });

  it("показывает подпись занятой операции", () => {
    setBusy("a", true, "Запуск ядра");
    expect(busyLabel()).toBe("Запуск ядра");
  });

  // Разделы работают одновременно: полоска гаснет, только когда закончили все.
  it("держится, пока занят хоть один раздел", () => {
    setBusy("a", true, "Запуск ядра");
    setBusy("b", true, "Команда серверу");
    setBusy("a", false);
    expect(busyLabel()).toBe("Команда серверу");
    setBusy("b", false);
    expect(busyLabel()).toBe("");
  });

  // Отметку снимает размонтирование компонента: лишний вызов не должен
  // гасить полоску, зажжённую другим разделом.
  it("снятие несуществующей отметки ничего не меняет", () => {
    setBusy("a", true, "Запуск ядра");
    setBusy("нет-такого", false);
    expect(busyLabel()).toBe("Запуск ядра");
  });
});
