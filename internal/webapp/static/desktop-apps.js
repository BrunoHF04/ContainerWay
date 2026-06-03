/** Aplicações do ambiente Linux — funcionam em janelas (API SSH/SFTP/Docker). */
(function () {
  const apps = new Map();

  function escapeHtml(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function formatSize(n) {
    if (n == null || n === 0) return "";
    const u = ["B", "KB", "MB", "GB"];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < u.length - 1) {
      v /= 1024;
      i++;
    }
    return `${v.toFixed(i ? 1 : 0)} ${u[i]}`;
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

  function normPath(p) {
    const s = String(p || "").replace(/\\/g, "/").replace(/\/+$/, "");
    return s || "/";
  }

  function terminalWSUrl(cols, rows, cwd) {
    const p = location.protocol === "https:" ? "wss:" : "ws:";
    const lang = encodeURIComponent(window.CWWebPrefs?.get?.("lang") || window.CWI18n?.lang?.() || "pt");
    return `${p}//${location.host}/api/ssh/terminal/ws?cols=${cols || 80}&rows=${rows || 24}&cwd=${encodeURIComponent(cwd || "/")}&lang=${lang}`;
  }

  function dockerExecWSUrl(id) {
    const p = location.protocol === "https:" ? "wss:" : "ws:";
    return `${p}//${location.host}/api/docker/exec/ws?id=${encodeURIComponent(id)}`;
  }

  /** ID de tela para permissões — alinhado a accessauth + PERM_SCREENS (docs/PERMISSOES_TELAS.md). */
  const APP_PERM_SCREEN = {
    files: "explorer",
    terminal: "terminal",
    system: "desktop",
    monitor: "disks",
    disks: "disks",
    docker: "docker",
    automations: "automations",
    editor: "explorer",
    help: null,
  };

  function canUseScreen(id) {
    return typeof canScreen === "function" ? canScreen(id) : true;
  }

  function canUseApp(app) {
    if (!app) return false;
    if (app.id === "help") return true;
    const screenId = APP_PERM_SCREEN[app.id] ?? app.screen;
    if (!screenId) return true;
    return canUseScreen(screenId);
  }

  function canUseAction(id) {
    return typeof canAction === "function" ? canAction(id) : true;
  }

  async function syncDesktopSudo() {
    if (typeof refreshSudoUI === "function") await refreshSudoUI();
    return state.sudo || { enabled: false, user: "" };
  }

  function paintSudoButton(btn) {
    if (!btn) return;
    const on = !!state.sudo?.enabled;
    const user = state.sudo?.user || "root";
    btn.textContent = on ? user : "Root";
    btn.classList.toggle("is-active", on);
    btn.title = on
      ? `Root/sudo ativo (${user}) — clique para desativar`
      : "Ativar root/sudo para acessar arquivos protegidos";
    btn.hidden = !canUseAction("sudo.use");
  }

  async function desktopToggleSudo() {
    if (!state.ssh.connected) {
      CWUI.toast("Conecte-se ao SSH primeiro.", "error");
      return;
    }
    if (!canUseAction("sudo.use")) {
      CWUI.toast("Sem permissão para usar root/sudo.", "error");
      return;
    }
    await syncDesktopSudo();
    if (state.sudo?.enabled) {
      if (!(await CWConfirm("Desativar modo root/sudo no servidor?"))) return;
      try {
        await api("/api/ssh/sudo", { method: "POST", body: JSON.stringify({ action: "disable" }) });
        state.sudo = { enabled: false, user: "" };
        CWUI.toast("Root/sudo desativado", "success");
        document.dispatchEvent(new CustomEvent("cw-sudo-changed"));
      } catch (e) {
        CWUI.toast(e.message, "error");
      }
      return;
    }
    const dlg = $("#sudo-dialog");
    const err = $("#sudo-error");
    if (err) {
      err.textContent = "";
      err.classList.add("hidden");
    }
    const pass = $("#sudo-pass");
    if (pass) pass.value = "";
    dlg?.showModal();
    pass?.focus();
  }

  function register(def) {
    apps.set(def.id, def);
  }

  function localizedApp(def) {
    const t = window.CWI18n?.t?.bind(window.CWI18n);
    if (!t) return def;
    const labelKey = `desktop.app.${def.id}.label`;
    const descKey = `desktop.app.${def.id}.desc`;
    const label = t(labelKey);
    const desc = t(descKey);
    return {
      ...def,
      label: label !== labelKey ? label : def.label,
      desc: desc !== descKey ? desc : def.desc,
    };
  }

  /* —— Gerenciador de arquivos —— */
  register({
    id: "files",
    label: "Arquivos",
    icon: "📁",
    desc: "Pastas no servidor",
    screen: "explorer",
    w: 560,
    h: 420,
    singleton: false,
    mount(body, ctx) {
      const opts = ctx.opts || {};
      const st = {
        path: normPath(opts.path || state.remotePath || "/"),
        target: opts.target || (opts.containerId ? "container" : "host"),
        containerId: opts.containerId || "",
        containerName: opts.containerName || "",
        sel: null,
        entries: [],
        loadGen: 0,
      };
      const REMOTE_SHORTCUTS = [
        { label: "Raiz (/)", path: "/" },
        { label: "Home (/home)", path: "/home" },
        { label: "Etc", path: "/etc" },
        { label: "Var", path: "/var" },
        { label: "Opt", path: "/opt" },
        { label: "Tmp", path: "/tmp" },
        { label: "Root", path: "/root" },
      ];

      body.className = "linux-app linux-app-files";
      body.innerHTML = `
        <div class="linux-files-chrome">
          <div class="linux-files-target" role="toolbar" data-i18n-aria="linux.files.dest.aria">
            <div class="linux-target-tabs" role="group" data-i18n-aria="linux.files.target.aria">
              <button type="button" class="linux-target-btn" data-target="host" data-i18n="linux.files.target.host">Servidor</button>
              <button type="button" class="linux-target-btn" data-target="container" data-i18n="linux.files.target.docker">Docker</button>
            </div>
            <select class="linux-container-select hidden" data-container-select aria-label="Docker"></select>
          </div>
          <div class="linux-files-toolbar" role="toolbar" data-i18n-aria="linux.files.toolbar.aria">
            <div class="linux-files-toolbar-actions">
              <button type="button" class="btn btn-ghost btn-sm btn-icon-text" data-act="up" data-i18n-title="linux.files.up">↑</button>
              <button type="button" class="btn btn-ghost btn-sm btn-icon-text" data-act="refresh" data-i18n-title="linux.files.refresh">↻</button>
              <button type="button" class="btn btn-ghost btn-sm btn-icon-text" data-act="open" data-i18n-title="linux.files.open">▶</button>
              <button type="button" class="btn btn-ghost btn-sm btn-icon-text" data-act="preview" data-i18n-title="linux.files.preview">👁</button>
              <button type="button" class="btn btn-ghost btn-sm" data-act="mkdir" data-i18n-title="linux.files.mkdir">+</button>
              <button type="button" class="btn btn-ghost btn-sm" data-act="rename" data-i18n-title="linux.files.rename">✎</button>
              <button type="button" class="btn btn-ghost btn-sm" data-act="push" data-i18n-title="linux.files.push">↑</button>
              <button type="button" class="btn btn-ghost btn-sm" data-act="pull" data-i18n-title="linux.files.pull">↓</button>
              <button type="button" class="btn btn-ghost btn-sm btn-icon-text" data-act="fav-add" data-i18n-title="linux.files.fav">★</button>
              <button type="button" class="btn btn-ghost btn-sm btn-sudo" data-act="sudo" data-i18n-title="linux.files.sudo">Root</button>
              <button type="button" class="btn btn-ghost btn-sm btn-icon-text" data-act="delete" data-i18n-title="linux.files.delete">🗑</button>
            </div>
            <input type="search" class="linux-files-search panel-filter" data-filter data-i18n="linux.files.filter" data-i18n-mode="placeholder" placeholder="Filtrar nesta pasta…" data-i18n-aria="linux.files.filter.aria" autocomplete="off" />
            <label class="linux-fav-wrap" data-i18n-title="linux.files.shortcuts.title">
              <span class="sr-only" data-i18n="linux.files.shortcuts">Atalhos</span>
              <select class="linux-fav-select panel-select" data-fav data-i18n-aria="linux.files.shortcuts"><option value="" data-i18n="linux.files.shortcuts.option">Atalhos…</option></select>
            </label>
            <input type="file" class="linux-upload-input" data-upload multiple hidden />
          </div>
        </div>
        <div class="linux-app-path" data-path-wrap></div>
        <div class="linux-app-list linux-scroll" data-list tabindex="0" role="listbox" aria-label="Arquivos"></div>
        <div class="linux-drop-overlay hidden" data-dropzone data-i18n="linux.files.dropzone"> Solte arquivos para enviar </div>`;

      window.CWI18n?.applyLabels?.(body);

      const listEl = body.querySelector("[data-list]");
      const pathWrap = body.querySelector("[data-path-wrap]");
      const sudoBtn = body.querySelector('[data-act="sudo"]');
      const favSel = body.querySelector("[data-fav]");
      const filterInp = body.querySelector("[data-filter]");
      const uploadInput = body.querySelector("[data-upload]");
      const dropZone = body.querySelector("[data-dropzone]");
      const containerSel = body.querySelector("[data-container-select]");
      const targetBtns = body.querySelectorAll(".linux-target-btn");

      function containerQs() {
        return st.target === "container" && st.containerId
          ? `&containerId=${encodeURIComponent(st.containerId)}`
          : "";
      }

      function containerBody(extra = {}) {
        if (st.target === "container" && st.containerId) {
          return { ...extra, containerId: st.containerId };
        }
        return extra;
      }

      function shortContainerLabel(c) {
        const name = (c.displayName || c.name || c.id || "").replace(/^\//, "");
        return name.length > 22 ? `${name.slice(0, 20)}…` : name;
      }

      function updatePathDisplay() {
        if (!pathWrap) return;
        pathWrap.innerHTML = "";
        pathWrap.appendChild(
          window.CWDesktopCore.renderBreadcrumbs(st.path, st.target, st.containerName, (p) => load(p))
        );
      }

      function favLabel(path, prefix = "") {
        const p = path || "/";
        const short = p.length > 34 ? "…" + p.slice(-32) : p;
        return prefix + short;
      }

      function addFavOption(value, text, disabled = false) {
        const o = document.createElement("option");
        o.value = value;
        o.textContent = text;
        if (disabled) o.disabled = true;
        favSel.appendChild(o);
      }

      async function refreshFavorites() {
        if (!favSel) return;
        favSel.innerHTML = '<option value="">Atalhos…</option>';
        for (const s of REMOTE_SHORTCUTS) {
          addFavOption(s.path, s.label);
        }
        const recent = JSON.parse(sessionStorage.getItem("cw-desktop-recent-paths") || "[]").filter(
          (p) => p && p !== "/" && !REMOTE_SHORTCUTS.some((s) => s.path === p)
        );
        if (recent.length) {
          addFavOption("__recent", "— Recentes —", true);
          for (const p of recent.slice(0, 8)) {
            addFavOption(p, favLabel(p, "↩ "));
          }
        }
        let favPaths = [];
        try {
          const data = await api("/api/explorer/favorites?side=right");
          favPaths = data.paths || [];
        } catch (_) { /* */ }
        if (favPaths.length) {
          addFavOption("__fav", "— Favoritos —", true);
          for (const p of favPaths) {
            addFavOption(p, favLabel(p, "★ "));
          }
        }
        CWUI.refreshSelect?.(favSel);
      }

      async function addFavorite() {
        const path = normPath(st.path || "/");
        if (!path) return;
        try {
          const data = await api("/api/explorer/favorites?side=right");
          const paths = data.paths || [];
          if (!paths.includes(path)) paths.push(path);
          await api("/api/explorer/favorites?side=right", {
            method: "PUT",
            body: JSON.stringify({ paths }),
          });
          await refreshFavorites();
          CWUI.toast("Pasta salva nos favoritos", "success");
        } catch (e) {
          CWUI.toast(e.message || "Não foi possível salvar favorito", "error");
        }
      }

      function filterEntries(entries, q) {
        if (typeof window.cwPanelFilter === "function") return window.cwPanelFilter(entries, q);
        const hay = (q || "").trim().toLowerCase();
        if (!hay) return entries || [];
        return (entries || []).filter((e) => e.name === ".." || e.name.toLowerCase().includes(hay));
      }

      function renderEntries(entries) {
        const list = filterEntries(entries, filterInp?.value || "");
        listEl.innerHTML = "";
        if (!list.length) {
          const q = (filterInp?.value || "").trim();
          listEl.innerHTML = `<p class="muted linux-app-empty">${q ? "Nenhum item corresponde ao filtro." : "Pasta vazia"}</p>`;
          return;
        }
        for (const e of list) {
          const isDir = entryIsDir(e);
          const row = document.createElement("div");
          row.className = "linux-file-row" + (isDir ? " is-dir" : "");
          row.setAttribute("role", "option");
          row.tabIndex = -1;
          const icon = window.CWUI?.fileIcon?.(e.name, isDir) || (isDir ? "📁" : "📄");
          row.innerHTML = `<span class="linux-file-icon">${icon}</span><span class="linux-file-name">${escapeHtml(e.name)}</span><span class="linux-file-meta">${isDir && e.name !== ".." ? "" : e.name === ".." ? "" : formatSize(e.size)}</span>`;
          row.addEventListener("click", (ev) => {
            ev.stopPropagation();
            if (isDir) {
              openEntry(e);
              return;
            }
            selectEntry(row, e);
          });
          row.addEventListener("dblclick", (ev) => {
            ev.preventDefault();
            ev.stopPropagation();
            openEntry(e);
          });
          listEl.appendChild(row);
        }
      }

      function clearFileFilter() {
        if (filterInp) filterInp.value = "";
      }

      function rememberPath(p) {
        try {
          const arr = JSON.parse(sessionStorage.getItem("cw-desktop-recent-paths") || "[]");
          const next = [p, ...arr.filter((x) => x !== p)].slice(0, 12);
          sessionStorage.setItem("cw-desktop-recent-paths", JSON.stringify(next));
        } catch (_) { /* */ }
      }

      async function uploadFiles(fileList) {
        if (!fileList?.length) return;
        if (!canUseAction("files.transfer")) {
          CWUI.toast("Sem permissão para transferências.", "error");
          return;
        }
        const form = new FormData();
        form.append("remoteDir", st.path || "/");
        if (st.target === "container" && st.containerId) {
          form.append("containerId", st.containerId);
        }
        for (const f of fileList) {
          form.append("files", f);
          const rel = f.webkitRelativePath || f.name;
          if (rel && rel !== f.name) form.append("paths", rel);
        }
        try {
          const res = await fetch("/api/transfer/upload", { method: "POST", body: form, credentials: "same-origin" });
          const data = await res.json().catch(() => ({}));
          if (!res.ok) throw new Error(data.error || res.statusText);
          CWUI.toast(data.message || "Upload enfileirado", "success");
          load(st.path);
        } catch (e) {
          CWUI.toast(e.message, "error");
        }
      }

      function updateTargetUI() {
        const isContainer = st.target === "container";
        targetBtns.forEach((b) => b.classList.toggle("active", b.dataset.target === st.target));
        containerSel?.classList.toggle("hidden", !isContainer);
        if (sudoBtn) sudoBtn.hidden = isContainer || !canUseAction("sudo.use");
      }

      async function loadContainersSelect() {
        if (!containerSel) return;
        containerSel.innerHTML = "";
        const ph = document.createElement("option");
        ph.value = "";
        ph.textContent = "Selecione um contêiner…";
        containerSel.appendChild(ph);
        try {
          const data = await api("/api/docker/containers");
          const list = (data.containers || []).filter(
            (c) => c.running || c.state === "running" || c.restarting || c.state === "restarting"
          );
          if (!list.length) {
            ph.textContent = "Nenhum contêiner em execução";
            return;
          }
          for (const c of list) {
            const opt = document.createElement("option");
            opt.value = c.idFull || c.id;
            opt.textContent = shortContainerLabel(c);
            opt.title = `${(c.displayName || c.name || "").replace(/^\//, "")} — ${c.image || ""}`;
            opt.dataset.name = (c.displayName || c.name || c.id || "").replace(/^\//, "");
            containerSel.appendChild(opt);
          }
          if (st.containerId) {
            containerSel.value = st.containerId;
            const opt = containerSel.selectedOptions[0];
            if (opt?.dataset.name) st.containerName = opt.dataset.name;
          } else if (containerSel.options.length > 1) {
            containerSel.selectedIndex = 1;
            st.containerId = containerSel.value;
            const opt = containerSel.selectedOptions[0];
            if (opt?.dataset.name) st.containerName = opt.dataset.name;
          }
        } catch {
          ph.textContent = "Docker indisponível";
        }
        CWUI.refreshSelect?.(containerSel);
      }

      function setTarget(mode) {
        if (st.target === mode) return;
        st.target = mode;
        st.path = "/";
        st.sel = null;
        clearFileFilter();
        if (mode === "container") {
          loadContainersSelect().then(() => {
            if (!st.containerId) {
              listEl.innerHTML = `<p class="muted linux-app-empty">Selecione um contêiner Docker.</p>`;
              updatePathDisplay();
              return;
            }
            load("/");
          });
        } else {
          st.containerId = "";
          st.containerName = "";
          load(st.path);
        }
        updateTargetUI();
      }

      targetBtns.forEach((btn) => {
        btn.addEventListener("click", () => setTarget(btn.dataset.target));
      });
      containerSel?.addEventListener("change", () => {
        st.containerId = containerSel.value || "";
        const opt = containerSel.selectedOptions[0];
        st.containerName = opt?.dataset.name || opt?.textContent || "";
        st.path = "/";
        st.sel = null;
        clearFileFilter();
        if (!st.containerId) {
          listEl.innerHTML = `<p class="muted linux-app-empty">Selecione um contêiner Docker.</p>`;
          updatePathDisplay();
          return;
        }
        load("/");
      });

      const onSudoChanged = () => {
        paintSudoButton(sudoBtn);
        load(st.path);
      };
      document.addEventListener("cw-sudo-changed", onSudoChanged);
      const onLang = () => {
        window.CWI18n?.applyLabels?.(body);
        updatePathDisplay();
      };
      document.addEventListener("cw-lang-change", onLang);
      st.cleanup = () => {
        document.removeEventListener("cw-sudo-changed", onSudoChanged);
        document.removeEventListener("cw-lang-change", onLang);
      };
      ctx.state = st;
      st.renderTaskbar = () => {
        const w = window.CWDesktop?.getWindow?.(ctx.winId);
        if (w) w.ctx.state = st;
      };

      syncDesktopSudo().then(() => paintSudoButton(sudoBtn));
      refreshFavorites();
      favSel?.addEventListener("change", () => {
        const v = favSel.value;
        favSel.value = "";
        CWUI.refreshSelect?.(favSel);
        if (v && !v.startsWith("__")) load(v);
      });
      filterInp?.addEventListener("input", () => renderEntries(st.entries || []));
      uploadInput?.addEventListener("change", () => {
        uploadFiles(uploadInput.files);
        uploadInput.value = "";
      });
      ["dragenter", "dragover"].forEach((ev) => {
        listEl.addEventListener(ev, (e) => {
          e.preventDefault();
          dropZone?.classList.remove("hidden");
        });
      });
      listEl.addEventListener("dragleave", (e) => {
        if (!listEl.contains(e.relatedTarget)) dropZone?.classList.add("hidden");
      });
      listEl.addEventListener("drop", (e) => {
        e.preventDefault();
        dropZone?.classList.add("hidden");
        uploadFiles(e.dataTransfer?.files);
      });

      function entryIsDir(e) {
        return !!(e.isDir || e.name === "..");
      }

      function openEntry(e) {
        if (!e) return;
        if (entryIsDir(e)) {
          load(e.path);
          return;
        }
        if (e.name === "..") return;
        if (!canUseAction("files.read")) {
          CWUI.toast("Sem permissão para abrir arquivos.", "error");
          return;
        }
        window.CWDesktop?.openEditor?.(e.path, e.name, ctx.winId, {
          containerId: st.target === "container" ? st.containerId : "",
          containerName: st.containerName,
        });
      }

      function selectEntry(row, e) {
        listEl.querySelectorAll(".linux-file-row").forEach((r) => r.classList.remove("is-selected"));
        row.classList.add("is-selected");
        st.sel = e;
      }

      async function load(path) {
        if (!state.ssh.connected) {
          listEl.innerHTML = `<p class="muted linux-app-empty">Conecte-se ao SSH.</p>`;
          return;
        }
        if (st.target === "container" && !st.containerId) {
          listEl.innerHTML = `<p class="muted linux-app-empty">Selecione um contêiner Docker.</p>`;
          updatePathDisplay();
          return;
        }
        const target = normPath(path || "/");
        const gen = ++st.loadGen;
        listEl.innerHTML = `<p class="muted linux-app-empty">Carregando…</p>`;
        try {
          const data = await api(`/api/remote/list?path=${encodeURIComponent(target)}${containerQs()}`);
          if (gen !== st.loadGen) return;
          st.path = normPath(data.path);
          if (st.target === "host") state.remotePath = st.path;
          rememberPath(st.path);
          st.sel = null;
          updatePathDisplay();
          refreshFavorites();
          st.entries = data.entries || [];
          renderEntries(st.entries);
        } catch (err) {
          if (gen !== st.loadGen) return;
          listEl.innerHTML = `<p class="linux-app-empty error">${escapeHtml(err.message)}</p>`;
          CWUI.toast(err.message, "error");
        }
      }

      function previewSelection() {
        if (!st.sel || st.sel.isDir || st.sel.name === "..") {
          CWUI.toast("Selecione um arquivo.", "error");
          return;
        }
        window.CWDesktopEnhancements?.openFilePreview?.(
          st.sel.path,
          st.target === "container" ? st.containerId : "",
          st.sel.name
        );
      }

      listEl.addEventListener("keydown", (ev) => {
        if (ev.key === "Enter" && st.sel) {
          ev.preventDefault();
          openEntry(st.sel);
        }
        if (ev.key === " " && st.sel && !st.sel.isDir && st.sel.name !== "..") {
          ev.preventDefault();
          previewSelection();
        }
      });

      body.addEventListener("click", async (ev) => {
        if (ev.target.closest(".linux-file-row")) return;
        const btn = ev.target.closest("[data-act]");
        if (!btn) return;
        const act = btn.dataset.act;
        if (act === "open") return openEntry(st.sel);
        if (act === "sudo") return desktopToggleSudo();
        if (act === "fav-add") return addFavorite();
        if (act === "refresh") return load(st.path);
        if (ev.target.closest(".linux-target-btn, [data-container-select]")) return;
        if (act === "up") {
          const data = await api(`/api/remote/list?path=${encodeURIComponent(st.path)}${containerQs()}`);
          const parent = (data.entries || []).find((e) => e.name === "..");
          if (parent) return load(parent.path);
          return;
        }
        if (act === "preview") return previewSelection();
        if (act === "mkdir") {
          const name = await (window.CWDesktopEnhancements?.linuxPrompt?.({
            title: "Nova pasta",
            label: "Nome da pasta",
          }) ?? Promise.resolve(prompt("Nome da nova pasta:")));
          if (!name) return;
          const newPath = normPath(st.path) + "/" + name;
          try {
            await api("/api/remote/mkdir", {
              method: "POST",
              body: JSON.stringify(containerBody({ path: newPath })),
            });
            CWUI.toast("Pasta criada", "success");
            load(st.path);
          } catch (e) {
            CWUI.toast(e.message, "error");
          }
          return;
        }
        if (act === "rename") {
          if (!st.sel || st.sel.name === "..") {
            CWUI.toast("Selecione um item.", "error");
            return;
          }
          if (!canUseAction("files.write")) {
            CWUI.toast("Sem permissão para renomear.", "error");
            return;
          }
          const name = await (window.CWDesktopEnhancements?.linuxPrompt?.({
            title: "Renomear",
            label: "Novo nome",
            defaultValue: st.sel.name,
          }) ?? Promise.resolve(prompt("Novo nome:", st.sel.name)));
          if (!name || name === st.sel.name) return;
          const dir = st.sel.path.replace(/[/\\][^/\\]+$/, "");
          const newPath = dir + "/" + name;
          try {
            await api("/api/remote/rename", {
              method: "POST",
              body: JSON.stringify(containerBody({ oldPath: st.sel.path, newPath })),
            });
            CWUI.toast("Renomeado", "success");
            load(st.path);
          } catch (e) {
            CWUI.toast(e.message, "error");
          }
          return;
        }
        if (act === "push") {
          uploadInput?.click();
          return;
        }
        if (act === "delete") {
          if (!st.sel || st.sel.name === "..") {
            CWUI.toast("Selecione um item.", "error");
            return;
          }
          const ok = await window.CWDesktopCore.confirmDelete({
            target: st.target,
            containerName: st.containerName,
            itemName: st.sel.name,
            isDir: !!st.sel.isDir,
          });
          if (!ok) return;
          try {
            await api("/api/remote/delete", {
              method: "POST",
              body: JSON.stringify(containerBody({ path: st.sel.path, recursive: !!st.sel.isDir })),
            });
            CWUI.toast("Removido", "success");
            load(st.path);
          } catch (e) {
            CWUI.toast(e.message, "error");
          }
          return;
        }
        if (act === "pull") {
          if (!st.sel || st.sel.name === "..") {
            CWUI.toast("Selecione um arquivo ou pasta.", "error");
            return;
          }
          if (!canUseAction("files.transfer")) {
            CWUI.toast("Sem permissão para transferências.", "error");
            return;
          }
          let localPath = state.localPath;
          if (!localPath) {
            try {
              const loc = await api("/api/local/list");
              localPath = loc.path;
              state.localPath = localPath;
            } catch (e) {
              CWUI.toast(e.message, "error");
              return;
            }
          }
          if (!(await CWConfirm(`Baixar "${st.sel.name}" para o PC (${localPath})?`))) return;
          try {
            await api("/api/transfer/pull", {
              method: "POST",
              body: JSON.stringify(containerBody({ localPath, remotePath: st.sel.path })),
            });
            CWUI.toast("Download enfileirado", "success");
          } catch (e) {
            CWUI.toast(e.message, "error");
          }
        }
      });

      updateTargetUI();
      if (st.target === "container") {
        loadContainersSelect().then(() => load(st.path));
      } else {
        load(st.path);
      }
    },
    unmount(ctx) {
      ctx.state?.cleanup?.();
    },
  });

  /* —— Terminal —— */
  register({
    id: "terminal",
    label: "Terminal",
    icon: "⌨️",
    desc: "Linha de comandos",
    screen: "terminal",
    w: 640,
    h: 400,
    singleton: false,
    mount(body, ctx) {
      body.className = "linux-app linux-app-terminal";
      body.innerHTML = `
        <div class="linux-term-chrome">
          <div class="linux-term-tabs" data-tabs role="tablist"></div>
          <button type="button" class="btn btn-ghost btn-sm linux-term-add" data-add-tab title="Nova aba">+</button>
        </div>
        <div class="linux-term-stack" data-stack></div>`;
      const tabsEl = body.querySelector("[data-tabs]");
      const stack = body.querySelector("[data-stack]");
      const fontSize = Number(window.CWWebPrefs?.get?.("terminalFontSize")) || 14;
      let tabSeq = 0;
      const tabs = [];
      let activeId = null;

      function fitActive() {
        const t = tabs.find((x) => x.id === activeId);
        if (!t) return;
        try {
          t.fit.fit();
          if (t.ws?.readyState === WebSocket.OPEN) {
            t.ws.send(JSON.stringify({ op: "resize", cols: t.term.cols, rows: t.term.rows }));
          }
        } catch (_) { /* */ }
      }

      function activateTab(id) {
        activeId = id;
        for (const t of tabs) {
          t.panel.classList.toggle("hidden", t.id !== id);
          t.tabBtn.classList.toggle("is-active", t.id === id);
        }
        const cur = tabs.find((x) => x.id === id);
        if (cur?.termApi) window.CWTerminalTools?.mountToolbar?.(body, cur.termApi, { statusEl: cur.statusEl });
        requestAnimationFrame(fitActive);
      }

      function closeTab(id) {
        const idx = tabs.findIndex((t) => t.id === id);
        if (idx < 0) return;
        const t = tabs[idx];
        t.ro?.disconnect();
        t.ws?.close();
        t.term?.dispose();
        t.tabBtn.remove();
        t.panel.remove();
        tabs.splice(idx, 1);
        if (activeId === id) {
          const next = tabs[Math.max(0, idx - 1)];
          if (next) activateTab(next.id);
          else activeId = null;
        }
      }

      function connectTab(t) {
        t.endedBanner.classList.add("hidden");
        t.panel.classList.remove("is-session-ended");
        if (t.ws) {
          t.ws.close();
          t.ws = null;
        }
        t.ws = new WebSocket(
          t.containerId ? dockerExecWSUrl(t.containerId) : terminalWSUrl(t.term.cols, t.term.rows, state.remotePath)
        );
        t.ws.onopen = () => {
          try {
            t.fit.fit();
            if (t.ws?.readyState === WebSocket.OPEN) {
              t.ws.send(JSON.stringify({ op: "resize", cols: t.term.cols, rows: t.term.rows }));
            }
          } catch (_) { /* */ }
        };
        t.ws.onmessage = (ev) => {
          const chunk = typeof ev.data === "string" ? ev.data : "";
          if (chunk) {
            const max = 2 * 1024 * 1024;
            t.logRef.value += chunk;
            if (t.logRef.value.length > max) t.logRef.value = t.logRef.value.slice(-max);
          }
          t.term.write(ev.data);
        };
        t.ws.onclose = () => {
          t.term.writeln("\r\n\x1b[33mSessão terminada.\x1b[0m\r\n");
          t.endedBanner.classList.remove("hidden");
          t.panel.classList.add("is-session-ended");
        };
        t.termApi = window.CWTerminalTools?.createTermApi?.(t.term, t.ws, { logRef: t.logRef, onReconnect: () => connectTab(t) });
      }

      function addTab(opts = {}) {
        const id = ++tabSeq;
        const panel = document.createElement("div");
        panel.className = "linux-term-panel";
        panel.dataset.tabId = String(id);
        const statusEl = document.createElement("p");
        statusEl.className = "terminal-status linux-term-status hidden";
        statusEl.setAttribute("role", "status");
        const endedBanner = document.createElement("div");
        endedBanner.className = "terminal-ended-banner linux-term-ended hidden";
        endedBanner.setAttribute("role", "alert");
        endedBanner.innerHTML =
          '<span>Sessão terminada.</span><button type="button" class="btn btn-primary btn-sm" data-term-reconnect>Reconectar</button>';
        const host = document.createElement("div");
        host.className = "linux-term-host";
        panel.append(statusEl, endedBanner, host);
        stack.appendChild(panel);
        const term = new Terminal({
          theme: { background: "#1a1a2e", foreground: "#e8eef7", cursor: "#38bdf8" },
          fontSize,
        });
        const fit = new (window.FitAddon?.FitAddon || FitAddon.FitAddon)();
        term.loadAddon(fit);
        term.open(host);
        const logRef = { value: "" };
        const containerId = opts.containerId || "";
        const label = opts.label || (containerId ? `🐳 ${(opts.containerName || "Docker").slice(0, 14)}` : `SSH ${tabs.length + 1}`);
        const tabBtn = document.createElement("button");
        tabBtn.type = "button";
        tabBtn.className = "linux-term-tab";
        tabBtn.setAttribute("role", "tab");
        tabBtn.title = label;
        const labelSpan = document.createElement("span");
        labelSpan.className = "linux-term-tab-label";
        labelSpan.textContent = label;
        const closeBtn = document.createElement("button");
        closeBtn.type = "button";
        closeBtn.className = "linux-term-tab-close";
        closeBtn.textContent = "×";
        closeBtn.title = "Fechar aba";
        closeBtn.addEventListener("click", (ev) => {
          ev.stopPropagation();
          if (tabs.length < 2) {
            CWUI.toast("Mantenha pelo menos uma aba.", "info");
            return;
          }
          closeTab(id);
        });
        tabBtn.append(labelSpan, closeBtn);
        tabsEl.appendChild(tabBtn);
        const t = { id, panel, tabBtn, host, term, fit, ws: null, logRef, termApi: null, statusEl, endedBanner, containerId };
        tabBtn.addEventListener("click", () => activateTab(id));
        endedBanner.querySelector("[data-term-reconnect]")?.addEventListener("click", () => connectTab(t));
        term.onData((d) => {
          if (t.ws?.readyState === WebSocket.OPEN) t.ws.send(d);
        });
        const ro = typeof ResizeObserver !== "undefined" ? new ResizeObserver(() => {
          if (activeId === id) fitActive();
        }) : null;
        ro?.observe(host);
        t.ro = ro;
        tabs.push(t);
        connectTab(t);
        activateTab(id);
        return t;
      }

      body.querySelector("[data-add-tab]")?.addEventListener("click", () => addTab());
      addTab({
        containerId: ctx.opts?.containerId || "",
        containerName: ctx.opts?.containerName || "",
        label: ctx.opts?.containerId
          ? `🐳 ${(ctx.opts.containerName || "Docker").replace(/^\//, "").slice(0, 18)}`
          : "SSH",
      });
      ctx.state = { tabs, fitActive, addTab, closeTab };
      ctx.onFocus = fitActive;
    },
    unmount(ctx) {
      const s = ctx.state;
      if (!s?.tabs) return;
      for (const t of [...s.tabs]) {
        t.ro?.disconnect();
        t.ws?.close();
        t.term?.dispose();
      }
      s.tabs.length = 0;
    },
  });

  /* —— Monitor do sistema —— */
  register({
    id: "monitor",
    label: "Monitor",
    icon: "📊",
    desc: "CPU, RAM e disco",
    screen: "disks",
    hidden: true,
    w: 400,
    h: 340,
    singleton: true,
    mount(body, ctx) {
      body.className = "linux-app linux-app-monitor";
      body.innerHTML = `<div class="linux-monitor-grid" data-body><p class="muted">Carregando…</p></div>`;
      const grid = body.querySelector("[data-body]");
      const render = (ov) => {
        if (!ov) {
          grid.innerHTML = `<p class="muted">Sem dados</p>`;
          return;
        }
        grid.innerHTML = `
          <div class="linux-monitor-card">
            <h4>Memória</h4>
            ${pctBar(ov.memPct)}
            <p class="muted">${formatBytes(ov.memUsed)} usados de ${formatBytes(ov.memTotal)}</p>
          </div>
          <div class="linux-monitor-card">
            <h4>Disco /</h4>
            ${pctBar(ov.diskPct)}
            <p class="muted">${formatBytes(ov.diskUsed)} de ${formatBytes(ov.diskTotal)}</p>
          </div>
          <div class="linux-monitor-card linux-monitor-card--wide">
            <h4>Servidor</h4>
            <p><strong>${escapeHtml(ov.hostname)}</strong> · ${escapeHtml(ov.os)}</p>
            <p class="muted">Ativo: ${escapeHtml(ov.uptime || "—")} · ${ov.cpus || "?"} núcleos</p>
            <p class="muted">Carga: ${escapeHtml(ov.load1)} ${escapeHtml(ov.load5)} ${escapeHtml(ov.load15)}</p>
            <p class="muted">Atualizado ${escapeHtml(ov.updatedAt || "")}</p>
          </div>`;
      };
      const tick = async () => {
        if (!state.ssh.connected) return;
        try {
          const ov = await api("/api/desktop/overview", { noAuthRedirect: true });
          window.CWDesktop?.setOverviewCache?.(ov);
          render(ov);
        } catch (e) {
          grid.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
        }
      };
      ctx.state = { timer: setInterval(tick, 5000) };
      ctx.onFocus = tick;
      tick();
    },
    unmount(ctx) {
      if (ctx.state?.timer) clearInterval(ctx.state.timer);
    },
  });

  /* —— Discos —— */
  register({
    id: "disks",
    label: "Discos",
    icon: "💾",
    desc: "Volumes e montagens",
    screen: "disks",
    hidden: true,
    w: 480,
    h: 380,
    singleton: true,
    mount(body, ctx) {
      body.className = "linux-app linux-app-disks";
      body.innerHTML = `<div class="linux-disks-list" data-list><p class="muted">Carregando…</p></div>`;
      const list = body.querySelector("[data-list]");
      const load = async () => {
        try {
          const data = await api("/api/disks/summary");
          const devs = data.devices || [];
          if (!devs.length) {
            list.innerHTML = `<p class="muted">Nenhum dispositivo listado.</p>`;
            return;
          }
          list.innerHTML = `<table class="linux-table"><thead><tr><th>Dispositivo</th><th>Uso</th><th>Montagem</th></tr></thead><tbody></tbody></table>`;
          const tb = list.querySelector("tbody");
          for (const d of devs) {
            const tr = document.createElement("tr");
            const pad = "&nbsp;".repeat(Math.min(6, (d.depth || 0) * 2));
            tr.innerHTML = `<td>${pad}${escapeHtml(d.name || d.path || "")}</td><td>${escapeHtml(d.size || "—")}</td><td>${escapeHtml(d.mount || "—")}</td>`;
            tb.appendChild(tr);
          }
        } catch (e) {
          list.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
        }
      };
      ctx.onFocus = load;
      load();
    },
  });

  /* —— Sistema (rede, armazenamento, serviços) —— */
  register({
    id: "system",
    label: "Sistema",
    icon: "🖧",
    desc: "Rede, discos e serviços",
    screen: "desktop",
    w: 640,
    h: 480,
    singleton: true,
    mount(body, ctx) {
      body.className = "linux-app linux-app-system";
      body.innerHTML = `
        <nav class="linux-sys-tabs" role="tablist">
          <button type="button" class="linux-sys-tab active" data-tab="overview" role="tab">Resumo</button>
          <button type="button" class="linux-sys-tab" data-tab="network" role="tab">Rede</button>
          <button type="button" class="linux-sys-tab" data-tab="storage" role="tab">Armazenamento</button>
          <button type="button" class="linux-sys-tab" data-tab="services" role="tab">Serviços</button>
        </nav>
        <div class="linux-sys-panels">
          <section class="linux-sys-panel" data-panel="overview"></section>
          <section class="linux-sys-panel hidden" data-panel="network"></section>
          <section class="linux-sys-panel hidden" data-panel="storage"></section>
          <section class="linux-sys-panel hidden" data-panel="services"></section>
        </div>`;
      const panels = {};
      body.querySelectorAll("[data-panel]").forEach((p) => {
        panels[p.dataset.panel] = p;
      });
      let activeTab = "overview";
      const sys = { lvOptions: [], rows: [] };

      function canManageHost() {
        return canUseAction("sudo.use");
      }

      function canManageDisks() {
        return canUseAction("disks.manage");
      }

      function paintSudoChip(el) {
        if (!el) return;
        const on = !!state.sudo?.enabled;
        const user = state.sudo?.user || "root";
        el.textContent = on ? `sudo · ${user}` : "sudo · inativo";
        el.classList.toggle("is-on", on);
        el.title = on
          ? "Sudo ativo — pode alterar rede, serviços e volumes"
          : "Ative o sudo (botão Root no hub ou painel) para alterações no host";
      }

      async function loadOverview() {
        const p = panels.overview;
        p.innerHTML = `<p class="muted">Carregando…</p>`;
        try {
          const ov = await api("/api/desktop/overview", { noAuthRedirect: true });
          window.CWDesktop?.setOverviewCache?.(ov);
          p.innerHTML = `
            <div class="linux-monitor-grid">
              <div class="linux-monitor-card"><h4>Memória</h4>${pctBar(ov.memPct)}<p class="muted">${formatBytes(ov.memUsed)} / ${formatBytes(ov.memTotal)}</p></div>
              <div class="linux-monitor-card"><h4>Disco /</h4>${pctBar(ov.diskPct)}<p class="muted">${formatBytes(ov.diskUsed)} / ${formatBytes(ov.diskTotal)}</p></div>
              <div class="linux-monitor-card linux-monitor-card--wide">
                <h4>Host</h4>
                <p><strong>${escapeHtml(ov.hostname)}</strong> · ${escapeHtml(ov.os)}</p>
                <p class="muted">Ativo: ${escapeHtml(ov.uptime || "—")} · ${ov.cpus || "?"} CPUs</p>
                <p class="muted">Carga ${escapeHtml(ov.load1)} ${escapeHtml(ov.load5)} ${escapeHtml(ov.load15)}</p>
              </div>
            </div>
            <p class="muted linux-sys-hint">Use os separadores <strong>Rede</strong>, <strong>Armazenamento</strong> e <strong>Serviços</strong> para gerir o host (requer sudo).</p>`;
        } catch (e) {
          p.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
        }
      }

      async function loadNetwork() {
        const p = panels.network;
        const manage = canManageHost();
        p.innerHTML = `<p class="muted">Carregando…</p>`;
        try {
          await syncDesktopSudo();
          const net = await api("/api/desktop/network", { noAuthRedirect: true });
          const dnsVal = (net.dns || []).join(", ");
          let rows = "";
          for (const iface of net.interfaces || []) {
            const isLo = iface.name === "lo";
            const acts =
              manage && !isLo
                ? `<td class="linux-sys-actions"><button type="button" class="btn btn-ghost btn-sm" data-iface-up="${escapeHtml(iface.name)}">Ligar</button>
                   <button type="button" class="btn btn-ghost btn-sm" data-iface-down="${escapeHtml(iface.name)}">Desligar</button></td>`
                : `<td class="muted">—</td>`;
            rows += `<tr><td><strong>${escapeHtml(iface.name)}</strong></td><td>${escapeHtml(iface.state)}</td><td>${escapeHtml((iface.ipv4 || []).join(", ") || "—")}</td><td class="muted">${escapeHtml(iface.mac || "—")}</td>${acts}</tr>`;
          }
          p.innerHTML = `
            <div class="linux-sys-toolbar">
              <span class="linux-sys-sudo-chip" data-sys-sudo></span>
              <button type="button" class="btn btn-ghost btn-sm" data-net-refresh>Atualizar</button>
              ${manage ? '<button type="button" class="btn btn-ghost btn-sm" data-sys-sudo-btn>Root / sudo</button>' : ""}
            </div>
            <p class="muted linux-sys-hint">Alterações no host exigem sudo. IPs estáticos complexos (Netplan) use o módulo <strong>Discos</strong> no hub ou o terminal.</p>
            <form class="linux-sys-form" data-net-host-form>
              <label class="linux-sys-label">Hostname</label>
              <div class="linux-sys-form-row">
                <input type="text" class="panel-filter" name="hostname" value="${escapeHtml(net.hostname || "")}" ${manage ? "" : "readonly"} autocomplete="off" />
                ${manage ? '<button type="submit" class="btn btn-primary btn-sm">Aplicar</button>' : ""}
              </div>
            </form>
            <form class="linux-sys-form" data-net-dns-form>
              <label class="linux-sys-label">Servidores DNS <span class="muted">(vírgula)</span></label>
              <div class="linux-sys-form-row">
                <input type="text" class="panel-filter" name="dns" value="${escapeHtml(dnsVal)}" ${manage ? "" : "readonly"} placeholder="8.8.8.8, 8.8.4.4" autocomplete="off" />
                ${manage ? '<button type="submit" class="btn btn-primary btn-sm">Aplicar DNS</button>' : ""}
              </div>
            </form>
            <div class="linux-sys-card">
              <p class="muted">Gateway: ${escapeHtml(net.defaultRoute || "—")}</p>
            </div>
            <div class="linux-sys-scroll">
              <table class="linux-table"><thead><tr><th>Interface</th><th>Estado</th><th>IPv4</th><th>MAC</th><th>Ações</th></tr></thead>
              <tbody>${rows || '<tr><td colspan="5" class="muted">Sem interfaces</td></tr>'}</tbody></table>
            </div>`;
          paintSudoChip(p.querySelector("[data-sys-sudo]"));
          p.querySelector("[data-net-refresh]")?.addEventListener("click", () => loadNetwork());
          p.querySelector("[data-sys-sudo-btn]")?.addEventListener("click", () => void desktopToggleSudo());
          p.querySelector("[data-net-host-form]")?.addEventListener("submit", async (ev) => {
            ev.preventDefault();
            if (!manage) return;
            const host = ev.target.hostname?.value?.trim();
            if (!host) {
              CWUI.toast("Indique o hostname.", "warning");
              return;
            }
            if (!(await CWConfirm(`Alterar hostname para «${host}»?`, { danger: true }))) return;
            try {
              await api("/api/desktop/network/apply", {
                method: "POST",
                body: JSON.stringify({ hostname: host }),
              });
              CWUI.toast("Hostname atualizado.", "success");
              loadNetwork();
            } catch (e) {
              CWUI.toast(e.message, "error");
            }
          });
          p.querySelector("[data-net-dns-form]")?.addEventListener("submit", async (ev) => {
            ev.preventDefault();
            if (!manage) return;
            const dns = (ev.target.dns?.value || "")
              .split(/[,;\s]+/)
              .map((s) => s.trim())
              .filter(Boolean);
            if (!dns.length) {
              CWUI.toast("Indique pelo menos um DNS.", "warning");
              return;
            }
            if (!(await CWConfirm(`Atualizar DNS para: ${dns.join(", ")}?`, { danger: true }))) return;
            try {
              await api("/api/desktop/network/apply", {
                method: "POST",
                body: JSON.stringify({ dns }),
              });
              CWUI.toast("DNS atualizado.", "success");
              loadNetwork();
            } catch (e) {
              CWUI.toast(e.message, "error");
            }
          });
          p.querySelectorAll("[data-iface-up]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const iface = btn.dataset.ifaceUp;
              if (!(await CWConfirm(`Ligar interface ${iface}?`))) return;
              try {
                await api("/api/desktop/network/iface", {
                  method: "POST",
                  body: JSON.stringify({ iface, action: "up" }),
                });
                CWUI.toast(`${iface} ligada.`, "success");
                loadNetwork();
              } catch (e) {
                CWUI.toast(e.message, "error");
              }
            });
          });
          p.querySelectorAll("[data-iface-down]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const iface = btn.dataset.ifaceDown;
              if (!(await CWConfirm(`Desligar interface ${iface}?`, { danger: true }))) return;
              try {
                await api("/api/desktop/network/iface", {
                  method: "POST",
                  body: JSON.stringify({ iface, action: "down" }),
                });
                CWUI.toast(`${iface} desligada.`, "success");
                loadNetwork();
              } catch (e) {
                CWUI.toast(e.message, "error");
              }
            });
          });
        } catch (e) {
          p.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
        }
      }

      async function loadStorage() {
        const p = panels.storage;
        const manageDisks = canManageDisks();
        const manageHost = canManageHost();
        p.innerHTML = `<p class="muted">Carregando…</p>`;
        try {
          await syncDesktopSudo();
          const data = await api("/api/disks/probe?sort=lsblk");
          sys.rows = data.rows || [];
          sys.lvOptions = data.lvOptions || [];
          let rows = "";
          for (const r of sys.rows) {
            const pad = "&nbsp;".repeat(Math.min(6, (r.depth || 0) * 2));
            const bar =
              r.usePct != null && r.usePct >= 0
                ? `<div class="linux-meter"><div class="linux-meter-fill" style="width:${Math.min(100, r.usePct)}%"></div></div>`
                : "";
            rows += `<tr><td>${pad}${escapeHtml(r.displayName || r.devPath || "")}</td><td>${escapeHtml(r.typeLabel || "—")}</td><td>${escapeHtml(r.mount || "—")}</td><td>${bar}<span class="muted">${escapeHtml(r.usageLine || "—")}</span></td></tr>`;
          }
          const lvOpts = sys.lvOptions
            .map((o) => `<option value="${escapeHtml(o.path)}">${escapeHtml(o.label || o.path)}</option>`)
            .join("");
          p.innerHTML = `
            <div class="linux-sys-toolbar">
              <span class="linux-sys-sudo-chip" data-sys-sudo></span>
              <button type="button" class="btn btn-ghost btn-sm" data-stor-refresh>Atualizar</button>
              ${manageHost ? '<button type="button" class="btn btn-ghost btn-sm" data-sys-sudo-btn>Root / sudo</button>' : ""}
              ${canUseScreen("disks") ? '<button type="button" class="btn btn-ghost btn-sm" data-stor-hub>Discos no hub</button>' : ""}
            </div>
            <form class="linux-sys-form linux-sys-usage" data-usage-form>
              <label class="linux-sys-label">Uso por pasta no servidor</label>
              <div class="linux-sys-form-row linux-sys-form-row--wrap">
                <input type="text" class="panel-filter" name="usagePath" placeholder="/var/log" value="/var/log" autocomplete="off" />
                <button type="submit" class="btn btn-ghost btn-sm">Analisar</button>
              </div>
            </form>
            <div class="linux-sys-usage-result" data-usage-result hidden></div>
            ${
              manageDisks
                ? `<form class="linux-sys-form linux-sys-extend" data-stor-extend>
              <label class="linux-sys-label">Ampliar volume LVM</label>
              <div class="linux-sys-form-row linux-sys-form-row--wrap">
                <select class="panel-filter" name="lv">${lvOpts}<option value="">— caminho manual —</option></select>
                <input type="text" class="panel-filter" name="lvPath" placeholder="/dev/mapper/vg-lv" autocomplete="off" />
                <input type="number" class="panel-filter linux-sys-gib" name="gib" min="0.1" step="0.1" value="1" title="GiB" />
                <select class="panel-filter linux-sys-fs" name="fs"><option value="ext4">ext4</option><option value="xfs">xfs</option><option value="btrfs">btrfs</option></select>
                <button type="submit" class="btn btn-primary btn-sm">Ampliar</button>
              </div>
            </form>`
                : '<p class="muted linux-sys-hint">Sem permissão para gerir discos — apenas visualização.</p>'
            }
            <div class="linux-sys-scroll">
              <table class="linux-table"><thead><tr><th>Dispositivo</th><th>Tipo</th><th>Montagem</th><th>Uso</th></tr></thead><tbody>${rows || '<tr><td colspan="4" class="muted">Sem dispositivos</td></tr>'}</tbody></table>
            </div>`;
          paintSudoChip(p.querySelector("[data-sys-sudo]"));
          p.querySelector("[data-stor-refresh]")?.addEventListener("click", () => loadStorage());
          p.querySelector("[data-sys-sudo-btn]")?.addEventListener("click", () => void desktopToggleSudo());
          p.querySelector("[data-stor-hub]")?.addEventListener("click", () => window.CWDesktop?.openAppInHub?.("disks"));
          p.querySelector("[data-usage-form]")?.addEventListener("submit", async (ev) => {
            ev.preventDefault();
            const dir = ev.target.usagePath?.value?.trim() || "/";
            const box = p.querySelector("[data-usage-result]");
            if (!box) return;
            box.hidden = false;
            box.innerHTML = `<p class="muted">Analisando ${escapeHtml(dir)}…</p>`;
            try {
              const data = await api(`/api/disks/usage?path=${encodeURIComponent(dir)}`);
              const entries = (data.entries || []).slice(0, 12);
              if (!entries.length) {
                box.innerHTML = `<p class="muted">Nenhum item em ${escapeHtml(dir)}.</p>`;
                return;
              }
              const max = entries[0]?.sizeBytes || 1;
              const rows = entries
                .map((e) => {
                  const pct = Math.round(((e.sizeBytes || 0) / max) * 100);
                  return `<tr><td>${escapeHtml(e.name)}</td><td><div class="linux-meter"><div class="linux-meter-fill" style="width:${pct}%"></div></div></td><td class="muted">${escapeHtml(formatSize(e.sizeBytes))}</td></tr>`;
                })
                .join("");
              box.innerHTML = `<table class="linux-table linux-table--compact"><thead><tr><th>Pasta/arquivo</th><th></th><th>Tamanho</th></tr></thead><tbody>${rows}</tbody></table>`;
            } catch (e) {
              box.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
            }
          });
          p.querySelector("[data-stor-extend]")?.addEventListener("submit", async (ev) => {
            ev.preventDefault();
            await syncDesktopSudo();
            if (!state.sudo?.enabled) {
              CWUI.toast("Ative o sudo para ampliar volumes.", "warning");
              return;
            }
            const lv = ev.target.lv?.value?.trim() || ev.target.lvPath?.value?.trim();
            const gib = parseFloat(String(ev.target.gib?.value || "").replace(",", "."), 10);
            const fs = ev.target.fs?.value || "ext4";
            if (!lv) {
              CWUI.toast("Selecione ou indique o caminho do LV.", "warning");
              return;
            }
            if (!gib || gib <= 0) {
              CWUI.toast("Indique GiB positivos.", "warning");
              return;
            }
            if (!(await CWConfirm(`Ampliar ${lv} em +${gib} GiB (${fs})?`, { danger: true }))) return;
            try {
              const res = await api("/api/disks/extend-lv", {
                method: "POST",
                body: JSON.stringify({ lv, gib, fs }),
              });
              CWUI.toast("Volume ampliado.", "success");
              if (res.output) CWUI.toast(res.output.slice(0, 200), "info");
              loadStorage();
            } catch (e) {
              CWUI.toast(e.message, "error");
            }
          });
        } catch (e) {
          p.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
        }
      }

      async function loadServices() {
        const p = panels.services;
        const manage = canManageHost();
        p.innerHTML = `<p class="muted">Carregando…</p>`;
        try {
          await syncDesktopSudo();
          const data = await api("/api/desktop/services", { noAuthRedirect: true });
          const list = data.services || [];
          let rows = "";
          for (const s of list) {
            const acts = manage
              ? `<td class="linux-sys-actions">
                  <button type="button" class="btn btn-ghost btn-sm" data-svc="${escapeHtml(s.unit)}" data-svc-act="restart">Reiniciar</button>
                  <button type="button" class="btn btn-ghost btn-sm" data-svc="${escapeHtml(s.unit)}" data-svc-act="stop">Parar</button>
                </td>`
              : `<td class="muted">—</td>`;
            rows += `<tr><td><code>${escapeHtml(s.unit)}</code></td><td>${escapeHtml(s.active)}</td><td class="muted">${escapeHtml(s.description || s.sub || "")}</td>${acts}</tr>`;
          }
          p.innerHTML = `
            <div class="linux-sys-toolbar">
              <span class="linux-sys-sudo-chip" data-sys-sudo></span>
              <button type="button" class="btn btn-ghost btn-sm" data-svc-refresh>Atualizar</button>
              ${manage ? '<button type="button" class="btn btn-ghost btn-sm" data-sys-sudo-btn>Root / sudo</button>' : ""}
            </div>
            <p class="muted linux-sys-hint">${list.length} unidade(s) · reinício/paragem exige sudo e permissão.</p>
            <div class="linux-sys-scroll">
              <table class="linux-table"><thead><tr><th>Unidade</th><th>Estado</th><th>Descrição</th><th>Ações</th></tr></thead>
              <tbody>${rows || '<tr><td colspan="4" class="muted">Nenhum serviço listado</td></tr>'}</tbody></table>
            </div>`;
          paintSudoChip(p.querySelector("[data-sys-sudo]"));
          p.querySelector("[data-svc-refresh]")?.addEventListener("click", () => loadServices());
          p.querySelector("[data-sys-sudo-btn]")?.addEventListener("click", () => void desktopToggleSudo());
          p.querySelectorAll("[data-svc-act]").forEach((btn) => {
            btn.addEventListener("click", async () => {
              const unit = btn.dataset.svc;
              const action = btn.dataset.svcAct;
              const danger = action === "stop";
              if (!(await CWConfirm(`${action} ${unit}?`, { danger }))) return;
              try {
                await api("/api/desktop/service/control", {
                  method: "POST",
                  body: JSON.stringify({ unit, action }),
                });
                CWUI.toast(`${unit}: ${action}`, "success");
                loadServices();
              } catch (e) {
                CWUI.toast(e.message, "error");
              }
            });
          });
        } catch (e) {
          p.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
        }
      }

      const loaders = {
        overview: loadOverview,
        network: loadNetwork,
        storage: loadStorage,
        services: loadServices,
      };

      function showTab(tab) {
        activeTab = tab;
        body.querySelectorAll(".linux-sys-tab").forEach((b) => {
          b.classList.toggle("active", b.dataset.tab === tab);
        });
        Object.entries(panels).forEach(([k, el]) => el.classList.toggle("hidden", k !== tab));
        loaders[tab]?.();
      }

      body.querySelectorAll(".linux-sys-tab").forEach((btn) => {
        btn.addEventListener("click", () => showTab(btn.dataset.tab));
      });
      document.addEventListener("cw-sudo-changed", () => {
        if (activeTab === "network" || activeTab === "storage" || activeTab === "services") loaders[activeTab]?.();
      });
      ctx.onFocus = () => loaders[activeTab]?.();
      showTab("overview");
    },
  });

  /* —— Docker (lista + detalhes) —— */
  register({
    id: "docker",
    label: "Docker",
    icon: "🐳",
    desc: "Containers Docker",
    screen: "docker",
    w: 880,
    h: 480,
    singleton: true,
    mount(body, ctx) {
      body.className = "linux-app linux-app-docker";
      body.innerHTML = `
        <div class="linux-app-toolbar">
          <button type="button" class="btn btn-ghost btn-sm" data-act="refresh">↻ Atualizar</button>
          <input type="search" class="linux-docker-filter" data-filter placeholder="Filtrar containers…" aria-label="Filtrar" />
          <label class="linux-docker-all checkbox-row"><input type="checkbox" data-all /> Mostrar parados</label>
        </div>
        <div class="linux-docker-workspace">
          <div class="linux-docker-list linux-scroll" data-list></div>
          <aside class="linux-docker-detail linux-scroll" data-detail>
            <p class="muted linux-docker-detail-empty">Clique em um container para ver detalhes.</p>
          </aside>
        </div>`;
      const list = body.querySelector("[data-list]");
      const detail = body.querySelector("[data-detail]");
      const allChk = body.querySelector("[data-all]");
      const filterInp = body.querySelector("[data-filter]");
      let allItems = [];
      let selectedId = null;
      let detailLoadedFor = null;
      let statsPollTimer = null;
      const statsSamples = new Map();
      ctx.state = { statsSamples };

      function stopStatsPoll() {
        if (statsPollTimer) {
          clearInterval(statsPollTimer);
          statsPollTimer = null;
        }
      }

      async function pollDetailStats(id) {
        const spark = detail.querySelector("[data-docker-spark]");
        if (!spark || selectedId !== id) return;
        try {
          const data = await api(`/api/docker/stats?id=${encodeURIComponent(id)}`);
          const m = data.metrics || {};
          const cpu = m.cpuPercent ?? 0;
          const mem = m.memPercent ?? 0;
          let arr = (statsSamples.get(id) || []).slice();
          arr.push({ cpu, mem });
          if (arr.length > 28) arr = arr.slice(-28);
          statsSamples.set(id, arr);
          window.CWDesktopEnhancements?.renderDockerStats?.(spark, arr);
        } catch (_) { /* */ }
      }

      function shortText(s, max = 56) {
        const t = String(s || "");
        if (t.length <= max) return t;
        return `${t.slice(0, max - 1)}…`;
      }

      function containerId(c) {
        return c.idFull || c.id;
      }

      function containerName(c) {
        return (c.displayName || c.name || c.id || "").replace(/^\//, "");
      }

      function canBrowseFiles() {
        return canUseAction("files.read") || canUseScreen("desktop") || canUseScreen("explorer");
      }

      function canDockerShell() {
        return canUseScreen("docker") || canUseScreen("desktop");
      }

      function openFiles(id, name) {
        if (!canBrowseFiles()) {
          CWUI.toast("Sem permissão para abrir arquivos.", "error");
          return;
        }
        if (!window.CWDesktop?.openContainerFiles) {
          CWUI.toast("Reabra o Ambiente Linux.", "error");
          return;
        }
        window.CWDesktop.openContainerFiles(id, name);
      }

      function openShell(id, name) {
        if (!canDockerShell()) {
          CWUI.toast("Sem permissão para console do container.", "error");
          return;
        }
        if (!window.CWDesktop?.openContainerTerminal) {
          CWUI.toast("Reabra o Ambiente Linux.", "error");
          return;
        }
        window.CWDesktop.openContainerTerminal(id, name);
      }

      function openLogs(id, name) {
        if (!canDockerShell()) {
          CWUI.toast("Sem permissão para ver logs.", "error");
          return;
        }
        window.CWDesktopCore?.openContainerLogs?.(id, name);
      }

      function addActionBtn(parent, label, className, onClick) {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = className || "btn btn-ghost btn-sm";
        btn.textContent = label;
        btn.addEventListener("click", (ev) => {
          ev.preventDefault();
          ev.stopPropagation();
          onClick();
        });
        parent.appendChild(btn);
        return btn;
      }

      function appendLifecycleActions(actions, c, id, running) {
        if (!canUseAction("docker.control")) return;
        if (running) {
          addActionBtn(actions, "Parar", "btn btn-ghost btn-sm", async () => {
            try {
              await api("/api/docker/stop", { method: "POST", body: JSON.stringify({ id }) });
              CWUI.toast("Container parado", "success");
              load();
            } catch (e) {
              CWUI.toast(e.message, "error");
            }
          });
          addActionBtn(actions, "Reiniciar", "btn btn-ghost btn-sm", async () => {
            try {
              await api("/api/docker/restart", { method: "POST", body: JSON.stringify({ id }) });
              CWUI.toast("Container reiniciado", "success");
              load();
            } catch (e) {
              CWUI.toast(e.message, "error");
            }
          });
        } else {
          addActionBtn(actions, "Iniciar", "btn btn-primary btn-sm", async () => {
            try {
              await api("/api/docker/start", { method: "POST", body: JSON.stringify({ id }) });
              CWUI.toast("Container iniciado", "success");
              load();
            } catch (e) {
              CWUI.toast(e.message, "error");
            }
          });
        }
      }

      async function showContainerDetail(c, card, force) {
        const id = containerId(c);
        const name = containerName(c);
        const running = !!(c.running || c.state === "running");
        selectedId = id;
        stopStatsPoll();
        list.querySelectorAll(".linux-docker-card").forEach((el) => el.classList.remove("is-selected"));
        card?.classList.add("is-selected");
        if (!force && detailLoadedFor === id && detail.querySelector(".linux-docker-detail-head")) {
          return;
        }
        detailLoadedFor = id;
        detail.innerHTML = `<p class="muted">Carregando detalhes…</p>`;
        const cpu = c.metrics?.cpuPercent;
        const mem = c.metrics?.memPercent;
        const metrics =
          cpu != null ? `CPU ${cpu.toFixed(0)}% · RAM ${(mem ?? 0).toFixed(0)}%` : "—";
        let inspectHtml = "";
        try {
          const data = await api(`/api/docker/inspect?id=${encodeURIComponent(id)}`);
          const ins = data.inspect || {};
          const labels = ins.labels || {};
          const labelKeys = Object.keys(labels).sort().slice(0, 8);
          const labelsHtml = labelKeys.length
            ? `<ul class="linux-docker-labels">${labelKeys
                .map((k) => `<li><code>${escapeHtml(k)}</code> ${escapeHtml(labels[k])}</li>`)
                .join("")}</ul>`
            : "";
          inspectHtml = `
            <dl class="linux-docker-dl">
              <dt>ID</dt><dd><code>${escapeHtml(ins.id || id)}</code></dd>
              <dt>Imagem</dt><dd title="${escapeHtml(ins.image || c.image || "")}">${escapeHtml(shortText(ins.image || c.image || "—", 72))}</dd>
              <dt>Hostname</dt><dd>${escapeHtml(ins.hostname || "—")}</dd>
              <dt>Reinícios</dt><dd>${escapeHtml(String(ins.restartCount ?? "—"))}</dd>
              <dt>Política</dt><dd>${escapeHtml(ins.restartPolicy || "—")}</dd>
              <dt>Iniciado</dt><dd>${escapeHtml(ins.startedAt || "—")}</dd>
            </dl>
            ${labelsHtml}`;
        } catch (e) {
          inspectHtml = `<p class="error">${escapeHtml(e.message)}</p>`;
        }
        detail.innerHTML = `
          <header class="linux-docker-detail-head">
            <h4>${escapeHtml(name)}</h4>
            <span class="linux-docker-state">${escapeHtml(c.state || "—")}</span>
          </header>
          <p class="muted">${escapeHtml(metrics)}</p>
          <div class="linux-docker-spark" data-docker-spark aria-label="Histórico CPU/RAM"></div>
          <p class="muted linux-docker-detail-image" title="${escapeHtml(c.image || "")}">${escapeHtml(shortText(c.image || "", 80))}</p>
          ${c.ports ? `<p class="muted linux-docker-detail-ports"><strong>Portas:</strong> ${escapeHtml(c.ports)}</p>` : ""}
          <div class="linux-docker-detail-actions" data-detail-actions></div>
          <div class="linux-docker-detail-body">${inspectHtml}</div>`;
        const actions = detail.querySelector("[data-detail-actions]");
        if (running) {
          if (canBrowseFiles()) addActionBtn(actions, "Arquivos", "btn btn-primary btn-sm", () => openFiles(id, name));
          if (canDockerShell()) {
            addActionBtn(actions, "Console", "btn btn-ghost btn-sm", () => openShell(id, name));
            addActionBtn(actions, "Logs", "btn btn-ghost btn-sm", () => openLogs(id, name));
          }
        }
        if (canUseAction("automations.manage")) {
          addActionBtn(actions, "Regra auto", "btn btn-ghost btn-sm", () =>
            window.CWDesktopEnhancements?.createDockerAutoRule?.(name)
          );
        }
        appendLifecycleActions(actions, c, id, running);
        if (running) {
          void pollDetailStats(id);
          statsPollTimer = setInterval(() => pollDetailStats(id), 3000);
        } else {
          const spark = detail.querySelector("[data-docker-spark]");
          window.CWDesktopEnhancements?.renderDockerStats?.(spark, statsSamples.get(id) || []);
        }
      }

      const load = async () => {
        detailLoadedFor = null;
        list.innerHTML = `<p class="muted">Carregando…</p>`;
        try {
          const q = allChk?.checked ? "?all=1&metrics=1" : "?metrics=1";
          const data = await api(`/api/docker/containers${q}`);
          allItems = data.containers || [];
          renderDockerList();
        } catch (e) {
          list.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
        }
      };

      function renderDockerList() {
        const q = (filterInp?.value || "").trim().toLowerCase();
        const items = allItems.filter((c) => {
          if (!q) return true;
          const name = (c.displayName || c.name || c.id || "").toLowerCase();
          return name.includes(q) || (c.image || "").toLowerCase().includes(q);
        });
        if (!items.length) {
          list.innerHTML = `<p class="muted">Nenhum container${q ? " (filtro)" : ""}.</p>`;
          detail.innerHTML = `<p class="muted linux-docker-detail-empty">Clique em um container para ver detalhes.</p>`;
          selectedId = null;
          return;
        }
        list.innerHTML = "";
        let selectCard = null;
        let selectItem = items[0];
        for (const c of items) {
          const id = containerId(c);
          const name = containerName(c);
          const running = !!(c.running || c.state === "running");
          const card = document.createElement("div");
          card.className = "linux-docker-card" + (running ? " is-running" : "");
          card.dataset.containerId = id;
          if (id === selectedId) {
            card.classList.add("is-selected");
            selectCard = card;
            selectItem = c;
          }
          const cpu = c.metrics?.cpuPercent;
          const mem = c.metrics?.memPercent;
          const metrics =
            cpu != null ? `CPU ${cpu.toFixed(0)}% · RAM ${(mem ?? 0).toFixed(0)}%` : "";
          card.innerHTML = `
            <div class="linux-docker-card-head">
              <strong>${escapeHtml(name)}</strong>
              <span class="linux-docker-state">${escapeHtml(c.state || "—")}</span>
            </div>
            <p class="muted">${escapeHtml(metrics)}</p>
            <p class="muted linux-docker-card-image" title="${escapeHtml(c.image || "")}">${escapeHtml(shortText(c.image || "", 64))}</p>
            <div class="linux-docker-actions"></div>`;
          const actions = card.querySelector(".linux-docker-actions");
          if (running) {
            if (canBrowseFiles()) addActionBtn(actions, "Arquivos", "btn btn-primary btn-sm", () => openFiles(id, name));
            if (canDockerShell()) {
              addActionBtn(actions, "Console", "btn btn-ghost btn-sm", () => openShell(id, name));
              addActionBtn(actions, "Logs", "btn btn-ghost btn-sm", () => openLogs(id, name));
            }
          }
          appendLifecycleActions(actions, c, id, running);
          card.addEventListener("click", (ev) => {
            if (ev.target.closest("button")) return;
            void showContainerDetail(c, card, true);
          });
          list.appendChild(card);
        }
        const pick = selectCard || list.querySelector(".linux-docker-card");
        const pickItem = selectItem || items[0];
        if (pick && pickItem) void showContainerDetail(pickItem, pick, !selectCard);
      }

      body.querySelector("[data-act=refresh]")?.addEventListener("click", (ev) => {
        ev.preventDefault();
        ev.stopPropagation();
        load();
      });
      allChk?.addEventListener("change", load);
      filterInp?.addEventListener("input", renderDockerList);
      ctx.onFocus = null;
      load();
    },
  });

  /* —— Automações —— */
  register({
    id: "automations",
    label: "Automações",
    icon: "⚙️",
    desc: "Regras e histórico",
    screen: "automations",
    w: 640,
    h: 440,
    singleton: true,
    mount(body, ctx) {
      body.className = "linux-app linux-app-auto";
      body.innerHTML = `
        <div class="linux-app-toolbar linux-auto-toolbar">
          <span class="badge muted" data-engine>Motor</span>
          <button type="button" class="btn btn-ghost btn-sm" data-toggle>Iniciar motor</button>
          <button type="button" class="btn btn-primary btn-sm" data-save hidden>Salvar regras</button>
          <button type="button" class="btn btn-ghost btn-sm" data-clear-hist>Limpar histórico</button>
          <button type="button" class="btn btn-ghost btn-sm" data-refresh title="Atualizar">↻</button>
        </div>
        <p class="muted linux-auto-hint">O motor verifica o Docker a cada 8s. Regras valem para este host.</p>
        <div class="linux-auto-split">
          <section class="linux-auto-panel">
            <h4 class="linux-auto-panel-title">Regras</h4>
            <div class="linux-auto-rules linux-scroll" data-rules></div>
          </section>
          <section class="linux-auto-panel">
            <h4 class="linux-auto-panel-title">Histórico</h4>
            <pre class="linux-auto-log linux-scroll" data-log></pre>
          </section>
        </div>
        <dialog class="linux-auto-dialog" data-rule-dialog>
          <form method="dialog" class="linux-auto-form" data-rule-form>
            <h3 data-rule-title>Editar regra</h3>
            <label class="field">Nome <input name="name" required /></label>
            <label class="field">Descrição <input name="description" /></label>
            <label class="field">Gatilho <input name="trigger" /></label>
            <label class="field">Ação <input name="action" /></label>
            <label class="field">Alvo (container) <input name="target" placeholder="ex: nginx" /></label>
            <label class="field">Cooldown (s) <input name="cooldownSec" type="number" min="10" value="60" /></label>
            <label class="field">Webhook <input name="webhookURL" type="url" placeholder="https://…" /></label>
            <label class="checkbox-row"><input name="enabled" type="checkbox" checked /> Regra ativa</label>
            <div class="btn-row end">
              <button type="button" class="btn btn-ghost btn-sm" data-rule-cancel>Cancelar</button>
              <button type="submit" class="btn btn-primary btn-sm">Aplicar</button>
            </div>
          </form>
        </dialog>`;
      const eng = body.querySelector("[data-engine]");
      const rulesBox = body.querySelector("[data-rules]");
      const log = body.querySelector("[data-log]");
      const saveBtn = body.querySelector("[data-save]");
      const dlg = body.querySelector("[data-rule-dialog]");
      const form = body.querySelector("[data-rule-form]");
      let rules = [];
      let rulesDirty = false;
      let editIdx = -1;

      function setDirty(on) {
        rulesDirty = on;
        if (saveBtn) saveBtn.hidden = !on;
      }

      function paintEngine(running) {
        eng.textContent = running ? "Motor ativo" : "Motor parado";
        eng.className = running ? "badge on" : "badge muted";
        body.querySelector("[data-toggle]").textContent = running ? "Parar motor" : "Iniciar motor";
      }

      function openRuleDialog(idx) {
        const r = rules[idx];
        if (!r) return;
        editIdx = idx;
        body.querySelector("[data-rule-title]").textContent = `Editar: ${r.name || r.id}`;
        form.elements.name.value = r.name || "";
        form.elements.description.value = r.description || "";
        form.elements.trigger.value = r.trigger || "";
        form.elements.action.value = r.action || "";
        form.elements.target.value = r.target || "";
        form.elements.cooldownSec.value = r.cooldownSec ?? 60;
        form.elements.webhookURL.value = r.webhookURL || "";
        form.elements.enabled.checked = !!r.enabled;
        dlg?.showModal();
      }

      function renderRules() {
        rulesBox.innerHTML = "";
        if (!rules.length) {
          rulesBox.innerHTML = `<p class="muted linux-app-empty">Nenhuma regra. Crie uma no módulo Docker (reinício automático) ou recarregue.</p>`;
          return;
        }
        for (let i = 0; i < rules.length; i++) {
          const r = rules[i];
          const row = document.createElement("div");
          row.className = "linux-auto-rule";
          const editable = r.kind === "docker_container_stopped_restart";
          row.innerHTML = `
            <div class="linux-auto-rule-main">
              <strong>${escapeHtml(r.name || r.id)}</strong>
              <span class="muted">${escapeHtml(r.trigger || "")} → ${escapeHtml(r.action || "")}</span>
              <span class="muted">${r.enabled ? "Ativa" : "Inativa"} · ${escapeHtml(r.target || "—")}</span>
            </div>`;
          const actions = document.createElement("div");
          actions.className = "linux-auto-rule-actions";
          if (editable && canUseAction("automations.manage")) {
            const edit = document.createElement("button");
            edit.type = "button";
            edit.className = "btn btn-ghost btn-sm";
            edit.textContent = "Editar";
            edit.addEventListener("click", () => openRuleDialog(i));
            actions.appendChild(edit);
            const toggle = document.createElement("button");
            toggle.type = "button";
            toggle.className = "btn btn-ghost btn-sm";
            toggle.textContent = r.enabled ? "Desativar" : "Ativar";
            toggle.addEventListener("click", () => {
              rules[i] = { ...r, enabled: !r.enabled };
              setDirty(true);
              renderRules();
            });
            actions.appendChild(toggle);
          } else {
            const tag = document.createElement("span");
            tag.className = "muted";
            tag.textContent = editable ? "" : "Somente leitura";
            actions.appendChild(tag);
          }
          row.appendChild(actions);
          rulesBox.appendChild(row);
        }
      }

      async function loadAll() {
        try {
          const [st, rulesRes, hist] = await Promise.all([
            api("/api/automations/engine"),
            api("/api/automations/rules"),
            api("/api/automations/history"),
          ]);
          paintEngine(!!st.running);
          rules = rulesRes.rules || [];
          if (typeof state !== "undefined") {
            state.autoRules = rules;
            state.autoRulesDirty = false;
          }
          setDirty(false);
          renderRules();
          window.CWDesktopEnhancements?.applyDockerAutomationPrefill?.(
            rules,
            setDirty,
            renderRules,
            openRuleDialog
          );
          log.textContent = (hist.lines || []).join("\n") || "(histórico vazio)";
        } catch (e) {
          rulesBox.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
          log.textContent = "";
        }
      }

      body.querySelector("[data-refresh]")?.addEventListener("click", (ev) => {
        ev.stopPropagation();
        loadAll();
      });
      body.querySelector("[data-toggle]")?.addEventListener("click", async (ev) => {
        ev.stopPropagation();
        if (!canUseAction("automations.manage")) {
          CWUI.toast("Sem permissão para controlar o motor.", "error");
          return;
        }
        try {
          const st = await api("/api/automations/engine");
          const action = st.running ? "stop" : "start";
          await api("/api/automations/engine", {
            method: "POST",
            body: JSON.stringify({ action }),
          });
          CWUI.toast(action === "start" ? "Motor iniciado" : "Motor parado", "success");
          await loadAll();
        } catch (e) {
          CWUI.toast(e.message, "error");
        }
      });
      saveBtn?.addEventListener("click", async (ev) => {
        ev.stopPropagation();
        if (!canUseAction("automations.manage")) {
          CWUI.toast("Sem permissão.", "error");
          return;
        }
        try {
          await api("/api/automations/rules", {
            method: "PUT",
            body: JSON.stringify({ rules }),
          });
          setDirty(false);
          if (typeof state !== "undefined") state.autoRulesDirty = false;
          CWUI.toast("Regras salvas", "success");
        } catch (e) {
          CWUI.toast(e.message, "error");
        }
      });
      body.querySelector("[data-clear-hist]")?.addEventListener("click", async (ev) => {
        ev.stopPropagation();
        if (!canUseAction("automations.manage")) {
          CWUI.toast("Sem permissão.", "error");
          return;
        }
        if (!(await CWConfirm("Limpar histórico deste host?"))) return;
        try {
          await api("/api/automations/history", { method: "DELETE" });
          CWUI.toast("Histórico limpo", "success");
          const hist = await api("/api/automations/history");
          log.textContent = (hist.lines || []).join("\n") || "(histórico vazio)";
        } catch (e) {
          CWUI.toast(e.message, "error");
        }
      });
      body.querySelector("[data-rule-cancel]")?.addEventListener("click", () => dlg?.close());
      form?.addEventListener("submit", (ev) => {
        ev.preventDefault();
        if (editIdx < 0) return;
        const r = { ...rules[editIdx] };
        r.name = form.elements.name.value.trim();
        r.description = form.elements.description.value.trim();
        r.trigger = form.elements.trigger.value.trim();
        r.action = form.elements.action.value.trim();
        r.target = form.elements.target.value.trim();
        r.cooldownSec = Math.max(10, parseInt(form.elements.cooldownSec.value, 10) || 60);
        r.webhookURL = form.elements.webhookURL.value.trim();
        r.enabled = form.elements.enabled.checked;
        rules[editIdx] = r;
        setDirty(true);
        dlg?.close();
        renderRules();
      });

      ctx.onFocus = null;
      loadAll();
      ctx.state = { timer: setInterval(loadAll, 12000) };
    },
    unmount(ctx) {
      if (ctx.state?.timer) clearInterval(ctx.state.timer);
    },
  });

  /* —— Editor (texto + imagens) —— */
  register({
    id: "editor",
    label: "Editor",
    icon: "📝",
    desc: "Editar arquivo",
    hidden: true,
    screen: "explorer",
    w: 620,
    h: 460,
    singleton: false,
    mount(body, ctx) {
      const path = ctx.opts?.path || "/";
      const name = ctx.opts?.name || "arquivo";
      const containerId = ctx.opts?.containerId || "";
      body.className = "linux-app linux-app-editor";
      body.innerHTML = `
        <div class="linux-app-toolbar">
          <span class="linux-editor-name">${escapeHtml(name)}</span>
          <button type="button" class="btn btn-primary btn-sm" data-save hidden>Salvar</button>
        </div>
        <p class="muted linux-editor-path">${escapeHtml(containerId ? `🐳 ${ctx.opts?.containerName || "contêiner"}:${path}` : path)}</p>
        <div class="linux-editor-view">
          <div class="linux-editor-image-wrap hidden">
            <img class="linux-editor-img" alt="" decoding="async" />
          </div>
          <textarea class="linux-editor-ta hidden" spellcheck="false"></textarea>
          <p class="linux-editor-hint hidden muted"></p>
        </div>`;
      const ta = body.querySelector(".linux-editor-ta");
      const imgWrap = body.querySelector(".linux-editor-image-wrap");
      const img = body.querySelector(".linux-editor-img");
      const hint = body.querySelector(".linux-editor-hint");
      const saveBtn = body.querySelector("[data-save]");

      function setEditorMode(mode) {
        if (mode === "image") {
          ta.classList.add("hidden");
          imgWrap.classList.remove("hidden");
          hint.classList.add("hidden");
          saveBtn.hidden = true;
        } else if (mode === "text") {
          ta.classList.remove("hidden");
          imgWrap.classList.add("hidden");
          hint.classList.add("hidden");
          saveBtn.hidden = false;
          img.removeAttribute("src");
        } else {
          ta.classList.add("hidden");
          imgWrap.classList.add("hidden");
          hint.classList.remove("hidden");
          saveBtn.hidden = true;
          hint.textContent =
            "Arquivo binário ou não suportado aqui. Ative Root na janela Arquivos ou use o gerenciador técnico.";
        }
      }

      async function loadFile() {
        ta.value = "Carregando…";
        ta.disabled = true;
        ta.classList.remove("hidden");
        imgWrap.classList.add("hidden");
        hint.classList.add("hidden");
        saveBtn.hidden = true;
        try {
          const cq = containerId ? `&containerId=${encodeURIComponent(containerId)}` : "";
          const data = await api(`/api/remote/file?path=${encodeURIComponent(path)}${cq}`);
          const mime = data.mimeType || "";
          if (data.encoding === "base64" && mime.startsWith("image/")) {
            img.src = `data:${mime};base64,${data.content}`;
            img.alt = name;
            setEditorMode("image");
            return;
          }
          if (data.encoding === "base64") {
            setEditorMode("binary");
            return;
          }
          ta.value = data.content ?? "";
          ta.disabled = false;
          setEditorMode("text");
          ta.focus();
        } catch (e) {
          setEditorMode("binary");
          hint.classList.remove("hidden");
          hint.textContent = e.message || "Erro ao abrir arquivo.";
          if (/permissão negada|sudo/i.test(hint.textContent)) {
            hint.textContent += " Ative o botão Root na janela Arquivos e abra de novo.";
          }
        }
      }

      const onSudoChanged = () => loadFile();
      document.addEventListener("cw-sudo-changed", onSudoChanged);

      ctx.state = {
        path,
        name,
        ta,
        dirty: false,
        cleanup: () => {
          document.removeEventListener("cw-sudo-changed", onSudoChanged);
          img.removeAttribute("src");
        },
      };
      ta.addEventListener("input", () => {
        ctx.state.dirty = true;
      });
      loadFile();

      saveBtn.addEventListener("click", async () => {
        if (!canUseAction("files.write")) {
          CWUI.toast("Sem permissão para gravar.", "error");
          return;
        }
        try {
          await api("/api/remote/file", {
            method: "PUT",
            body: JSON.stringify(
              containerId ? { path, content: ta.value, containerId } : { path, content: ta.value }
            ),
          });
          ctx.state.dirty = false;
          CWUI.toast("Gravado", "success");
        } catch (e) {
          CWUI.toast(e.message, "error");
        }
      });
    },
    unmount(ctx) {
      ctx.state?.cleanup?.();
    },
  });

  /* —— Ajuda —— */
  register({
    id: "help",
    label: "Ajuda",
    icon: "❓",
    desc: "Guia rápido",
    screen: "desktop",
    w: 420,
    h: 320,
    singleton: true,
    mount(body) {
      body.className = "linux-app linux-app-help";
      body.innerHTML = `
        <ul class="linux-help-list">
          <li><strong>Arquivos</strong> — servidor ou Docker; ↑ PC / soltar arquivos para enviar; renomear e pré-visualizar.</li>
          <li><strong>Sistema</strong> — rede (hostname, DNS, interfaces), LVM e serviços (com sudo).</li>
          <li><strong>Docker</strong> — arquivos, console e logs por container; clique no card para detalhes.</li>
          <li><strong>Automações</strong> — ligue o motor, edite regras de reinício e veja o histórico.</li>
          <li>Atalhos: <kbd>Alt</kbd>+<kbd>Tab</kbd> janelas, <kbd>Ctrl</kbd>+<kbd>W</kbd> fechar, <kbd>Ctrl</kbd>+<kbd>M</kbd> minimizar tudo, <kbd>Ctrl</kbd>+<kbd>K</kbd> lista de comandos (terminal), <kbd>?</kbd> esta ajuda.</li>
          <li>Arraste a janela ao <strong>topo</strong> para maximizar; às <strong>bordas</strong> para metade da tela; duplo-clique na barra de título alterna maximizar.</li>
          <li>Menu <strong>CW</strong>: pesquisa, recentes e papel de fundo. Botão <strong>⌂</strong> na janela abre o módulo técnico.</li>
        </ul>
        <p class="muted">O Ubuntu Server continua sem interface gráfica instalada.</p>`;
    },
  });

  window.CWDesktopApps = {
    all() {
      return [...apps.values()].filter((a) => !a.hidden).map(localizedApp);
    },
    get(id) {
      const a = apps.get(id);
      return a ? localizedApp(a) : undefined;
    },
    visible() {
      return this.all().filter((a) => canUseApp(a));
    },
    canUseApp,
    formatBytes,
    pctBar,
    toggleSudo: desktopToggleSudo,
    syncSudo: syncDesktopSudo,
  };
})();
