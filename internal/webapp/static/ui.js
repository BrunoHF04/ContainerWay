/** Utilitários visuais partilhados — toasts, confirmações, skeleton, etc. */
const CWUI = (() => {
  const $ = (sel, root = document) => root.querySelector(sel);

  function toast(message, type = "info", ms = 4200) {
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
      const done = (v) => { dlg.close(); resolve(v); };
      okBtn.onclick = () => done(true);
      cancelBtn.onclick = () => done(false);
      dlg.onclose = () => resolve(false);
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

  function showSplash(label = "A ligar SSH…") {
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
        $("#shortcuts-dialog")?.showModal();
      }
      if (e.key === "Escape") {
        $("#shortcuts-dialog")?.close();
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

  function renderBreadcrumbs(container, parts, onNavigate) {
    if (!container) return;
    container.innerHTML = "";
    const nav = document.createElement("nav");
    nav.className = "breadcrumbs";
    nav.setAttribute("aria-label", "Caminho");
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
    panel.classList.toggle("hidden", !inner.children.length && !(active && active.name));
  }

  return {
    toast,
    confirmDialog,
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
  };
})();

window.CWUI = CWUI;
