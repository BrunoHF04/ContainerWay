/** Ambiente Linux — sessão, atalhos, notificações, tour e helpers partilhados. */
(function () {
  const SESSION_KEY = "cw-desktop-session";
  const SESSION_SKIP_KEY = "cw-desktop-session-skip";
  const TOUR_KEY = "cw-desktop-tour-done";
  const RECENT_KEY = "cw-desktop-recent";
  const MAX_NOTIFS = 40;
  const MAX_RECENT = 6;

  const notifs = [];

  function escapeHtml(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function saveSession(collectWindows) {
    try {
      const data = {
        v: 1,
        savedAt: Date.now(),
        windows: collectWindows ? collectWindows() : [],
      };
      sessionStorage.setItem(SESSION_KEY, JSON.stringify(data));
    } catch (_) { /* quota */ }
  }

  function loadSession() {
    try {
      const raw = sessionStorage.getItem(SESSION_KEY);
      if (!raw) return null;
      return JSON.parse(raw);
    } catch {
      return null;
    }
  }

  function pushNotif(msg, level = "info", meta = {}) {
    notifs.unshift({ msg, level, at: Date.now(), appId: meta.appId || null });
    if (notifs.length > MAX_NOTIFS) notifs.length = MAX_NOTIFS;
    renderNotifs();
  }

  function clearNotifs() {
    notifs.length = 0;
    renderNotifs();
  }

  function pushRecentApp(appId) {
    if (!appId || appId === "help") return;
    try {
      let list = JSON.parse(localStorage.getItem(RECENT_KEY) || "[]");
      if (!Array.isArray(list)) list = [];
      list = list.filter((id) => id !== appId);
      list.unshift(appId);
      localStorage.setItem(RECENT_KEY, JSON.stringify(list.slice(0, MAX_RECENT)));
    } catch (_) { /* */ }
  }

  function getRecentApps() {
    try {
      const list = JSON.parse(localStorage.getItem(RECENT_KEY) || "[]");
      return Array.isArray(list) ? list : [];
    } catch {
      return [];
    }
  }

  function renderNotifs() {
    const badge = document.getElementById("linux-notify-badge");
    const list = document.getElementById("linux-notify-list");
    if (!badge || !list) return;
    const unread = notifs.length;
    badge.hidden = unread === 0;
    badge.textContent = unread > 9 ? "9+" : String(unread);
    list.innerHTML = "";
    if (!unread) {
      list.innerHTML = '<p class="muted linux-notify-empty">Sem notificações recentes.</p>';
      return;
    }
    const head = document.createElement("div");
    head.className = "linux-notify-actions";
    head.innerHTML = `<button type="button" class="btn btn-ghost btn-sm" id="linux-notify-clear">Limpar tudo</button>`;
    list.appendChild(head);
    head.querySelector("#linux-notify-clear")?.addEventListener("click", clearNotifs);
    const groups = [
      { key: "error", label: "Erros" },
      { key: "warning", label: "Avisos" },
      { key: "success", label: "Sucesso" },
      { key: "info", label: "Informação" },
    ];
    for (const g of groups) {
      const items = notifs.filter((n) => (n.level || "info") === g.key).slice(0, 6);
      if (!items.length) continue;
      const title = document.createElement("p");
      title.className = "linux-notify-group-title";
      title.textContent = g.label;
      list.appendChild(title);
      for (const n of items) {
        const el = document.createElement("div");
        el.className = `linux-notify-item linux-notify-item--${n.level}`;
        const openBtn =
          n.appId && window.CWDesktop
            ? `<button type="button" class="btn btn-ghost btn-sm linux-notify-open" data-app="${escapeHtml(n.appId)}">Abrir</button>`
            : "";
        el.innerHTML = `<span>${escapeHtml(n.msg)}</span><div class="linux-notify-item-foot"><time class="muted">${new Date(n.at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time>${openBtn}</div>`;
        list.appendChild(el);
      }
    }
    list.querySelectorAll(".linux-notify-open").forEach((btn) => {
      btn.addEventListener("click", () => {
        window.CWDesktop?.openApp?.(btn.dataset.app);
        document.getElementById("linux-notify-pop").hidden = true;
        document.getElementById("linux-btn-notify")?.setAttribute("aria-expanded", "false");
      });
    });
  }

  function hookToasts() {
    if (window.__cwDesktopToastHooked) return;
    window.__cwDesktopToastHooked = true;
    const orig = window.CWUI?.toast;
    if (!orig) return;
    window.CWUI.toast = function (msg, level) {
      if (document.getElementById("app")?.classList.contains("app-desktop-mode")) {
        pushNotif(String(msg || ""), level || "info", { appId: window.__cwToastAppId || null });
        window.__cwToastAppId = null;
      }
      return orig.apply(this, arguments);
    };
  }

  async function confirmDelete(opts) {
    const { target, containerName, itemName, isDir } = opts || {};
    const kind = isDir ? "a pasta" : "o arquivo";
    const where =
      target === "container"
        ? `dentro do container «${containerName || "Docker"}»`
        : "no servidor";
    if (typeof CWConfirm === "function") {
      return CWConfirm(`Excluir ${kind} «${itemName}» ${where}?`, { danger: true });
    }
    return confirm(`Excluir ${kind} «${itemName}» ${where}?`);
  }

  function renderPanelStats(overview, dockerCount, dockerRunning) {
    const el = document.getElementById("linux-panel-stats");
    if (!el) return;
    if (!state.ssh?.connected) {
      el.textContent = "—";
      el.title = "Sem conexão SSH";
      return;
    }
    const parts = [];
    if (dockerRunning != null) {
      parts.push(`🐳 ${dockerRunning}${dockerCount != null ? `/${dockerCount}` : ""}`);
    }
    if (overview?.memPct != null) {
      parts.push(`RAM ${overview.memPct.toFixed(0)}%`);
    }
    if (overview?.diskPct != null) {
      parts.push(`Disco ${overview.diskPct.toFixed(0)}%`);
    }
    el.textContent = parts.join(" · ") || "—";
    el.title = parts.join(" · ");
  }

  async function pollPanelStats() {
    if (state.screen !== "desktop" || !state.ssh?.connected) return;
    let ov = window.CWDesktop?.getOverviewCache?.();
    let running = null;
    let total = null;
    try {
      if (!ov) {
        ov = await api("/api/desktop/overview", { noAuthRedirect: true });
        window.CWDesktop?.setOverviewCache?.(ov);
      }
      const dc = await api("/api/docker/containers", { noAuthRedirect: true });
      const list = dc.containers || [];
      total = list.length;
      running = list.filter((c) => c.running || c.state === "running").length;
    } catch (_) { /* */ }
    renderPanelStats(ov, total, running);
  }

  function isDesktopOnboarded() {
    try {
      return localStorage.getItem(TOUR_KEY) === "1";
    } catch {
      return false;
    }
  }

  function markDesktopOnboarded() {
    try {
      localStorage.setItem(TOUR_KEY, "1");
    } catch (_) { /* */ }
    document.dispatchEvent(new CustomEvent("cw-desktop-onboarded"));
  }

  function showTourIfNeeded() {
    if (isDesktopOnboarded()) return;
    const dlg = document.getElementById("linux-tour-dialog");
    if (!dlg?.showModal) return;
    dlg.showModal();
  }

  function bindTour() {
    const dlg = document.getElementById("linux-tour-dialog");
    document.getElementById("linux-tour-skip")?.addEventListener("click", () => {
      markDesktopOnboarded();
      dlg?.close();
    });
    document.getElementById("linux-tour-start")?.addEventListener("click", () => {
      markDesktopOnboarded();
      dlg?.close();
      window.CWDesktop?.openApp?.("files");
    });
    if (dlg && !dlg.dataset.cwTourBound) {
      dlg.dataset.cwTourBound = "1";
      dlg.addEventListener("close", () => markDesktopOnboarded());
    }
  }

  function bindNotifyMenu() {
    const btn = document.getElementById("linux-btn-notify");
    const pop = document.getElementById("linux-notify-pop");
    if (!btn || !pop) return;
    btn.addEventListener("click", (ev) => {
      ev.stopPropagation();
      const open = pop.hidden;
      pop.hidden = !open;
      btn.setAttribute("aria-expanded", open ? "true" : "false");
      if (open) renderNotifs();
    });
    document.addEventListener("click", (ev) => {
      if (pop.hidden) return;
      if (pop.contains(ev.target) || btn.contains(ev.target)) return;
      pop.hidden = true;
      btn.setAttribute("aria-expanded", "false");
    });
  }

  function bindShortcuts() {
    document.addEventListener("keydown", (ev) => {
      if (state.screen !== "desktop") return;
      const tag = (ev.target?.tagName || "").toLowerCase();
      if (tag === "input" || tag === "textarea" || tag === "select" || ev.target?.isContentEditable) {
        if (!(ev.altKey && ev.key === "Tab")) return;
      }
      const mod = ev.ctrlKey || ev.metaKey;
      if (mod && ev.key.toLowerCase() === "w") {
        ev.preventDefault();
        window.CWDesktop?.closeFocusedWindow?.();
        return;
      }
      if (mod && ev.key.toLowerCase() === "m") {
        ev.preventDefault();
        window.CWDesktop?.minimizeAll?.();
        return;
      }
      if (ev.altKey && ev.key === "Tab") {
        ev.preventDefault();
        window.CWDesktop?.cycleWindows?.(ev.shiftKey);
        return;
      }
      if (ev.key === "?" && !mod) {
        ev.preventDefault();
        window.CWDesktop?.openApp?.("help");
      }
    });
  }

  function setSSHOverlay(connected) {
    const desk = document.getElementById("linux-desktop");
    if (!desk) return;
    desk.classList.toggle("is-ssh-down", !connected);
    const banner = document.getElementById("linux-ssh-banner");
    if (banner) banner.hidden = !!connected;
  }

  function onDesktopEnter() {
    hookToasts();
    window.CWDesktopEnhancements?.applyWallpaperPerHost?.() || applyDesktopWallpaper();
    window.CWDesktopEnhancements?.applyCompactMode?.();
    window.CWDesktopEnhancements?.startTransferPoll?.();
    const soundBtn = document.getElementById("linux-sound-toggle");
    if (soundBtn) soundBtn.setAttribute("aria-pressed", window.CWWebPrefs?.get?.("desktopSounds") ? "true" : "false");
    setSSHOverlay(!!state.ssh?.connected);
    pollPanelStats();
    if (!window.__cwDesktopStatsTimer) {
      window.__cwDesktopStatsTimer = setInterval(pollPanelStats, 20000);
    }
    requestAnimationFrame(() => showTourIfNeeded());
  }

  function onDesktopLeave() {
    window.CWDesktopEnhancements?.stopTransferPoll?.();
    if (window.__cwDesktopStatsTimer) {
      clearInterval(window.__cwDesktopStatsTimer);
      window.__cwDesktopStatsTimer = null;
    }
  }

  bindTour();
  bindNotifyMenu();
  bindShortcuts();

  document.addEventListener("cw-ssh-changed", (ev) => {
    if (state.screen !== "desktop") return;
    setSSHOverlay(!!ev.detail?.connected);
    if (!ev.detail?.connected) {
      pushNotif("Conexão SSH encerrada — reconecte para continuar.", "error");
    } else {
      pollPanelStats();
    }
  });

  function applyDesktopWallpaper() {
    const wall = window.CWWebPrefs?.get?.("desktopWallpaper") || "ubuntu";
    const desk = document.getElementById("linux-desktop");
    if (desk) desk.dataset.wallpaper = wall;
    document.querySelectorAll(".linux-wallpaper-btn").forEach((btn) => {
      btn.classList.toggle("is-active", btn.dataset.wall === wall);
    });
  }

  function bindWallpaperPicks() {
    /* Papel de fundo por host: desktop-enhancements.js */
  }

  function bindRestoreDialog() {
    document.getElementById("linux-restore-skip")?.addEventListener("click", () => {
      sessionStorage.setItem(SESSION_SKIP_KEY, "1");
      document.getElementById("linux-restore-dialog")?.close();
    });
    document.getElementById("linux-restore-ok")?.addEventListener("click", () => {
      document.getElementById("linux-restore-dialog")?.close();
      window.CWDesktop?.restoreSessionNow?.();
    });
  }

  bindWallpaperPicks();
  bindRestoreDialog();

  window.CWDesktopCore = {
    SESSION_KEY,
    SESSION_SKIP_KEY,
    TOUR_KEY,
    isDesktopOnboarded,
    markDesktopOnboarded,
    saveSession,
    loadSession,
    pushNotif,
    clearNotifs,
    pushRecentApp,
    getRecentApps,
    confirmDelete,
    pollPanelStats,
    applyDesktopWallpaper,
    onDesktopEnter,
    onDesktopLeave,
    renderBreadcrumbs(path, target, containerName, onNav) {
      const tr = (k, v) => (window.CWI18n && window.CWI18n.t(k, v)) || k;
      const wrap = document.createElement("nav");
      wrap.className = "linux-breadcrumbs";
      wrap.setAttribute("aria-label", tr("linux.files.breadcrumb.aria"));
      const parts = String(path || "/")
        .split("/")
        .filter((p, i, a) => i > 0 || a.length > 1);
      const prefix =
        target === "container"
          ? [{ label: `🐳 ${containerName || tr("linux.files.target.docker")}`, path: "/" }]
          : [{ label: tr("linux.files.breadcrumb.server"), path: "/" }];
      let acc = "";
      const crumbs = [...prefix];
      for (const p of parts) {
        acc += `/${p}`;
        crumbs.push({ label: p, path: acc });
      }
      crumbs.forEach((c, i) => {
        if (i > 0) {
          const sep = document.createElement("span");
          sep.className = "linux-bc-sep";
          sep.textContent = "›";
          wrap.appendChild(sep);
        }
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "linux-bc-btn";
        btn.textContent = c.label;
        btn.disabled = i === crumbs.length - 1;
        if (!btn.disabled) btn.addEventListener("click", () => onNav(c.path));
        wrap.appendChild(btn);
      });
      return wrap;
    },
    openContainerConsole(id, name) {
      const dlg = document.getElementById("docker-exec-dialog");
      const box = document.getElementById("docker-exec-terminal");
      const title = document.getElementById("docker-exec-title");
      if (!dlg || !box || !window.Terminal) {
        CWUI.toast("Console indisponível.", "error");
        return;
      }
      if (window.__cwDesktopExecSocket) {
        window.__cwDesktopExecSocket.close();
        window.__cwDesktopExecSocket = null;
      }
      if (window.__cwDesktopExecTerm) {
        window.__cwDesktopExecTerm.dispose();
        window.__cwDesktopExecTerm = null;
      }
      title.textContent = `Console — ${name || id}`;
      box.innerHTML = "";
      dlg.showModal();
      const term = new Terminal({
        theme: { background: "#1a1a2e", foreground: "#e8eef7", cursor: "#38bdf8" },
        fontSize: Number(window.CWWebPrefs?.get?.("terminalFontSize")) || 14,
      });
      const fit = new (window.FitAddon?.FitAddon || FitAddon.FitAddon)();
      term.loadAddon(fit);
      term.open(box);
      fit.fit();
      const p = location.protocol === "https:" ? "wss:" : "ws:";
      const ws = new WebSocket(`${p}//${location.host}/api/docker/exec/ws?id=${encodeURIComponent(id)}`);
      window.__cwDesktopExecTerm = term;
      window.__cwDesktopExecSocket = ws;
      ws.onopen = () => term.writeln("\r\n\x1b[32mConectado ao container.\x1b[0m\r\n");
      ws.onmessage = (ev) => term.write(ev.data);
      ws.onclose = () => term.writeln("\r\n\x1b[33mSessão terminada.\x1b[0m\r\n");
      term.onData((d) => {
        if (ws.readyState === WebSocket.OPEN) ws.send(d);
      });
      const onClose = () => {
        ws.close();
        term.dispose();
        window.__cwDesktopExecTerm = null;
        window.__cwDesktopExecSocket = null;
        dlg.removeEventListener("close", onClose);
      };
      dlg.addEventListener("close", onClose);
    },
    async openContainerLogs(id, name) {
      const dlg = document.getElementById("linux-docker-logs-dialog");
      const body = document.getElementById("linux-docker-logs-body");
      const title = document.getElementById("linux-docker-logs-title");
      if (!dlg || !body) return;
      title.textContent = `Logs — ${name || id}`;
      body.textContent = "Carregando…";
      dlg.showModal();
      try {
        const data = await api(`/api/docker/logs?id=${encodeURIComponent(id)}&tail=300`);
        body.textContent = data.logs || "(vazio)";
      } catch (e) {
        body.textContent = e.message || "Erro";
      }
    },
  };
})();
