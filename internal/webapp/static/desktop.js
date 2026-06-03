/** Ambiente Linux — gestor de janelas e shell do desktop. */
(function () {
  const $ = (sel, root) => (root || document).querySelector(sel);

  let overviewCache = null;
  let pollTimer = null;
  let clockTimer = null;
  let zTop = 10;
  let winSeq = 0;
  const windows = new Map();
  const WIN_ANIM_MS = 340;
  const APP_HUB_SCREEN = {
    files: "explorer",
    terminal: "terminal",
    disks: "disks",
    docker: "docker",
    automations: "automations",
    editor: "explorer",
  };

  function winMotionReduced() {
    return (
      window.matchMedia("(prefers-reduced-motion: reduce)").matches ||
      document.documentElement.classList.contains("reduce-motion")
    );
  }

  function escapeHtml(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function workspaceRect() {
    const el = $("#linux-windows");
    if (!el) return { w: 800, h: 500, left: 0, top: 0 };
    const r = el.getBoundingClientRect();
    return { w: r.width, h: r.height, left: r.left, top: r.top };
  }

  function defaultPlacement(w, h) {
    const area = workspaceRect();
    const n = windows.size;
    const margin = 24;
    const maxW = Math.max(280, area.w - margin * 2);
    const maxH = Math.max(200, area.h - margin * 2);
    const width = Math.min(w, maxW);
    const height = Math.min(h, maxH);
    const x = margin + (n % 4) * 28;
    const y = margin + (n % 4) * 24;
    return { x, y, w: width, h: height };
  }

  let altTabIdx = -1;
  let saveSessionTimer = null;

  function collectSession() {
    return [...windows.values()]
      .filter((w) => !w.closed && !w.minimized)
      .slice(0, 8)
      .map((w) => ({
        appId: w.appId,
        title: w.title,
        x: w.x,
        y: w.y,
        w: w.w,
        h: w.h,
        maximized: w.maximized,
        opts: w.ctx?.opts || {},
      }));
  }

  function scheduleSaveSession() {
    if (saveSessionTimer) clearTimeout(saveSessionTimer);
    saveSessionTimer = setTimeout(() => {
      window.CWDesktopCore?.saveSession?.(collectSession);
    }, 400);
  }

  function taskHintFor(win) {
    const st = win.ctx?.state;
    if (st?.path) {
      const p = st.target === "container" ? `🐳 ${st.containerName || ""}:${st.path}` : st.path;
      return `${win.title}\n${p}`;
    }
    return win.title;
  }

  function updateWorkspaceEmpty() {
    const empty = $("#linux-workspace-empty");
    if (!empty) return;
    const hasWin = [...windows.values()].some((w) => !w.closed);
    const onboarded = window.CWDesktopCore?.isDesktopOnboarded?.() ?? false;
    const hide = hasWin || onboarded;
    empty.hidden = hide;
    empty.setAttribute("aria-hidden", hide ? "true" : "false");
  }

  function renderTaskbar() {
    const bar = $("#linux-taskbar");
    if (!bar) return;
    const open = [...windows.values()].filter((w) => !w.closed);
    const emptyLbl = $("#linux-taskbar-empty");
    if (emptyLbl) emptyLbl.hidden = open.length > 0;
    bar.querySelectorAll(".linux-task-btn").forEach((n) => n.remove());
    for (const w of open) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "linux-task-btn" + (w.minimized ? "" : " is-active") + (w.focused && !w.minimized ? " is-focused" : "");
      const app = CWDesktopApps.get(w.appId);
      const glyph = app?.icon || "▣";
      btn.innerHTML = `<span class="linux-task-icon" aria-hidden="true">${glyph}</span><span class="linux-task-label">${escapeHtml(w.title)}</span>`;
      btn.title = taskHintFor(w);
      btn.dataset.winId = w.id;
      btn.addEventListener("click", () => {
        if (w.minimized || !w.focused) restoreWindow(w.id);
        else minimizeWindow(w.id);
      });
      bar.appendChild(btn);
    }
    updateWorkspaceEmpty();
  }

  function applyGeometry(win) {
    const el = win.el;
    el.style.transform = "";
    el.style.opacity = "";
    if (win.maximized) {
      el.style.left = "0";
      el.style.top = "0";
      el.style.width = "100%";
      el.style.height = "100%";
      el.classList.add("is-maximized");
    } else {
      el.style.left = `${win.x}px`;
      el.style.top = `${win.y}px`;
      el.style.width = `${win.w}px`;
      el.style.height = `${win.h}px`;
      el.classList.remove("is-maximized");
    }
    syncMaximizeButton(win);
  }

  function syncMaximizeButton(win) {
    const btn = win.el?.querySelector('[data-act="max"]');
    if (!btn) return;
    btn.textContent = win.maximized ? "❐" : "□";
    btn.title = win.maximized ? "Restaurar tamanho" : "Maximizar";
  }

  function getMinimizeTargetRect(win) {
    const btn = $(`#linux-taskbar [data-win-id="${win.id}"]`);
    if (btn) return btn.getBoundingClientRect();
    const bar = $("#linux-taskbar");
    if (bar && !bar.hidden) return bar.getBoundingClientRect();
    return { left: 20, top: window.innerHeight - 36, width: 72, height: 28, right: 92, bottom: window.innerHeight - 8 };
  }

  function runWinAnimation(el, keyframes, duration = WIN_ANIM_MS) {
    if (winMotionReduced() || !el.animate) return Promise.resolve();
    return el
      .animate(keyframes, {
        duration,
        easing: "cubic-bezier(0.32, 0.72, 0, 1)",
        fill: "forwards",
      })
      .finished.catch(() => {});
  }

  async function animateMinimize(win) {
    const el = win.el;
    if (winMotionReduced()) return;
    const rect = el.getBoundingClientRect();
    const target = getMinimizeTargetRect(win);
    const tcx = target.left + target.width / 2;
    const tcy = target.top + target.height / 2;
    const cx = rect.left + rect.width / 2;
    const cy = rect.top + rect.height / 2;
    const scale = Math.min(0.12, Math.max(0.06, target.width / Math.max(rect.width, 1)));
    el.classList.add("is-minimizing");
    el.style.transformOrigin = "center center";
    await runWinAnimation(el, [
      { transform: "translate(0, 0) scale(1)", opacity: 1, filter: "blur(0)" },
      {
        transform: `translate(${tcx - cx}px, ${tcy - cy}px) scale(${scale})`,
        opacity: 0,
        filter: "blur(4px)",
      },
    ]);
    el.classList.remove("is-minimizing");
    el.style.transform = "";
    el.style.opacity = "";
    el.style.filter = "";
  }

  async function animateRestore(win) {
    const el = win.el;
    if (winMotionReduced()) return;
    applyGeometry(win);
    const rect = el.getBoundingClientRect();
    const target = getMinimizeTargetRect(win);
    const tcx = target.left + target.width / 2;
    const tcy = target.top + target.height / 2;
    const cx = rect.left + rect.width / 2;
    const cy = rect.top + rect.height / 2;
    const scale = Math.min(0.12, Math.max(0.06, target.width / Math.max(rect.width, 1)));
    el.classList.add("is-restoring");
    el.style.transformOrigin = "center center";
    el.style.transform = `translate(${tcx - cx}px, ${tcy - cy}px) scale(${scale})`;
    el.style.opacity = "0";
    el.style.filter = "blur(4px)";
    await runWinAnimation(el, [
      { transform: `translate(${tcx - cx}px, ${tcy - cy}px) scale(${scale})`, opacity: 0, filter: "blur(4px)" },
      { transform: "translate(0, 0) scale(1)", opacity: 1, filter: "blur(0)" },
    ], 360);
    el.classList.remove("is-restoring");
    el.style.transform = "";
    el.style.opacity = "";
    el.style.filter = "";
  }

  function focusWindow(id) {
    const win = windows.get(id);
    if (!win || win.closed) return;
    const wasFocused = win.focused && !win.minimized;
    zTop += 1;
    win.z = zTop;
    win.el.style.zIndex = String(zTop);
    win.focused = true;
    for (const [wid, w] of windows) {
      if (wid !== id) w.focused = false;
    }
    if (!wasFocused) win.ctx.onFocus?.();
    if (win.appId === "terminal") {
      requestAnimationFrame(() => win.ctx.state?.fitActive?.());
    }
    renderTaskbar();
    scheduleSaveSession();
  }

  async function minimizeWindow(id) {
    const win = windows.get(id);
    if (!win || win.minimized || win._animating) return;
    win._animating = true;
    if (win.maximized) {
      win.maximized = false;
      if (win.restoreGeom) Object.assign(win, win.restoreGeom);
      applyGeometry(win);
      await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
    }
    renderTaskbar();
    await animateMinimize(win);
    win.minimized = true;
    win.el.classList.add("is-minimized");
    win.focused = false;
    win._animating = false;
    renderTaskbar();
  }

  async function restoreWindow(id) {
    const win = windows.get(id);
    if (!win || win._animating) return;
    if (!win.minimized) {
      focusWindow(id);
      return;
    }
    win._animating = true;
    win.minimized = false;
    win.el.classList.remove("is-minimized");
    renderTaskbar();
    zTop += 1;
    win.z = zTop;
    win.el.style.zIndex = String(zTop);
    await animateRestore(win);
    win._animating = false;
    focusWindow(id);
  }

  function toggleMaximize(id) {
    const win = windows.get(id);
    if (!win || win._animating || win.minimized) return;
    win.el.classList.add("is-snapping");
    if (!win.maximized) {
      win.restoreGeom = { x: win.x, y: win.y, w: win.w, h: win.h };
      win.maximized = true;
    } else {
      win.maximized = false;
      if (win.restoreGeom) Object.assign(win, win.restoreGeom);
    }
    applyGeometry(win);
    focusWindow(id);
    if (win.appId === "terminal") {
      requestAnimationFrame(() => win.ctx.state?.fitActive?.());
    }
    setTimeout(() => {
      win.el.classList.remove("is-snapping");
      scheduleSaveSession();
    }, WIN_ANIM_MS);
  }

  function openContainerTerminal(containerId, containerName) {
    return openApp("terminal", {
      containerId,
      containerName: containerName || "container",
      title: `Terminal · ${containerName || "Docker"}`,
    });
  }

  async function closeWindow(id) {
    const win = windows.get(id);
    if (!win || win._animating) return;
    win._animating = true;
    const el = win.el;
    if (!winMotionReduced() && el.animate) {
      el.classList.add("is-closing");
      await runWinAnimation(
        el,
        [
          { transform: "scale(1)", opacity: 1 },
          { transform: "scale(0.88)", opacity: 0, filter: "blur(6px)" },
        ],
        260
      );
    }
    const app = CWDesktopApps.get(win.appId);
    app?.unmount?.(win.ctx);
    win._cleanupDrag?.();
    el.remove();
    win.closed = true;
    windows.delete(id);
    win._animating = false;
    renderTaskbar();
    scheduleSaveSession();
  }

  function snapWindow(win, edge) {
    const area = workspaceRect();
    win.maximized = false;
    win.el.classList.remove("is-maximized");
    if (edge === "max") {
      win.maximized = true;
      applyGeometry(win);
      return;
    }
    const halfW = Math.floor(area.w / 2);
    if (edge === "left") {
      win.x = 0;
      win.y = 0;
      win.w = halfW;
      win.h = area.h;
    } else if (edge === "right") {
      win.x = halfW;
      win.y = 0;
      win.w = area.w - halfW;
      win.h = area.h;
    }
    applyGeometry(win);
    scheduleSaveSession();
  }

  function cycleWindows(backward) {
    const open = [...windows.values()].filter((w) => !w.closed && !w.minimized);
    if (!open.length) return;
    const focused = open.findIndex((w) => w.focused);
    altTabIdx = backward
      ? (focused <= 0 ? open.length - 1 : focused - 1)
      : (focused < 0 || focused >= open.length - 1 ? 0 : focused + 1);
    const pick = open[altTabIdx];
    showAltTab(open, pick);
    focusWindow(pick.id);
  }

  function showAltTab(open, current) {
    const box = $("#linux-alt-tab");
    if (!box || !open.length) return;
    box.hidden = false;
    box.setAttribute("aria-hidden", "false");
    box.innerHTML = open
      .map(
        (w) =>
          `<button type="button" class="linux-alt-item${w.id === current.id ? " is-current" : ""}" data-id="${w.id}">${escapeHtml(w.title)}</button>`
      )
      .join("");
    box.querySelectorAll(".linux-alt-item").forEach((btn) => {
      btn.addEventListener("click", () => {
        focusWindow(btn.dataset.id);
        hideAltTab();
      });
    });
    clearTimeout(box._hideT);
    box._hideT = setTimeout(hideAltTab, 900);
  }

  function hideAltTab() {
    const box = $("#linux-alt-tab");
    if (!box) return;
    box.hidden = true;
    box.setAttribute("aria-hidden", "true");
    box.innerHTML = "";
  }

  function closeFocusedWindow() {
    for (const w of windows.values()) {
      if (w.focused && !w.closed) return void closeWindow(w.id);
    }
  }

  function minimizeAll() {
    for (const w of windows.values()) {
      if (!w.closed && !w.minimized) void minimizeWindow(w.id);
    }
  }

  function bindDragResize(win) {
    const head = win.el.querySelector(".linux-win-head");
    const resize = win.el.querySelector(".linux-win-resize");
    let drag = null;
    head?.addEventListener("mousedown", (ev) => {
      if (ev.target.closest("button")) return;
      if (ev.detail === 2) {
        if (win.maximized) {
          win.maximized = false;
          if (win.restoreGeom) Object.assign(win, win.restoreGeom);
          applyGeometry(win);
          scheduleSaveSession();
        } else {
          toggleMaximize(win.id);
        }
        return;
      }
      if (win.maximized) return;
      focusWindow(win.id);
      const rect = win.el.getBoundingClientRect();
      drag = { dx: ev.clientX - rect.left, dy: ev.clientY - rect.top, startX: ev.clientX, startY: ev.clientY };
      ev.preventDefault();
    });
    resize?.addEventListener("mousedown", (ev) => {
      if (win.maximized) return;
      focusWindow(win.id);
      drag = { resize: true, startX: ev.clientX, startY: ev.clientY, w: win.w, h: win.h };
      ev.preventDefault();
    });
    const snapEl = $("#linux-snap-preview");
    const onMove = (ev) => {
      if (!drag) return;
      win.el.classList.add("is-dragging");
      const area = workspaceRect();
      if (!drag.resize && snapEl) {
        const relY = ev.clientY - area.top;
        const relX = ev.clientX - area.left;
        let zone = "";
        if (relY < 12) zone = "max";
        else if (relX < 16) zone = "left";
        else if (relX > area.w - 16) zone = "right";
        if (zone) {
          snapEl.hidden = false;
          snapEl.setAttribute("aria-hidden", "false");
          snapEl.className = `linux-snap-preview is-${zone}`;
        } else {
          snapEl.hidden = true;
          snapEl.setAttribute("aria-hidden", "true");
          snapEl.className = "linux-snap-preview";
        }
      }
      if (drag.resize) {
        win.w = Math.max(280, Math.min(area.w - win.x, drag.w + (ev.clientX - drag.startX)));
        win.h = Math.max(160, Math.min(area.h - win.y, drag.h + (ev.clientY - drag.startY)));
        applyGeometry(win);
        if (win.appId === "terminal") win.ctx.state?.fitActive?.();
      } else {
        win.x = Math.max(0, Math.min(area.w - 120, ev.clientX - area.left - drag.dx));
        win.y = Math.max(0, Math.min(area.h - 40, ev.clientY - area.top - drag.dy));
        applyGeometry(win);
      }
    };
    const onUp = (ev) => {
      if (snapEl) {
        snapEl.hidden = true;
        snapEl.setAttribute("aria-hidden", "true");
      }
      if (drag && !drag.resize && ev) {
        const area = workspaceRect();
        const relY = ev.clientY - area.top;
        const relX = ev.clientX - area.left;
        if (relY < 12) snapWindow(win, "max");
        else if (relX < 16) snapWindow(win, "left");
        else if (relX > area.w - 16) snapWindow(win, "right");
        else scheduleSaveSession();
      } else if (drag?.resize) scheduleSaveSession();
      drag = null;
      win.el.classList.remove("is-dragging");
    };
    document.addEventListener("mousemove", onMove);
    document.addEventListener("mouseup", onUp);
    win._cleanupDrag = () => {
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
    };
  }

  function canOpenDesktopApp(app) {
    if (!app?.screen || typeof canScreen !== "function") return true;
    if (canScreen(app.screen)) return true;
    if (canScreen("desktop")) {
      const allowedViaDesktop = ["explorer", "terminal", "docker", "disks", "services", "automations", "desktop"];
      if (allowedViaDesktop.includes(app.screen)) return true;
    }
    return false;
  }

  function openApp(appId, opts = {}) {
    const app = CWDesktopApps.get(appId);
    if (!app) return null;
    if (!canOpenDesktopApp(app)) {
      CWUI.toast("Sem permissão para esta aplicação.", "error");
      return null;
    }
    if (!state.ssh.connected && appId !== "help") {
      CWUI.toast("Conecte-se ao SSH primeiro.", "error");
      return null;
    }
    closeStartMenu();
    window.CWDesktopCore?.pushRecentApp?.(appId);
    if (app.singleton) {
      for (const [id, w] of windows) {
        if (w.appId === appId && !w.closed) {
          restoreWindow(id);
          window.CWDesktopCore?.markDesktopOnboarded?.();
          if (opts.path && appId === "editor") {
            closeWindow(id);
            break;
          }
          return id;
        }
      }
    }
    const id = `win-${++winSeq}`;
    const place = defaultPlacement(app.w || 480, app.h || 360);
    const host = $("#linux-windows");
    if (!host) return null;
    const title = opts.title || app.label;
    const el = document.createElement("div");
    el.className = "linux-win";
    el.dataset.winId = id;
    el.innerHTML = `
      <header class="linux-win-head">
        <span class="linux-win-icon" aria-hidden="true">${app.icon || "▣"}</span>
        <span class="linux-win-title">${escapeHtml(title)}</span>
        <div class="linux-win-controls">
          <button type="button" class="linux-win-btn linux-win-btn-hub" data-act="hub" title="Abrir no hub técnico">⌂</button>
          <button type="button" class="linux-win-btn" data-act="min" title="Minimizar">−</button>
          <button type="button" class="linux-win-btn" data-act="max" title="Maximizar">□</button>
          <button type="button" class="linux-win-btn linux-win-btn-close" data-act="close" title="Fechar">×</button>
        </div>
      </header>
      <div class="linux-win-body"></div>
      <span class="linux-win-resize" aria-hidden="true"></span>`;
    host.appendChild(el);
    const body = el.querySelector(".linux-win-body");
    const ctx = { winId: id, opts, minimized: false, onFocus: null };
    const win = {
      id,
      appId,
      title,
      el,
      x: place.x,
      y: place.y,
      w: place.w,
      h: place.h,
      maximized: false,
      minimized: false,
      focused: false,
      closed: false,
      ctx,
    };
    windows.set(id, win);
    applyGeometry(win);
    bindDragResize(win);
    el.addEventListener("mousedown", () => focusWindow(id));
    el.querySelector('[data-act="close"]')?.addEventListener("click", () => void closeWindow(id));
    el.querySelector('[data-act="min"]')?.addEventListener("click", () => void minimizeWindow(id));
    el.querySelector('[data-act="max"]')?.addEventListener("click", () => toggleMaximize(id));
    el.querySelector('[data-act="hub"]')?.addEventListener("click", () => openAppInHub(appId));
    syncMaximizeButton(win);
    app.mount(body, ctx);
    if (!winMotionReduced()) {
      el.classList.add("is-opening");
      setTimeout(() => el.classList.remove("is-opening"), 450);
    }
    focusWindow(id);
    window.CWDesktopCore?.markDesktopOnboarded?.();
    return id;
  }

  function openEditor(path, name, fromWinId, extra = {}) {
    if (fromWinId) {
      const w = windows.get(fromWinId);
      if (w?.ctx?.state?.dirty) {
        if (!confirm("O editor tem alterações não guardadas. Continuar?")) return;
      }
    }
    return openApp("editor", { path, name, title: name || "Editor", ...extra });
  }

  function openContainerFiles(containerId, containerName) {
    const label = (containerName || "container").replace(/^\//, "");
    return openApp("files", {
      target: "container",
      containerId,
      containerName: label,
      path: "/",
      title: `Arquivos · ${label}`,
    });
  }

  function closeAllWindows() {
    for (const id of [...windows.keys()]) {
      void closeWindow(id);
    }
  }

  function makeLauncherButton(app, extraClass) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = `linux-icon-btn ${extraClass || ""}`.trim();
    btn.setAttribute("role", "listitem");
    btn.title = app.desc || app.label;
    btn.innerHTML = `<span class="linux-icon-glyph" aria-hidden="true">${app.icon}</span><span class="linux-icon-label">${escapeHtml(app.label)}</span>`;
    btn.addEventListener("click", () => openApp(app.id));
    return btn;
  }

  function openAppInHub(appId) {
    const screen = APP_HUB_SCREEN[appId];
    if (!screen) {
      CWUI?.toast?.("Esta aplicação só existe no ambiente simplificado.", "info");
      return;
    }
    showScreen(screen);
    if (screen === "hub") renderHub();
  }

  function renderStartRecent(apps) {
    const wrap = $("#linux-start-recent");
    const row = $("#linux-start-recent-row");
    if (!wrap || !row) return;
    const ids = window.CWDesktopCore?.getRecentApps?.() || [];
    const tiles = ids.map((id) => apps.find((a) => a.id === id)).filter(Boolean);
    if (!tiles.length) {
      wrap.hidden = true;
      return;
    }
    wrap.hidden = false;
    row.innerHTML = "";
    for (const app of tiles) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "linux-start-recent-btn";
      btn.textContent = app.label;
      btn.title = app.desc || app.label;
      btn.addEventListener("click", () => openApp(app.id));
      row.appendChild(btn);
    }
  }

  function filterStartTiles(query) {
    const q = String(query || "")
      .toLowerCase()
      .trim();
    $("#linux-start-grid")
      ?.querySelectorAll(".linux-start-tile")
      .forEach((tile) => {
        const hay = (tile.dataset.search || "").toLowerCase();
        tile.hidden = q ? !hay.includes(q) : false;
      });
  }

  function renderLaunchers() {
    const desk = $("#linux-icons");
    const start = $("#linux-start-grid");
    if (!desk || !start) return;
    desk.innerHTML = "";
    start.innerHTML = "";
    const apps = CWDesktopApps.visible();
    for (const app of apps) {
      desk.appendChild(makeLauncherButton(app));
      const tile = document.createElement("button");
      tile.type = "button";
      tile.className = "linux-start-tile";
      tile.dataset.search = `${app.label} ${app.desc || ""} ${app.id}`;
      tile.innerHTML = `<span class="linux-start-tile-icon">${app.icon}</span><span class="linux-start-tile-text"><strong>${escapeHtml(app.label)}</strong><span class="muted">${escapeHtml(app.desc || "")}</span></span>`;
      tile.addEventListener("click", () => openApp(app.id));
      start.appendChild(tile);
    }
    renderStartRecent(apps);
    filterStartTiles($("#linux-start-search")?.value);
  }

  async function refreshDesktopLatency() {
    const pill = $("#linux-panel-ssh");
    if (!pill || !state.ssh?.connected) return;
    const t0 = performance.now();
    try {
      await api(`/api/remote/list?path=${encodeURIComponent(state.remotePath || "/")}`);
      const ms = Math.round(performance.now() - t0);
      pill.title = `SSH ligado · ${ms} ms`;
    } catch {
      pill.title = "SSH ligado";
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
      ssh.textContent = on ? "Conectado" : "Desconectado";
      ssh.classList.toggle("is-online", on);
      ssh.classList.toggle("is-offline", !on);
      if (on) refreshDesktopLatency();
      else ssh.title = "SSH desligado";
    }
    if (!state.ssh.connected) {
      window.CWDesktopCore?.renderPanelStats?.(null, null, null);
    }
  }

  function tickClock() {
    const el = $("#linux-panel-clock");
    if (!el) return;
    const now = new Date();
    el.textContent = now.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    el.dateTime = now.toISOString();
  }

  const START_MENU_ANIM_MS = 240;

  function positionStartMenu() {
    const btn = $("#linux-btn-menu");
    const menu = $("#linux-start-menu");
    const inner = menu?.querySelector(".linux-start-menu-inner");
    if (!btn || !menu || !inner) return;
    const gap = 8;
    const pad = 12;
    const r = btn.getBoundingClientRect();
    const w = inner.offsetWidth || 448;
    const h = inner.offsetHeight || 420;
    let top = r.bottom + gap;
    let left = r.left;
    if (left + w > window.innerWidth - pad) left = window.innerWidth - w - pad;
    if (left < pad) left = pad;
    if (top + h > window.innerHeight - pad) top = Math.max(pad, r.top - h - gap);
    menu.style.top = `${top}px`;
    menu.style.left = `${left}px`;
    const originX = Math.min(Math.max(16, r.left + r.width / 2 - left), w - 16);
    menu.style.setProperty("--start-origin-x", `${originX}px`);
  }

  function closeStartMenu() {
    const menu = $("#linux-start-menu");
    const backdrop = $("#linux-start-backdrop");
    const btn = $("#linux-btn-menu");
    if (!menu || menu.hidden) return;
    menu.classList.remove("is-open");
    backdrop?.classList.remove("is-open");
    btn?.classList.remove("is-active");
    btn?.setAttribute("aria-expanded", "false");
    const finish = () => {
      menu.hidden = true;
      if (backdrop) backdrop.hidden = true;
    };
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      finish();
      return;
    }
    setTimeout(finish, START_MENU_ANIM_MS);
  }

  function openStartMenu() {
    const menu = $("#linux-start-menu");
    const backdrop = $("#linux-start-backdrop");
    const btn = $("#linux-btn-menu");
    if (!menu || !menu.hidden) return;
    positionStartMenu();
    menu.hidden = false;
    if (backdrop) backdrop.hidden = false;
    requestAnimationFrame(() => {
      positionStartMenu();
      menu.classList.add("is-open");
      backdrop?.classList.add("is-open");
      btn?.classList.add("is-active");
      btn?.setAttribute("aria-expanded", "true");
    });
  }

  function toggleStartMenu() {
    const menu = $("#linux-start-menu");
    if (!menu) return;
    if (menu.hidden) openStartMenu();
    else closeStartMenu();
  }

  window.addEventListener("resize", () => {
    if (!$("#linux-start-menu")?.classList.contains("is-open")) return;
    positionStartMenu();
  });

  async function refreshOverview(silent) {
    if (!state.ssh.connected) return;
    try {
      overviewCache = await api("/api/desktop/overview", { noAuthRedirect: true });
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

  function restoreSessionNow() {
    const sess = window.CWDesktopCore?.loadSession?.();
    if (!sess?.windows?.length) return;
    applySessionRows(sess.windows);
  }

  function applySessionRows(rows) {
    for (const row of rows) {
      const wid = openApp(row.appId, row.opts || {});
      if (!wid) continue;
      const win = windows.get(wid);
      if (!win) continue;
      if (row.title) {
        win.title = row.title;
        const t = win.el.querySelector(".linux-win-title");
        if (t) t.textContent = row.title;
      }
      if (row.maximized) {
        win.restoreGeom = { x: row.x, y: row.y, w: row.w, h: row.h };
        win.maximized = true;
      } else {
        win.x = row.x ?? win.x;
        win.y = row.y ?? win.y;
        win.w = row.w ?? win.w;
        win.h = row.h ?? win.h;
      }
      applyGeometry(win);
    }
    updateWorkspaceEmpty();
  }

  async function maybeRestoreSession() {
    if (windows.size > 0) return;
    const sess = window.CWDesktopCore?.loadSession?.();
    if (!sess?.windows?.length || !state.ssh.connected) return;
    const skipKey = window.CWDesktopCore?.SESSION_SKIP_KEY || "cw-desktop-session-skip";
    if (sessionStorage.getItem(skipKey) === "1") return;
    const dlg = document.getElementById("linux-restore-dialog");
    if (dlg?.showModal) {
      const body = document.getElementById("linux-restore-body");
      if (body) body.textContent = `Restaurar ${sess.windows.length} janela(s) da última visita neste ambiente?`;
      dlg.showModal();
      return;
    }
    if (!confirm(`Restaurar ${sess.windows.length} janela(s) da sessão anterior?`)) {
      sessionStorage.setItem(skipKey, "1");
      return;
    }
    applySessionRows(sess.windows);
  }

  function onEnter() {
    document.getElementById("app")?.classList.add("app-desktop-mode");
    hideAltTab();
    document.getElementById("linux-window-layer")?.remove();
    renderLaunchers();
    updatePanelChrome();
    startPoll();
    window.CWDesktopEnhancements?.applyWallpaperPerHost?.() || window.CWDesktopCore?.applyDesktopWallpaper?.();
    window.CWDesktopCore?.onDesktopEnter?.();
    updateWorkspaceEmpty();
    if (state.ssh.connected) maybeRestoreSession();
  }

  function onLeave() {
    window.CWDesktopCore?.saveSession?.(collectSession);
    window.CWDesktopCore?.onDesktopLeave?.();
    document.getElementById("app")?.classList.remove("app-desktop-mode");
    closeStartMenu();
    closeAllWindows();
    stopPoll();
  }

  function onSSHChanged(connected) {
    updatePanelChrome();
    const desk = document.getElementById("linux-desktop");
    desk?.classList.toggle("is-ssh-down", !connected);
    document.getElementById("linux-ssh-banner")?.toggleAttribute("hidden", connected);
    if (!connected) {
      for (const w of windows.values()) {
        w.el?.classList.add("is-ssh-blocked");
      }
    } else {
      for (const w of windows.values()) {
        w.el?.classList.remove("is-ssh-blocked");
      }
    }
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

  $("#linux-start-search")?.addEventListener("input", (ev) => filterStartTiles(ev.target.value));
  $("#linux-workspace-tour")?.addEventListener("click", () => {
    document.getElementById("linux-tour-dialog")?.showModal?.();
  });

  document.addEventListener("cw-desktop-onboarded", updateWorkspaceEmpty);

  document.addEventListener("cw-pref-change", (ev) => {
    if (ev.detail?.name === "desktopWallpaper")
      window.CWDesktopEnhancements?.applyWallpaperPerHost?.() || window.CWDesktopCore?.applyDesktopWallpaper?.();
  });

  document.addEventListener("click", (ev) => {
    const menu = $("#linux-start-menu");
    if (!menu || menu.hidden) return;
    if (menu.contains(ev.target) || $("#linux-btn-menu")?.contains(ev.target)) return;
    closeStartMenu();
  });

  $("#linux-start-backdrop")?.addEventListener("click", closeStartMenu);

  document.addEventListener("keydown", (ev) => {
    if (state.screen !== "desktop") return;
    if (ev.key === "Escape") {
      hideAltTab();
      if (!$("#linux-start-menu")?.hidden) {
        closeStartMenu();
        ev.preventDefault();
      }
    }
  });

  document.getElementById("linux-docker-logs-close")?.addEventListener("click", () => {
    document.getElementById("linux-docker-logs-dialog")?.close();
  });

  window.CWDesktop = {
    onEnter,
    onLeave,
    onSSHChanged,
    openApp,
    openEditor,
    openContainerFiles,
    openContainerTerminal,
    closeFocusedWindow,
    closeAllWindows,
    closeStartMenu,
    minimizeAll,
    cycleWindows,
    restoreSessionNow,
    openAppInHub,
    refreshOverview,
    getWindow: (id) => windows.get(id),
    getOverviewCache: () => overviewCache,
    setOverviewCache: (ov) => {
      overviewCache = ov;
    },
  };
})();
