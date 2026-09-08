// Подставной бэкенд для снимков интерфейса: тот же собранный фронтенд,
// но данные демонстрационные (адреса из диапазонов для документации).
(function () {
  var screenIndex = Number(new URLSearchParams(location.search).get("screen") || 0);

  var kcp = { noDelay: 1, interval: 10, resend: 2, nc: 1, sndWnd: 1024, rcvWnd: 1024, mtu: 1350, ackNoDelay: true };
  function profile(id, name, addr, obf, vpn, sub) {
    return {
      id: id, name: name,
      ssh: { ip: addr.split(":")[0], port: 22, username: "root", password: "", authType: "PASSWORD", sshKey: "",
             hostFingerprint: "SHA256:8Xn2f2c1Qy0KpQ0mS2t9r5Jw", rootMode: "ROOT", sudoPassword: "" },
      client: { serverAddress: addr, vkLink: "https://vk.ru/call/join/EXAMPLE", provider: "vk", threads: 12,
                streamsPerCred: 3, useUdp: true, manualCaptcha: false, localPort: "127.0.0.1:9000", debugMode: false,
                dnsMode: "system", customDns: "", platform: "desktop", magicSwitch: false, magicTurn: "", magicPort: "",
                routes: true, tunnelTransport: vpn ? "wireguard" : "none", wireGuardConfig: vpn ? "[Interface]" : "",
                wireGuardTunnelName: "freeturn-wg", splitTunnelMode: "off", splitTunnelSubnets: "", logsEnabled: true,
                clientId: "c64a26ffcfd0540cecfd6fa2dbade293" },
      proxyListen: "0.0.0.0:56000", proxyConnect: "127.0.0.1:51820",
      opts: { obfProfile: obf, obfKey: "a".repeat(64), obfTimingMs: 0, proxyMode: "udp", kcp: kcp },
      subUrl: sub ? "https://example.com/sub" : undefined,
    };
  }

  var list = [
    profile("nl01", "vdsina NL01", "203.0.113.10:56000", "rtpopus", true, false),
    profile("fi", "Резерв FI", "198.51.100.20:56000", "rtpopus3", false, false),
    profile("de1", "Hetzner DE-1", "203.0.113.44:56000", "rtpopus3", false, true),
    profile("pl", "Warszawa PL-1", "198.51.100.88:56000", "rtpopus2", false, true),
  ];

  var started = new Date(Date.now() - 64 * 60 * 1000).toISOString();
  var logLines = [
    "Free Turn Proxy client version=3.4.0",
    "Client ID: c64a26ffcfd0540cecfd6fa2dbade293",
    "provider=vk",
    "OBF profile=rtpopus: peer server must use matching -obf-profile and -obf-key",
    "route manager: gateway=192.168.1.1",
    "[DNS] UDP/53 probe OK, using UDP",
    "[STREAM 1] [Captcha] solver succeeded",
    "Ensuring route to 203.0.113.143/32 via 192.168.1.1",
    "[STREAM 1] TURN allocation up: relayed=203.0.113.143:47075",
    "Туннель поднят: адаптер freeturn-wg, DNS 1.1.1.1, маршруты 0.0.0.0/0",
  ].map(function (text, i) {
    return { seq: i + 1, time: "23:24:" + String(12 + i).padStart(2, "0"), level: i === 3 ? "warn" : "info", text: text };
  });

  var probe = {
    installed: true, version: "v3.4.0", running: true, mode: "udp", obf: "rtpopus", runtime: "systemd", euid: 0,
    wg: { present: true, port: 51820 }, virt: "kvm", wg_kernel: true,
    conflicts: { warp: false, x3ui: true, wgeasy: false, tailscale: false, other_ifaces: null },
  };

  var settings = {
    theme: "dark", minimizeToTray: true, startMinimized: false, autostart: true, autoConnectLast: true,
    coreUpdateMode: "ask", coreUpdateHours: 6, checkGuiUpdates: true, githubToken: "", subRefreshHours: 12,
    subscriptions: ["https://example.com/sub"], subLastRefresh: new Date().toISOString(),
    ownClientId: "c64a26ffcfd0540cecfd6fa2dbade293", lastProfileId: "nl01", keepLogLines: 5000,
    suppressAdminWarn: false, coreVersionChecked: new Date().toISOString(),
  };

  var ok = function (v) { return function () { return Promise.resolve(v); }; };

  // Демонстрационная ссылка и заглушка QR: настоящий код рисует бэкенд.
  var DEMO_LINK = "freeturn://eyJ2IjoxLCJuYW1lIjoidmRzaW5hIE5MMDEiLCJob3N0IjoiMjAzLjAuMTEzLjEwIn0";
  function qrDataURI() {
    var n = 65;
    var cell = 4;
    var svg = '<svg xmlns="http://www.w3.org/2000/svg" width="' + n * cell + '" height="' + n * cell + '">';
    svg += '<rect width="100%" height="100%" fill="#fff"/>';
    var seed = 7;
    for (var y = 0; y < n; y++) {
      for (var x = 0; x < n; x++) {
        seed = (seed * 1103515245 + 12345) % 2147483648;
        var finder = (x < 7 && y < 7) || (x >= n - 7 && y < 7) || (x < 7 && y >= n - 7);
        var on = finder ? (x % 6 === 0 || y % 6 === 0 || (x > 1 && x < 5 && y > 1 && y < 5)) : seed % 2 === 0;
        if (on) svg += '<rect x="' + x * cell + '" y="' + y * cell + '" width="' + cell + '" height="' + cell + '" fill="#000"/>';
      }
    }
    svg += "</svg>";
    return "data:image/svg+xml;base64," + btoa(svg);
  }

  window.runtime = { EventsOn: function () { return function () {}; }, LogError: function () {} };
  window.go = { main: { App: {
    Environment: ok({ admin: true, webView2: true, webView2Version: "126.0", windows: true }),
    Info: ok({ version: "0.1.0", dataDir: "%APPDATA%\\FreeTurn", coreDir: "%LOCALAPPDATA%\\FreeTurn\\core",
               logDir: "%APPDATA%\\FreeTurn\\logs", coreRepo: "samosvalishe/free-turn-proxy", guiRepo: "kamilsmtv/freeturn-windows" }),
    GetSettings: ok(settings), SaveSettings: ok(undefined), AutostartEnabled: ok(true),
    CoreStatus: ok({ state: "running", profileId: "nl01", pid: 4242, version: "v3.4.0", startedAt: started, error: "" }),
    CoreLog: ok(logLines), CoreClearLog: ok(undefined), CoreStop: ok(undefined),
    Profiles: ok({ list: list, activeId: "nl01" }),
    TrafficStats: ok({ available: true, adapter: "freeturn-wg", rxBytes: 193273528, txBytes: 1503238553, reason: "", rxRate: 0, txRate: 0 }),
    TunnelStatus: ok({ enabled: true, up: true, adapter: "freeturn-wg", error: "" }),
    UpdateStatus: ok({ installed: true, version: "v3.4.0", latestVersion: "v3.4.0", updateReady: false, changelog: "",
                       releaseUrl: "", canRollback: true, rollbackTarget: "v3.3.1", checkedAt: new Date().toISOString(), error: "" }),
    CheckGUIUpdate: ok({ current: "0.1.0", latest: "0.1.0", updateReady: false, changelog: "", releaseUrl: "", checkedAt: "", error: "" }),
    VPSProbe: ok({ ok: true, fingerprint: "", error: "", probe: probe }),
    ExportLink: function (id, includeVK) {
      return Promise.resolve(DEMO_LINK + (includeVK ? "LnZrLnJ1L2NhbGwvam9pbi9FWEFNUExF" : ""));
    },
    QRCode: function () { return Promise.resolve({ uri: qrDataURI(), modules: 65 }); },
    GenerateClientID: ok("c64a26ffcfd0540cecfd6fa2dbade293"),
    WintunInstalled: ok(true),
  } } };

  // Публичные адреса на снимках размываем: даже демонстрационные IP выглядят
  // как чей-то настоящий сервер. Служебные оставляем как есть - без них
  // экраны «Сервер» и «Журнал» теряют смысл.
  var KEEP = ["0.0.0.0", "255.255.255.255", "1.1.1.1", "1.0.0.1", "8.8.8.8", "8.8.4.4", "9.9.9.9"];

  function isServiceAddress(ip) {
    if (KEEP.indexOf(ip) !== -1) return true;
    var o = ip.split(".").map(Number);
    if (o.some(function (n) { return isNaN(n) || n > 255; })) return true; // не адрес - не трогаем
    if (o[0] === 127) return true;                                  // петля
    if (o[0] === 10) return true;                                   // RFC 1918
    if (o[0] === 172 && o[1] >= 16 && o[1] <= 31) return true;      // RFC 1918
    if (o[0] === 192 && o[1] === 168) return true;                  // RFC 1918
    if (o[0] === 169 && o[1] === 254) return true;                  // link-local
    return false;
  }

  function blurAddresses() {
    var style = document.createElement("style");
    style.textContent = ".shot-blur{filter:blur(5px);opacity:.85}";
    document.head.appendChild(style);

    var re = /\b\d{1,3}(?:\.\d{1,3}){3}\b/g;
    var walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    var nodes = [];
    while (walker.nextNode()) {
      if (walker.currentNode.nodeValue && /\d{1,3}(?:\.\d{1,3}){3}/.test(walker.currentNode.nodeValue)) {
        nodes.push(walker.currentNode);
      }
    }

    nodes.forEach(function (node) {
      var text = node.nodeValue;
      var frag = document.createDocumentFragment();
      var last = 0;
      var m;
      re.lastIndex = 0;
      while ((m = re.exec(text)) !== null) {
        if (m.index > last) frag.appendChild(document.createTextNode(text.slice(last, m.index)));
        if (isServiceAddress(m[0])) {
          frag.appendChild(document.createTextNode(m[0]));
        } else {
          var span = document.createElement("span");
          span.className = "shot-blur";
          span.textContent = m[0];
          frag.appendChild(span);
        }
        last = m.index + m[0].length;
      }
      if (last < text.length) frag.appendChild(document.createTextNode(text.slice(last)));
      node.parentNode.replaceChild(frag, node);
    });
  }

  // Нужный экран открываем кликом по разделу - как это делает пользователь.
  window.addEventListener("load", function () {
    setTimeout(function () {
      var items = document.querySelectorAll("nav button");
      if (screenIndex > 0 && items[screenIndex]) items[screenIndex].click();
      // Экран уже отрисован - размываем адреса перед самим снимком.
      setTimeout(blurAddresses, 400);
    }, 120);
  });
})();
