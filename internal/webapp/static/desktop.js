/** Ambiente Linux simplificado — simulação de desktop no browser para utilizadores leigos. */
(function () {
  const $ = (sel, root) => (root || document).querySelector(sel);

  const APPS = [
    { id: "files", screen: "explorer", icon: "📁", label: "Arquivos", desc: "Pastas e transferências" },
    { id: "docker", screen: "docker", icon: "🐳", label: "Aplicações", desc: "Contêineres Docker" },
    { id: "disks", screen: "disks", icon: "💾", label: "Discos", desc: "Espaço e armazenamento" },
    { id: "terminal", screen: "terminal", icon: "⌨️", label: "Terminal", desc: "Linha de comandos" },
    { id: "auto", screen: "automations", icon: "⚙️", label: "Automações", desc: "Tarefas agendadas" },
    { id: "about", screen: null, icon: "ℹ️", label: "Sobre o servidor", desc: "Estado e recursos" },
    { id: "help", screen: null, icon: "❓", label: "Ajuda rápida", desc: "Como usar este ambiente" },
  ];

  let pollTimer = null;
  let clockTimer = null;
  let overviewCache = null;

  function escapeHtml(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function formatBytes(n) {
    if (!n || n < 0) return "—";
    const u = ["B", "KB", "MB", "GB", "TB"];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < u.length - 1) {
      v /= 1024;
      i++;
    }
    return `${v.toFixed(i ? 1 : 0)} ${u[i]}`;
  }

  function pctBar(pct) {
    const p = Math.min(100, Math.max(0, Number(pct) || 0));
    return `<div class="linux-meter"><div class="linux-meter-fill" style="width:${p.toFixed(0)}%"></div></div><span class="linux-meter-label">${p.toFixed(0)}%</span>`;
  }

  function visibleApps() {
    return APPS.filter((a) => !a.screen || (typeof canScreen === "function" && canScreen(a.screen)));
  }

  function launchApp(app) {
    if (app.screen) {
      closeStartMenu();
      closeLinuxWindow();
      if (typeof showScreen === "function") showScreen(app.screen);
      if (app.screen === "docker" && typeof loadDocker === "function") loadDocker();
      if (app.screen === "disks" && typeof loadDisks === "function") loadDisks();
      return;
    }
    if (app.id === "about") openAboutWindow();
    if (app.id === "help") openHelpWindow();
  }

  function makeIconButton(app, extraClass) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = `linux-icon-btn ${extraClass || ""}`.trim();
    btn.setAttribute("role", "listitem");
    btn.title = app.desc;
    btn.innerHTML = `<span class="linux-icon-glyph" aria-hidden="true">${app.icon}</span><span class="linux-icon-label">${escapeHtml(app.label)}</span>`;
    btn.addEventListener("click", () => launchApp(app));
    return btn;
  }

  function renderIcons() {
    const desk = $("#linux-icons");
    const dock = $("#linux-dock");
    const start = $("#linux-start-grid");
    if (!desk || !dock || !start) return;
    desk.innerHTML = "";
    dock.innerHTML = "";
    start.innerHTML = "";
    const apps = visibleApps();
    for (const app of apps) {
      if (app.id !== "help") desk.appendChild(makeIconButton(app));
      dock.appendChild(makeIconButton(app, "linux-dock-btn"));
      const tile = document.createElement("button");
      tile.type = "button";
      tile.className = "linux-start-tile";
      tile.innerHTML = `<span class="linux-start-tile-icon">${app.icon}</span><span class="linux-start-tile-text"><strong>${escapeHtml(app.label)}</strong><span class="muted">${escapeHtml(app.desc)}</span></span>`;
      tile.addEventListener("click", () => launchApp(app));
      start.appendChild(tile);
    }
  }

  function updatePanelChrome() {
    const host = $("#linux-panel-host");
    const ssh = $("#linux-panel-ssh");
    if (host) {
      const h = state.ssh.host || "servidor";
      const u = state.ssh.user || "";
      host.textContent = u ? `${u} · ${h}` : h;
    }
    if (ssh) {
      const on = state.ssh.connected;
      ssh.textContent = on ? "Ligado" : "Desligado";
      ssh.classList.toggle("is-online", on);
      ssh.classList.toggle("is-offline", !on);
    }
  }

  function tickClock() {
    const el = $("#linux-panel-clock");
    if (!el) return;
    const now = new Date();
    el.textContent = now.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    el.dateTime = now.toISOString();
  }

  function closeStartMenu() {
    const menu = $("#linux-start-menu");
    const btn = $("#linux-btn-menu");
    if (!menu) return;
    menu.hidden = true;
    btn?.setAttribute("aria-expanded", "false");
  }

  function toggleStartMenu() {
    const menu = $("#linux-start-menu");
    const btn = $("#linux-btn-menu");
    if (!menu) return;
    const open = menu.hidden;
    menu.hidden = !open;
    btn?.setAttribute("aria-expanded", open ? "true" : "false");
    if (open) closeLinuxWindow();
  }

  function closeLinuxWindow() {
    const layer = $("#linux-window-layer");
    if (!layer) return;
    layer.hidden = true;
    layer.innerHTML = "";
  }

  function openLinuxWindow(title, bodyHtml) {
    const layer = $("#linux-window-layer");
    if (!layer) return;
    closeStartMenu();
    layer.hidden = false;
    layer.innerHTML = `
      <div class="linux-window glass-card" role="dialog" aria-label="${escapeHtml(title)}">
        <header class="linux-window-head">
          <span class="linux-window-dots" aria-hidden="true"><i></i><i></i><i></i></span>
          <h3 class="linux-window-title">${escapeHtml(title)}</h3>
          <button type="button" class="linux-window-close" aria-label="Fechar">×</button>
        </header>
        <div class="linux-window-body">${bodyHtml}</div>
      </div>`;
    layer.querySelector(".linux-window-close")?.addEventListener("click", closeLinuxWindow);
    layer.addEventListener("click", (ev) => {
      if (ev.target === layer) closeLinuxWindow();
    }, { once: true });
  }

  function aboutBody(ov) {
    if (!ov) {
      return `<p class="muted">A carregar informação do servidor…</p>`;
    }
    return `
      <dl class="linux-info-grid">
        <dt>Nome</dt><dd>${escapeHtml(ov.hostname || "—")}</dd>
        <dt>Sistema</dt><dd>${escapeHtml(ov.os || "—")}</dd>
        <dt>Ligado há</dt><dd>${escapeHtml(ov.uptime || "—")}</dd>
        <dt>Processadores</dt><dd>${ov.cpus || "—"} núcleos</dd>
        <dt>Memória</dt><dd>${pctBar(ov.memPct)} <span class="muted">${formatBytes(ov.memUsed)} / ${formatBytes(ov.memTotal)}</span></dd>
        <dt>Disco (/) </dt><dd>${pctBar(ov.diskPct)} <span class="muted">${formatBytes(ov.diskUsed)} / ${formatBytes(ov.diskTotal)}</span></dd>
        <dt>Carga</dt><dd>${escapeHtml(ov.load1)} · ${escapeHtml(ov.load5)} · ${escapeHtml(ov.load15)}</dd>
      </dl>
      <p class="muted linux-window-foot">Atualizado às ${escapeHtml(ov.updatedAt || "")}. Dados lidos por SSH — não altera o Ubuntu Server.</p>`;
  }

  function openAboutWindow() {
    openLinuxWindow("Sobre o servidor", aboutBody(overviewCache));
    refreshOverview(true);
  }

  function openHelpWindow() {
    openLinuxWindow(
      "Ajuda rápida",
      `<ul class="linux-help-list">
        <li>Este ambiente <strong>não instala</strong> interface gráfica no servidor — é uma vista amigável no browser.</li>
        <li>Clique nos ícones ou no menu <strong>CW</strong> para abrir ficheiros, Docker, discos e outras ferramentas.</li>
        <li><strong>Menu técnico</strong> abre o hub original com todos os módulos e opções avançadas.</li>
        <li>Para desligar a sessão use <strong>Desconectar</strong> ou <strong>Sair</strong> na barra superior.</li>
      </ul>
      <p class="muted">Atalho: <kbd>?</kbd> abre ajuda da tela atual; <kbd>Ctrl</kbd>+<kbd>K</kbd> paleta de comandos.</p>`
    );
  }

  async function refreshOverview(silent) {
    if (!state.ssh.connected) return;
    try {
      const ov = await api("/api/desktop/overview");
      overviewCache = ov;
      const layer = $("#linux-window-layer");
      const dlg = layer?.querySelector(".linux-window");
      if (dlg && dlg.getAttribute("aria-label") === "Sobre o servidor") {
        const body = dlg.querySelector(".linux-window-body");
        if (body) body.innerHTML = aboutBody(ov);
      }
    } catch (e) {
      if (!silent) CWUI.toast(e.message, "error");
    }
  }

  function startPoll() {
    stopPoll();
    refreshOverview(true);
    pollTimer = setInterval(() => refreshOverview(true), 45000);
    tickClock();
    clockTimer = setInterval(tickClock, 1000);
  }

  function stopPoll() {
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = null;
    if (clockTimer) clearInterval(clockTimer);
    clockTimer = null;
  }

  function onEnter() {
    document.getElementById("app")?.classList.add("app-desktop-mode");
    renderIcons();
    updatePanelChrome();
    startPoll();
  }

  function onLeave() {
    document.getElementById("app")?.classList.remove("app-desktop-mode");
    closeStartMenu();
    closeLinuxWindow();
    stopPoll();
  }

  $("#linux-btn-menu")?.addEventListener("click", (ev) => {
    ev.stopPropagation();
    toggleStartMenu();
  });
  $("#linux-start-close")?.addEventListener("click", closeStartMenu);
  $("#linux-start-hub")?.addEventListener("click", () => {
    closeStartMenu();
    showScreen("hub");
    renderHub();
  });
  $("#linux-btn-hub")?.addEventListener("click", () => {
    closeStartMenu();
    showScreen("hub");
    renderHub();
  });

  document.addEventListener("click", (ev) => {
    const menu = $("#linux-start-menu");
    if (!menu || menu.hidden) return;
    if (menu.contains(ev.target) || $("#linux-btn-menu")?.contains(ev.target)) return;
    closeStartMenu();
  });

  document.addEventListener("keydown", (ev) => {
    if (ev.key === "Escape" && state.screen === "desktop") {
      if (!$("#linux-start-menu")?.hidden) {
        closeStartMenu();
        ev.preventDefault();
        return;
      }
      if (!$("#linux-window-layer")?.hidden) {
        closeLinuxWindow();
        ev.preventDefault();
      }
    }
  });

  window.CWDesktop = { onEnter, onLeave, refreshOverview };
})();
