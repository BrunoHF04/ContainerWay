/** Utilitários visuais partilhados — toasts, confirmações, skeleton, etc. */
const CWUI = (() => {
  const $ = (sel, root = document) => root.querySelector(sel);

  function toast(message, type = "info", ms) {
    if (ms == null && window.CWWebPrefs?.toastDurationMs) {
      ms = window.CWWebPrefs.toastDurationMs();
    }
    if (ms == null) ms = 4200;
    const box = $("#toast-stack");
    if (!box) return;
    const el = document.createElement("div");
    el.className = `toast toast-${type}`;
    el.innerHTML = `<span class="toast-msg">${escapeHtml(String(message))}</span>`;
    box.appendChild(el);
    requestAnimationFrame(() => el.classList.add("show"));
    const close = () => {
      el.classList.remove("show");
      setTimeout(() => el.remove(), 350);
    };
    const t = setTimeout(close, ms);
    el.addEventListener("click", () => { clearTimeout(t); close(); });
  }

  function escapeHtml(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function nameInputDialog({ title = "Nome", label = "Nome", value = "", ok = "OK" } = {}) {
    return new Promise((resolve) => {
      const dlg = $("#name-input-dialog");
      const field = $("#name-input-field");
      const errEl = $("#name-input-error");
      if (!dlg || !field) {
        resolve(window.prompt(label, value)?.trim() || null);
        return;
      }
      $("#name-input-title").textContent = title;
      $("#name-input-label").textContent = label;
      field.value = value || "";
      if (errEl) {
        errEl.textContent = "";
        errEl.classList.add("hidden");
      }
      const done = (v) => {
        dlg.close();
        resolve(v);
      };
      $("#name-input-ok").onclick = () => {
        const v = field.value.trim();
        if (!v) {
          if (errEl) {
            errEl.textContent = "Indique um nome válido.";
            errEl.classList.remove("hidden");
          }
          return;
        }
        done(v);
      };
      $("#name-input-cancel").onclick = () => done(null);
      dlg.onclose = () => resolve(null);
      dlg.showModal();
      setTimeout(() => {
        field.focus();
        field.select();
      }, 50);
      field.onkeydown = (e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          $("#name-input-ok")?.click();
        }
      };
    });
  }

  function confirmDialog(message, { title = "Confirmar", ok = "Confirmar", cancel = "Cancelar", danger = false } = {}) {
    return new Promise((resolve) => {
      const dlg = $("#confirm-dialog");
      if (!dlg) { resolve(window.confirm(message)); return; }
      $("#confirm-title").textContent = title;
      $("#confirm-body").textContent = message;
      const okBtn = $("#confirm-ok");
      const cancelBtn = $("#confirm-cancel");
      okBtn.textContent = ok;
      okBtn.classList.toggle("btn-danger", danger);
      let settled = false;
      const done = (v) => {
        if (settled) return;
        settled = true;
        dlg.close();
        resolve(v);
      };
      okBtn.onclick = () => done(true);
      cancelBtn.onclick = () => done(false);
      dlg.onclose = () => {
        if (!settled) done(false);
      };
      dlg.showModal();
    });
  }

  function skeletonList(container, rows = 6) {
    if (!container) return;
    container.innerHTML = "";
    container.classList.add("skeleton-host");
    for (let i = 0; i < rows; i++) {
      const row = document.createElement("div");
      row.className = "skeleton-row";
      row.innerHTML = '<span class="sk sk-icon"></span><span class="sk sk-line"></span><span class="sk sk-meta"></span>';
      container.appendChild(row);
    }
  }

  function emptyState(container, { icon = "📭", title, desc, actionLabel, onAction }) {
    if (!container) return;
    container.classList.remove("skeleton-host");
    container.innerHTML = `
      <div class="empty-state">
        <span class="empty-icon">${icon}</span>
        <strong>${escapeHtml(title)}</strong>
        <p>${escapeHtml(desc || "")}</p>
        ${actionLabel ? `<button type="button" class="btn btn-primary btn-sm empty-action">${escapeHtml(actionLabel)}</button>` : ""}
      </div>`;
    const btn = container.querySelector(".empty-action");
    if (btn && onAction) btn.addEventListener("click", onAction);
  }

  const FILE_ICONS = [
    { re: /\.(png|jpe?g|gif|webp|svg|ico)$/i, icon: "🖼️" },
    { re: /\.(mp4|mkv|avi|mov|webm)$/i, icon: "🎬" },
    { re: /\.(mp3|wav|flac|ogg)$/i, icon: "🎵" },
    { re: /\.(zip|tar|gz|bz2|7z|rar)$/i, icon: "📦" },
    { re: /\.(go|js|ts|py|java|c|cpp|h|rs|php|rb)$/i, icon: "💻" },
    { re: /\.(json|ya?ml|toml|xml)$/i, icon: "📋" },
    { re: /\.(md|txt|log)$/i, icon: "📝" },
    { re: /\.(sql|db)$/i, icon: "🗄️" },
    { re: /\.(sh|bash|ps1|bat)$/i, icon: "⚡" },
    { re: /\.(pdf)$/i, icon: "📕" },
    { re: /\.(deb|rpm)$/i, icon: "📀" },
  ];

  function fileIcon(name, isDir) {
    if (isDir) return "📁";
    if (name === "..") return "⬆️";
    for (const { re, icon } of FILE_ICONS) {
      if (re.test(name)) return icon;
    }
    return "📄";
  }

  function hostAccent(host) {
    const h = String(host || "").trim();
    if (!h) {
      document.documentElement.style.removeProperty("--host-accent");
      return;
    }
    let hash = 0;
    for (let i = 0; i < h.length; i++) hash = (hash * 31 + h.charCodeAt(i)) >>> 0;
    const hue = hash % 360;
    document.documentElement.style.setProperty("--host-accent", `hsl(${hue} 68% 58%)`);
    document.documentElement.style.setProperty("--host-accent-dim", `hsla(${hue} 68% 58% / 0.18)`);
  }

  function showSplash(label = "Conectando SSH…") {
    const el = $("#splash-overlay");
    if (!el) return;
    el.querySelector(".splash-label").textContent = label;
    el.classList.remove("hidden");
    el.setAttribute("aria-busy", "true");
  }

  function hideSplash() {
    const el = $("#splash-overlay");
    if (!el) return;
    el.classList.add("hidden");
    el.removeAttribute("aria-busy");
  }

  const COMMANDS = [];

  function registerCommands(list) {
    COMMANDS.length = 0;
    COMMANDS.push(...list);
  }

  function openCommandPalette(filter = "") {
    const dlg = $("#cmd-palette");
    const input = $("#cmd-input");
    if (!dlg || !input) return;
    input.value = filter;
    renderCommands(filter);
    dlg.showModal();
    setTimeout(() => input.focus(), 50);
  }

  function renderCommands(q) {
    const ul = $("#cmd-results");
    if (!ul) return;
    const query = (q || "").toLowerCase().trim();
    const items = COMMANDS.filter((c) => {
      if (!query) return true;
      const hay = `${c.title} ${c.sub || ""} ${c.kw || ""}`.toLowerCase();
      return hay.includes(query);
    }).slice(0, 12);
    ul.innerHTML = "";
    if (!items.length) {
      ul.innerHTML = '<li class="cmd-empty">Nenhum comando</li>';
      return;
    }
    items.forEach((c, i) => {
      const li = document.createElement("li");
      li.className = "cmd-item" + (i === 0 ? " active" : "");
      li.dataset.id = c.id;
      li.innerHTML = `<span class="cmd-title">${escapeHtml(c.title)}</span><span class="cmd-sub">${escapeHtml(c.sub || "")}</span>`;
      li.addEventListener("click", () => runCommand(c));
      ul.appendChild(li);
    });
  }

  function runCommand(c) {
    $("#cmd-palette")?.close();
    c.run?.();
  }

  function bindCommandPalette() {
    const dlg = $("#cmd-palette");
    const input = $("#cmd-input");
    if (!dlg || !input) return;
    input.addEventListener("input", () => renderCommands(input.value));
    input.addEventListener("keydown", (e) => {
      const items = [...dlg.querySelectorAll(".cmd-item")];
      let idx = items.findIndex((el) => el.classList.contains("active"));
      if (e.key === "ArrowDown") {
        e.preventDefault();
        if (idx >= 0) items[idx].classList.remove("active");
        idx = Math.min(idx + 1, items.length - 1);
        if (items[idx]) items[idx].classList.add("active");
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        if (idx >= 0) items[idx].classList.remove("active");
        idx = Math.max(idx - 1, 0);
        if (items[idx]) items[idx].classList.add("active");
      } else if (e.key === "Enter") {
        e.preventDefault();
        const active = items[idx >= 0 ? idx : 0];
        const id = active?.dataset.id;
        const cmd = COMMANDS.find((c) => c.id === id);
        if (cmd) runCommand(cmd);
      }
    });
    document.addEventListener("keydown", (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        openCommandPalette();
      }
      if (e.key === "?" && !e.ctrlKey && !e.metaKey && e.target.tagName !== "INPUT" && e.target.tagName !== "TEXTAREA") {
        if (typeof window.openScreenHelp === "function") window.openScreenHelp();
      }
      if (e.key === "Escape") {
        $("#screen-help-dialog")?.close();
      }
    });
  }

  function bindRipple() {
    document.querySelectorAll(".topbar .btn, .topbar .theme-toggle, .topbar .brand-mini").forEach((btn) => {
      btn.classList.add("btn-ripple-host");
      btn.addEventListener("click", function ripple(ev) {
        const r = document.createElement("span");
        r.className = "btn-ripple";
        const rect = this.getBoundingClientRect();
        const size = Math.max(rect.width, rect.height);
        r.style.width = r.style.height = `${size}px`;
        r.style.left = `${ev.clientX - rect.left - size / 2}px`;
        r.style.top = `${ev.clientY - rect.top - size / 2}px`;
        this.appendChild(r);
        setTimeout(() => r.remove(), 600);
      });
    });
  }

  function renderBreadcrumbs(container, parts, onNavigate, prefix) {
    if (!container) return;
    container.innerHTML = "";
    const nav = document.createElement("nav");
    nav.className = "breadcrumbs";
    nav.setAttribute("aria-label", "Caminho");
    if (prefix) {
      const chip = document.createElement("span");
      chip.className = "bc-context";
      chip.textContent = prefix;
      chip.title = prefix;
      nav.appendChild(chip);
      const sep0 = document.createElement("span");
      sep0.className = "bc-sep";
      sep0.textContent = "›";
      nav.appendChild(sep0);
    }
    parts.forEach((p, i) => {
      if (i > 0) {
        const sep = document.createElement("span");
        sep.className = "bc-sep";
        sep.textContent = "›";
        nav.appendChild(sep);
      }
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "bc-part" + (i === parts.length - 1 ? " current" : "");
      btn.textContent = p.label;
      btn.title = p.path || p.label;
      if (i < parts.length - 1 && p.path) {
        btn.addEventListener("click", () => onNavigate(p.path));
      } else {
        btn.disabled = true;
      }
      nav.appendChild(btn);
    });
    container.appendChild(nav);
  }

  function pathToBreadcrumbs(path, isWindows) {
    if (!path) return [{ label: "Raiz", path: isWindows ? "" : "/" }];
    const sep = path.includes("\\") ? "\\" : "/";
    const norm = path.replace(/[/\\]+$/, "");
    const bits = norm.split(sep).filter(Boolean);
    const out = [];
    if (isWindows && /^[A-Za-z]:/.test(norm)) {
      const drive = bits.shift() + sep;
      out.push({ label: drive, path: drive });
      let acc = drive;
      for (const b of bits) {
        acc += b + sep;
        out.push({ label: b, path: acc });
      }
    } else {
      out.push({ label: "/", path: "/" });
      let acc = "/";
      for (const b of bits) {
        acc = acc.endsWith("/") ? acc + b : acc + "/" + b;
        out.push({ label: b, path: acc });
      }
    }
    return out;
  }

  function renderTransferProgress(active, recent) {
    const panel = $("#transfer-progress-panel");
    if (!panel) return;
    const inner = $("#transfer-progress-inner");
    if (!inner) return;
    inner.innerHTML = "";
    if (active && active.name) {
      const pct = active.total > 0 ? Math.min(100, Math.round((active.done / active.total) * 100)) : null;
      const bar = document.createElement("div");
      bar.className = "xfer-active";
      bar.innerHTML = `
        <div class="xfer-head"><strong>${escapeHtml(active.name)}</strong>
          <span class="muted">${pct != null ? pct + "%" : "em curso…"}</span></div>
        <div class="progress-track"><div class="progress-fill ${pct == null ? "indeterminate" : ""}" style="width:${pct != null ? pct : 40}%"></div></div>`;
      inner.appendChild(bar);
    }
    const running = (recent || []).filter((r) => r.status === "running");
    for (const r of running.slice(0, 3)) {
      if (active && r.name === active.name) continue;
      const pct = r.total > 0 ? Math.min(100, Math.round((r.done / r.total) * 100)) : null;
      const bar = document.createElement("div");
      bar.className = "xfer-active";
      bar.innerHTML = `
        <div class="xfer-head"><strong>${escapeHtml(r.name)}</strong></div>
        <div class="progress-track"><div class="progress-fill ${pct == null ? "indeterminate" : ""}" style="width:${pct != null ? pct : 30}%"></div></div>`;
      inner.appendChild(bar);
    }
    const show = inner.children.length > 0;
    panel.classList.toggle("hidden", !show);
  }

  const _floatMenus = new Set();

  function setMenuBackdrop(visible) {
    const backdrop = document.getElementById("explorer-menu-backdrop");
    if (!backdrop) return;
    backdrop.classList.toggle("hidden", !visible);
    backdrop.classList.toggle("is-visible", visible);
  }

  function closeAllFloatMenus(except) {
    for (const d of _floatMenus) {
      if (d !== except && d.open) d.removeAttribute("open");
    }
    setMenuBackdrop(false);
  }

  /** Menu flutuante: portal para body, um aberto de cada vez, sem sobrepor painéis. */
  function wireFloatingDropdown(detailsEl, panelSelector, align = "left", group = "explorer") {
    if (!detailsEl) return;
    const panel = detailsEl.querySelector(panelSelector);
    if (!panel) return;
    _floatMenus.add(detailsEl);
    detailsEl.dataset.floatGroup = group;

    const placeholder = document.createComment("cw-menu");
    let portaled = false;
    const anchor = () => detailsEl.querySelector("summary") || detailsEl;
    const menuPrefer = detailsEl.dataset.menuPrefer || "auto";

    const restorePanel = () => {
      panel.classList.remove("is-floating");
      panel.style.cssText = "";
      if (portaled && placeholder.parentNode) {
        placeholder.parentNode.insertBefore(panel, placeholder);
        portaled = false;
      }
    };

    const place = () => {
      if (!detailsEl.open) {
        restorePanel();
        return;
      }
      if (!portaled) {
        detailsEl.insertBefore(placeholder, panel);
        document.body.appendChild(panel);
        portaled = true;
      }
      panel.classList.remove("menu-anchor-up");
      panel.classList.add("is-floating");
      panel.style.display = "flex";
      const ar = anchor().getBoundingClientRect();
      const pw = panel.offsetWidth || 180;
      const ph = panel.offsetHeight || 160;
      let top;
      let left;
      let openUp = false;
      const centerX = ar.left + ar.width / 2 - pw / 2;

      const explorerGrid = document.querySelector("#view-explorer .explorer");
      const gridTop = explorerGrid?.getBoundingClientRect().top ?? window.innerHeight;

      if (menuPrefer === "down") {
        top = ar.bottom + 8;
        left = centerX;
      } else if (menuPrefer === "up") {
        top = ar.top - ph - 8;
        left = centerX;
        openUp = true;
      } else {
        top = ar.bottom + 6;
        left = align === "right" ? ar.right - pw : ar.left;
        if (top + ph > gridTop - 4 && ar.top > ph + 12) {
          top = ar.top - ph - 6;
          left = centerX;
          openUp = true;
        }
      }

      if (openUp) panel.classList.add("menu-anchor-up");

      left = Math.max(8, Math.min(left, window.innerWidth - pw - 8));
      top = Math.max(8, Math.min(top, window.innerHeight - ph - 8));
      panel.style.top = `${top}px`;
      panel.style.left = `${left}px`;

      setMenuBackdrop(true);
    };

    detailsEl.addEventListener("toggle", () => {
      if (detailsEl.open) {
        document.querySelectorAll(`details[data-float-group="${group}"][open]`).forEach((d) => {
          if (d !== detailsEl) d.removeAttribute("open");
        });
        requestAnimationFrame(() => requestAnimationFrame(place));
      } else {
        restorePanel();
        if (![..._floatMenus].some((d) => d.open)) setMenuBackdrop(false);
      }
    });

    panel.addEventListener("click", (e) => e.stopPropagation());

    window.addEventListener("resize", () => { if (detailsEl.open) place(); });
    window.addEventListener("scroll", () => { if (detailsEl.open) place(); }, true);
  }

  document.addEventListener("pointerdown", (e) => {
    if (e.target.closest("details[data-float-group] > summary")) return;
    if (e.target.closest(".is-floating")) return;
    if (![..._floatMenus].some((d) => d.open)) return;
    closeAllFloatMenus();
  });

  document.getElementById("explorer-menu-backdrop")?.addEventListener("pointerdown", () => closeAllFloatMenus());

  const selectControllers = new WeakMap();
  const openSelectWraps = new Set();

  function closeSelectWrap(wrap) {
    if (!wrap) return;
    const select = wrap.querySelector("select.cw-select-native");
    const ctrl = select && selectControllers.get(select);
    if (ctrl?.close) ctrl.close();
    else {
      wrap.classList.remove("is-open");
      wrap.querySelector(".cw-select-list")?.classList.add("hidden");
      wrap.querySelector(".cw-select-trigger")?.setAttribute("aria-expanded", "false");
      openSelectWraps.delete(wrap);
    }
  }

  function closeAllSelects(except) {
    for (const wrap of [...openSelectWraps]) {
      if (wrap !== except) closeSelectWrap(wrap);
    }
  }

  function shouldEnhanceSelect(select) {
    if (!select || select.multiple || select.size > 1) return false;
    if (select.hasAttribute("data-select-native")) return false;
    return true;
  }

  function enhanceSelect(select) {
    if (!shouldEnhanceSelect(select)) return;
    if (select.closest(".cw-select")) {
      refreshSelect(select);
      return;
    }

    const wrap = document.createElement("div");
    wrap.className = "cw-select";
    if (select.classList.contains("panel-select")) wrap.classList.add("cw-select--compact");
    if (select.classList.contains("explorer-lang-select")) wrap.classList.add("cw-select--compact");
    if (select.classList.contains("docker-poll-select")) wrap.classList.add("cw-select--poll");
    if (select.disabled) wrap.classList.add("is-disabled");

    select.classList.add("cw-select-native");
    select.parentNode.insertBefore(wrap, select);
    wrap.appendChild(select);

    const trigger = document.createElement("button");
    trigger.type = "button";
    trigger.className = "cw-select-trigger";
    trigger.setAttribute("aria-haspopup", "listbox");

    const valueEl = document.createElement("span");
    valueEl.className = "cw-select-value";

    const chevron = document.createElement("span");
    chevron.className = "cw-select-chevron";
    chevron.setAttribute("aria-hidden", "true");
    chevron.innerHTML =
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg>';

    const list = document.createElement("div");
    list.className = "cw-select-list hidden";
    list.setAttribute("role", "listbox");

    trigger.append(valueEl, chevron);
    wrap.append(trigger, list);

    const listPlaceholder = document.createComment("cw-select-list");
    let listPortaled = false;
    let focusIdx = -1;

    function unportalList() {
      list.classList.remove("cw-select-list--floating", "menu-anchor-up");
      list.style.cssText = "";
      if (listPortaled && listPlaceholder.parentNode) {
        listPlaceholder.parentNode.insertBefore(list, listPlaceholder);
        listPortaled = false;
      }
    }

    function placeList() {
      const ar = trigger.getBoundingClientRect();
      const minW = wrap.classList.contains("cw-select--poll") ? 92 : ar.width;
      list.style.display = "block";
      const lh = list.offsetHeight || 200;
      const lw = Math.max(list.offsetWidth || 0, minW);
      let top = ar.bottom + 6;
      let left = ar.left;
      list.classList.remove("menu-anchor-up");
      if (top + lh > window.innerHeight - 8 && ar.top > lh + 12) {
        top = ar.top - lh - 6;
        list.classList.add("menu-anchor-up");
      }
      left = Math.max(8, Math.min(left, window.innerWidth - lw - 8));
      top = Math.max(8, Math.min(top, window.innerHeight - lh - 8));
      list.style.cssText = `position:fixed;top:${top}px;left:${left}px;min-width:${Math.round(lw)}px;z-index:10140;`;
    }

    function portalList() {
      if (!listPortaled) {
        wrap.insertBefore(listPlaceholder, list);
        document.body.appendChild(list);
        listPortaled = true;
      }
      list.classList.add("cw-select-list--floating");
      placeList();
    }

    function closeInternal() {
      unportalList();
      list.classList.add("hidden");
      wrap.classList.remove("is-open");
      trigger.setAttribute("aria-expanded", "false");
      openSelectWraps.delete(wrap);
    }

    const onReposition = () => {
      if (wrap.classList.contains("is-open") && listPortaled) placeList();
    };
    window.addEventListener("resize", onReposition);
    window.addEventListener("scroll", onReposition, true);

    function syncDisabled() {
      const off = select.disabled;
      wrap.classList.toggle("is-disabled", off);
      trigger.disabled = off;
    }

    function syncLabel() {
      const opt = select.selectedOptions[0];
      valueEl.textContent = opt?.textContent?.trim() || "—";
    }

    function buildOptions() {
      list.innerHTML = "";
      focusIdx = -1;
      [...select.options].forEach((opt) => {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "cw-select-option";
        btn.setAttribute("role", "option");
        btn.dataset.value = opt.value;
        btn.textContent = opt.textContent;
        const selected = opt.selected;
        btn.setAttribute("aria-selected", selected ? "true" : "false");
        btn.classList.toggle("is-selected", selected);
        if (opt.disabled) {
          btn.disabled = true;
          btn.classList.add("is-disabled");
        }
        btn.addEventListener("click", (e) => {
          e.preventDefault();
          if (opt.disabled) return;
          select.value = opt.value;
          select.dispatchEvent(new Event("change", { bubbles: true }));
          buildOptions();
          close();
        });
        list.appendChild(btn);
      });
      syncLabel();
      syncDisabled();
    }

    function open() {
      if (select.disabled) return;
      closeAllSelects(wrap);
      closeAllFloatMenus();
      list.classList.remove("hidden");
      wrap.classList.add("is-open");
      trigger.setAttribute("aria-expanded", "true");
      openSelectWraps.add(wrap);
      portalList();
      requestAnimationFrame(() => {
        placeList();
        const selBtn = list.querySelector(".cw-select-option.is-selected");
        focusIdx = selBtn ? [...list.querySelectorAll(".cw-select-option:not(:disabled)")].indexOf(selBtn) : 0;
        focusOption(focusIdx);
      });
    }

    function close() {
      closeInternal();
    }

    function focusOption(idx) {
      const items = [...list.querySelectorAll(".cw-select-option:not(:disabled)")];
      if (!items.length) return;
      focusIdx = Math.max(0, Math.min(idx, items.length - 1));
      items.forEach((el, i) => el.classList.toggle("is-focused", i === focusIdx));
      items[focusIdx]?.scrollIntoView({ block: "nearest" });
    }

    function chooseFocused() {
      const items = [...list.querySelectorAll(".cw-select-option:not(:disabled)")];
      if (!items[focusIdx]) return;
      items[focusIdx].click();
    }

    trigger.addEventListener("click", (e) => {
      e.preventDefault();
      if (wrap.classList.contains("is-open")) close();
      else open();
    });

    trigger.addEventListener("keydown", (e) => {
      if (e.key === "ArrowDown" || e.key === "ArrowUp" || e.key === "Enter" || e.key === " ") {
        e.preventDefault();
      }
      if (!wrap.classList.contains("is-open")) {
        if (e.key === "ArrowDown" || e.key === "Enter" || e.key === " ") open();
        return;
      }
      if (e.key === "Escape") {
        close();
        return;
      }
      if (e.key === "ArrowDown") focusOption(focusIdx + 1);
      if (e.key === "ArrowUp") focusOption(focusIdx - 1);
      if (e.key === "Enter" || e.key === " ") chooseFocused();
    });

    select.addEventListener("change", buildOptions);

    selectControllers.set(select, { buildOptions, close: closeInternal, unportalList, listEl: list });
    buildOptions();
  }

  function refreshSelect(select) {
    if (!select) return;
    const ctrl = selectControllers.get(select);
    if (ctrl) ctrl.buildOptions();
    else enhanceSelect(select);
  }

  function enhanceSelects(root = document) {
    root.querySelectorAll("select").forEach((sel) => enhanceSelect(sel));
  }

  document.addEventListener("pointerdown", (e) => {
    if (e.target.closest(".cw-select")) return;
    if (e.target.closest(".cw-select-list--floating")) return;
    closeAllSelects();
  });

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => enhanceSelects());
  } else {
    enhanceSelects();
  }

  return {
    toast,
    confirmDialog,
    nameInputDialog,
    skeletonList,
    emptyState,
    fileIcon,
    hostAccent,
    showSplash,
    hideSplash,
    registerCommands,
    openCommandPalette,
    bindCommandPalette,
    bindRipple,
    renderBreadcrumbs,
    pathToBreadcrumbs,
    renderTransferProgress,
    escapeHtml,
    wireFloatingDropdown,
    closeAllFloatMenus,
    enhanceSelect,
    enhanceSelects,
    refreshSelect,
  };
})();

window.CWUI = CWUI;
window.CWConfirm = (msg, opts) => CWUI.confirmDialog(msg, opts);
