import { describe, expect, it } from "vitest";
import type { CoreStatus, Profile, TunnelStatus } from "./api";
import { connectionPhase, isVPN, runningProfile } from "./phase";

function profile(id: string, transport: string): Profile {
  return { id, name: id, client: { tunnelTransport: transport } } as unknown as Profile;
}

const core = (state: CoreStatus["state"], profileId = "vpn"): CoreStatus =>
  ({ state, profileId, pid: 1, version: "v3.4.0", startedAt: "", error: "" }) as CoreStatus;

const tunnel = (up: boolean, error = ""): TunnelStatus =>
  ({ enabled: true, up, adapter: up ? "freeturn-wg" : "", error }) as TunnelStatus;

const vpn = profile("vpn", "wireguard");
const proxy = profile("proxy", "none");

describe("фаза подключения", () => {
  it("без ядра - отключено", () => {
    const p = connectionPhase(null, null, vpn);
    expect(p.text).toBe("Отключено");
    expect(p.live).toBe(false);
    expect(p.busy).toBe(false);
  });

  it("запуск ядра - переход, а не связь", () => {
    const p = connectionPhase(core("starting"), null, vpn);
    expect(p.busy).toBe(true);
    expect(p.live).toBe(false);
  });

  // Главное, ради чего фаза и появилась: запущенное ядро в режиме VPN -
  // это ещё не подключение, пока не поднят туннель.
  it("ядро работает, туннеля нет - «поднимаю туннель»", () => {
    const p = connectionPhase(core("running"), tunnel(false), vpn);
    expect(p.text).toBe("Поднимаю туннель…");
    expect(p.busy).toBe(true);
    expect(p.live).toBe(false);
  });

  it("туннель поднят - подключено", () => {
    const p = connectionPhase(core("running"), tunnel(true), vpn);
    expect(p.text).toBe("Подключено");
    expect(p.live).toBe(true);
    expect(p.busy).toBe(false);
  });

  it("в режиме прокси связь есть сразу после запуска ядра", () => {
    const p = connectionPhase(core("running", "proxy"), null, proxy);
    expect(p.live).toBe(true);
  });

  it("сорванный туннель - ошибка с причиной", () => {
    const p = connectionPhase(core("running"), tunnel(false, "рукопожатие не прошло"), vpn);
    expect(p.tone).toBe("error");
    expect(p.hint).toBe("рукопожатие не прошло");
    expect(p.live).toBe(false);
  });

  it("остановка - переход без связи", () => {
    const p = connectionPhase(core("stopping"), tunnel(true), vpn);
    expect(p.busy).toBe(true);
    expect(p.live).toBe(false);
  });

  it("упавшее ядро - ошибка с его текстом", () => {
    const c = { ...core("failed"), error: "ядро не запустилось" };
    expect(connectionPhase(c, null, vpn).hint).toBe("ядро не запустилось");
  });
});

describe("профиль, с которым работает ядро", () => {
  // Активный профиль переключают, не отключаясь: судить о режиме надо по
  // тому, что реально запущено, иначе VPN-сеанс выглядит как прокси.
  it("берётся по profileId ядра, а не по активному", () => {
    const list = [vpn, proxy];
    expect(runningProfile(core("running", "vpn"), list, proxy)?.id).toBe("vpn");
  });

  it("без profileId остаётся активный", () => {
    expect(runningProfile(core("stopped", ""), [vpn, proxy], proxy)?.id).toBe("proxy");
  });

  it("неизвестный profileId не роняет фазу", () => {
    expect(runningProfile(core("running", "нет-такого"), [vpn], vpn)?.id).toBe("vpn");
  });
});

describe("режим профиля", () => {
  it("wireguard - это VPN", () => {
    expect(isVPN(vpn)).toBe(true);
    expect(isVPN(proxy)).toBe(false);
    expect(isVPN(null)).toBe(false);
  });
});
