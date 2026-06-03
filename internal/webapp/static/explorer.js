/** Explorador dual — paridade com desktop (operações, favoritos, contêineres, comparar). */
(function () {
  Object.assign(state, {
    remoteTarget: "host",
    remoteContainerId: "",
    localFilter: "",
    remoteFilter: "",
    localEntries: [],
    remoteEntries: [],
    ctxSide: null,
    ctxEntry: null,
    sudo: { enabled: false, user: "" },
    fileEdit: null,
  });

  function remoteSource() {
    return state.remoteTarget === "container" ? "container" : "host";
  }

  function remoteApiExtra() {
    const cid = state.remoteTarget === "container" ? state.remoteContainerId : "";
    return cid ? `&containerId=${encodeURIComponent(cid)}` : "";
  }

  function updateSudoButton() {
    const btn = $("#btn-sudo");
    if (!btn) return;
    const hostMode = remoteSource() === "host";
    const on = !!state.sudo?.enabled;
    btn.disabled = !state.ssh.connected || !hostMode;
    btn.classList.toggle("is-active", on && hostMode);
    if (!hostMode) {
      btn.textContent = "—";
      btn.title = "Sudo só está disponível no modo Host SFTP";
      return;
    }
    const user = state.sudo.user || "root";
    btn.title = on ? `Sudo ativo (${user}) — clique para desativar` : "Ativar sudo/root no host";
    btn.textContent = on ? user : "Sudo";
  }

  window.refreshSudoUI = async function refreshSudoUI() {
    if (!state.ssh.connected) {
      state.sudo = { enabled: false, user: "" };
      updateSudoButton();
      return;
    }
    try {
      const st = await api("/api/ssh/sudo", { noAuthRedirect: true });
      state.sudo = { enabled: !!st.enabled, user: st.user || "" };
    } catch {
      state.sudo = { enabled: false, user: "" };
    }
    updateSudoButton();
  };

  function filterEntries(entries, q) {
    if (typeof window.cwPanelFilter === "function") return window.cwPanelFilter(entries, q);
    const hay = (q || "").trim().toLowerCase();
    if (!hay) return entries || [];
    return (entries || []).filter((e) => e.name === ".." || e.name.toLowerCase().includes(hay));
  }

  function normExplorerPath(p) {
    const s = String(p || "").replace(/\\/g, "/").replace(/\/+$/, "");
    return s || "/";
  }

  function clearPanelFilter(side) {
    if (side === "local") {
      state.localFilter = "";
      const el = $("#local-filter");
      if (el) el.value = "";
    } else {
      state.remoteFilter = "";
      const el = $("#remote-filter");
      if (el) el.value = "";
    }
  }

  window.renderEntries = function renderEntries(container, entries, side) {
    container.classList.remove("skeleton-host");
    container.innerHTML = "";
    const q = side === "local" ? state.localFilter : state.remoteFilter;
    const list = filterEntries(entries, q);
    if (side === "local") state.localEntries = entries || [];
    else state.remoteEntries = entries || [];
    if (!list.length) {
      CWUI.emptyState(container, {
        icon: "📂",
        title: q ? "Sem resultados" : "Pasta vazia",
        desc: q
          ? (window.CWI18n?.t("explorer.empty.filter") || "Nenhum item corresponde ao filtro.")
          : (side === "local"
            ? (window.CWI18n?.t("explorer.empty.local") || "Não há arquivos nesta pasta local.")
            : (window.CWI18n?.t("explorer.empty.remote") || "Não há arquivos neste diretório.")),
      });
      return;
    }
    for (const e of list) {
      const row = document.createElement("div");
      row.className = "file-row" + (e.isDir ? " dir" : "");
      row.dataset.path = e.path;
      row.dataset.isDir = e.isDir ? "1" : "0";
      row.dataset.name = e.name;
      const icon = CWUI.fileIcon(e.name, e.isDir);
      row.innerHTML = `<span class="icon">${icon}</span><span class="name">${escapeHtml(e.name)}</span><span class="meta">${e.isDir ? "" : formatSize(e.size)}</span>`;
      row.addEventListener("click", () => {
        container.querySelectorAll(".file-row").forEach((r) => r.classList.remove("selected"));
        row.classList.add("selected");
        if (side === "local") state.selLocal = e;
        else state.selRemote = e;
      });
      row.addEventListener("dblclick", () => {
        if (e.isDir) {
          if (side === "local") loadLocal(e.path);
          else loadRemote(e.path);
        } else if (e.name !== "..") {
          openFileEditor(side, e);
        }
      });
      row.addEventListener("contextmenu", (ev) => {
        ev.preventDefault();
        row.click();
        showCtxMenu(ev, side, e);
      });
      container.appendChild(row);
    }
  };

  window.loadLocal = async function loadLocal(path) {
    const nextPath = normExplorerPath(path);
    if (nextPath !== normExplorerPath(state.localPath)) clearPanelFilter("local");
    CWUI.skeletonList($("#local-list"), 8);
    const q = path ? `?path=${encodeURIComponent(path)}` : "";
    const data = await api("/api/local/list" + q);
    state.localPath = data.path;
    state.selLocal = null;
    renderEntries($("#local-list"), data.entries, "local");
    if (typeof updateExplorerBreadcrumbs === "function") updateExplorerBreadcrumbs();
  };

  window.openRemoteExplorer = function openRemoteExplorer(remotePath) {
    setRemoteTarget("host");
    showScreen("explorer");
    void window.loadRemote(remotePath || "/");
  };

  window.loadRemote = async function loadRemote(path) {
    if (!state.ssh.connected) return;
    if (state.remoteTarget === "container" && !state.remoteContainerId) {
      CWUI.toast("Selecione um contêiner.", "error");
      return;
    }
    const nextPath = normExplorerPath(path || "/");
    if (nextPath !== normExplorerPath(state.remotePath)) clearPanelFilter("remote");
    const listEl = $("#remote-list");
    CWUI.skeletonList(listEl, 8);
    try {
      const data = await api(`/api/remote/list?path=${encodeURIComponent(path || "/")}${remoteApiExtra()}`);
      state.remotePath = data.path;
      state.selRemote = null;
      renderEntries(listEl, data.entries, "remote");
      if (typeof updateExplorerBreadcrumbs === "function") updateExplorerBreadcrumbs();
    } catch (e) {
      state.remoteEntries = [];
      const msg = e.message || "Erro ao ler o diretório remoto.";
      CWUI.emptyState(listEl, {
        icon: "⚠️",
        title: "Não foi possível listar",
        desc: msg,
      });
      CWUI.toast(msg, "error");
      if (typeof window.showRemotePermissionAlert === "function") {
        window.showRemotePermissionAlert(msg);
      }
    }
  };

  function shortContainerLabel(c) {
    const name = (c.displayName || c.name || c.id || "").replace(/^\//, "");
    const short = name.length > 18 ? name.slice(0, 16) + "…" : name;
    return short;
  }

  window.openDockerContainerInExplorer = function (c) {
    const id = c.idFull || c.id;
    if (!id) return;
    showScreen("explorer");
    setRemoteTarget("container");
    const sel = $("#remote-container");
    const trySelect = () => {
      if (!sel) return;
      for (const opt of sel.options) {
        if (opt.value === id) {
          sel.value = id;
          state.remoteContainerId = id;
          loadRemote("/");
          return;
        }
      }
      loadContainersSelect().then(() => {
        for (const opt of sel.options) {
          if (opt.value === id) {
            sel.value = id;
            state.remoteContainerId = id;
            loadRemote("/");
            return;
          }
        }
      });
    };
    trySelect();
  };

  async function loadContainersSelect() {
    const sel = $("#remote-container");
    if (!sel) return;
    sel.innerHTML = "";
    const ph = document.createElement("option");
    ph.value = "";
    ph.textContent = "Selecione um contêiner…";
    sel.appendChild(ph);
    try {
      const data = await api("/api/docker/containers");
      const list = data.containers || [];
      if (typeof window.onExplorerContainersLoaded === "function") {
        window.onExplorerContainersLoaded(list);
      }
      if (!list.length) {
        ph.textContent = "Nenhum contêiner em execução";
        return;
      }
      for (const c of list) {
        const opt = document.createElement("option");
        opt.value = c.idFull || c.id;
        opt.textContent = shortContainerLabel(c);
        opt.title = `${c.name || c.id} — ${c.image}`;
        sel.appendChild(opt);
      }
      if (sel.options.length > 1) {
        sel.selectedIndex = 1;
        state.remoteContainerId = sel.value;
      }
    } catch {
      ph.textContent = "Docker indisponível";
    }
    CWUI.refreshSelect?.(sel);
  }

  function fillPanelMenu(listEl, items, onPick) {
    if (!listEl) return;
    listEl.innerHTML = "";
    if (!items.length) {
      listEl.innerHTML = '<span class="panel-menu-empty">Nenhum atalho</span>';
      return;
    }
    for (const it of items) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "panel-menu-item";
      btn.textContent = it.label;
      btn.title = it.path;
      btn.addEventListener("click", () => {
        onPick(it.path);
        window.CWUI?.closeAllFloatMenus?.();
      });
      listEl.appendChild(btn);
    }
  }

  async function refreshFavorites() {
    const leftItems = [];
    const rightItems = [];
    try {
      const sc = await api("/api/local/shortcuts");
      for (const s of sc.shortcuts || []) {
        leftItems.push({ label: s.label, path: s.path });
      }
    } catch { /* ignore */ }
    try {
      const leftFav = await api("/api/explorer/favorites?side=left");
      for (const p of leftFav.paths || []) {
        leftItems.push({ label: p.length > 36 ? "…" + p.slice(-34) : p, path: p });
      }
    } catch { /* ignore */ }
    try {
      const rightFav = await api("/api/explorer/favorites?side=right");
      for (const p of rightFav.paths || []) {
        rightItems.push({ label: p.length > 36 ? "…" + p.slice(-34) : p, path: p });
      }
    } catch { /* ignore */ }
    fillPanelMenu($("#local-menu-list"), leftItems, (p) => loadLocal(p));
    fillPanelMenu($("#remote-menu-list"), rightItems, (p) => loadRemote(p));
  }

  function resetRemoteToRoot() {
    state.remotePath = "/";
    state.selRemote = null;
    clearPanelFilter("remote");
  }

  function setRemoteTarget(mode) {
    const prev = state.remoteTarget;
    state.remoteTarget = mode;
    if (prev !== mode) {
      resetRemoteToRoot();
    }
    $$(".remote-target-btn").forEach((b) => {
      b.classList.toggle("active", b.dataset.target === mode);
    });
    const showC = mode === "container";
    $("#remote-container")?.classList.toggle("hidden", !showC);
    $("#remote-container-filter")?.classList.toggle("hidden", !showC);
    if (!state.ssh.connected) return;
    if (showC) {
      loadContainersSelect().then(() => {
        if (state.remoteContainerId) loadRemote("/");
      });
    } else {
      state.remoteContainerId = "";
      loadRemote("/");
    }
    updateSudoButton();
    if (typeof window.updateExplorerBreadcrumbs === "function") window.updateExplorerBreadcrumbs();
  }

  async function addFavorite(side) {
    const path = side === "local" ? state.localPath : state.remotePath;
    if (!path) return;
    const key = side === "local" ? "left" : "right";
    const data = await api(`/api/explorer/favorites?side=${key}`);
    const paths = data.paths || [];
    if (!paths.includes(path)) paths.push(path);
    await api(`/api/explorer/favorites?side=${key}`, { method: "PUT", body: JSON.stringify({ paths }) });
    await refreshFavorites();
    CWUI.toast("Adicionado aos favoritos", "success");
  }

  function selectedEntry(side) {
    return side === "local" ? state.selLocal : state.selRemote;
  }

  async function mkdirActive(side) {
    const name = await CWUI.nameInputDialog({ title: "Nova pasta", label: "Nome da pasta:" });
    if (!name) return;
    const base = side === "local" ? state.localPath : state.remotePath;
    const sep = side === "local" ? (base.includes("\\") ? "\\" : "/") : "/";
    const path = base.replace(/[/\\]+$/, "") + sep + name.trim();
    if (side === "local") {
      await api("/api/local/mkdir", { method: "POST", body: JSON.stringify({ path }) });
      await loadLocal(state.localPath);
    } else {
      await api("/api/remote/mkdir", {
        method: "POST",
        body: JSON.stringify({ path, containerId: state.remoteContainerId || undefined }),
      });
      await loadRemote(state.remotePath);
    }
    CWUI.toast("Pasta criada", "success");
  }

  async function renameActive(side) {
    const sel = selectedEntry(side);
    if (!sel || sel.name === "..") {
      CWUI.toast("Selecione um item.", "error");
      return;
    }
    const name = await CWUI.nameInputDialog({ title: "Renomear", label: "Novo nome:", value: sel.name });
    if (!name || name === sel.name) return;
    const dir = sel.path.replace(/[/\\][^/\\]+$/, "");
    const sep = side === "local" ? (dir.includes("\\") ? "\\" : "/") : "/";
    const newPath = dir + sep + name.trim();
    if (side === "local") {
      await api("/api/local/rename", { method: "POST", body: JSON.stringify({ oldPath: sel.path, newPath }) });
      await loadLocal(state.localPath);
    } else {
      await api("/api/remote/rename", {
        method: "POST",
        body: JSON.stringify({ oldPath: sel.path, newPath, containerId: state.remoteContainerId || undefined }),
      });
      await loadRemote(state.remotePath);
    }
    CWUI.toast("Renomeado", "success");
  }

  async function deleteActive(side) {
    const sel = selectedEntry(side);
    if (!sel || sel.name === "..") {
      CWUI.toast("Selecione um item.", "error");
      return;
    }
    const rec = sel.isDir;
    const needConfirm = window.CWWebPrefs?.get?.("explorerConfirmDelete") !== false;
    if (needConfirm && !(await CWConfirm(`Excluir ${sel.name}?${rec ? " (pasta e conteúdo)" : ""}`, { danger: true, ok: "Excluir" }))) return;
    if (side === "local") {
      await api("/api/local/delete", { method: "POST", body: JSON.stringify({ path: sel.path, recursive: rec }) });
      await loadLocal(state.localPath);
    } else {
      await api("/api/remote/delete", {
        method: "POST",
        body: JSON.stringify({ path: sel.path, recursive: rec, containerId: state.remoteContainerId || undefined }),
      });
      await loadRemote(state.remotePath);
    }
    CWUI.toast("Apagado", "success");
  }

  async function copyActive(side) {
    const sel = selectedEntry(side);
    if (!sel || sel.name === "..") {
      CWUI.toast("Selecione um item.", "error");
      return;
    }
    const source = side === "local" ? "local" : remoteSource();
    await api("/api/explorer/clipboard", {
      method: "POST",
      body: JSON.stringify({
        source,
        path: sel.path,
        isDir: !!sel.isDir,
        containerId: side === "remote" && state.remoteTarget === "container" ? state.remoteContainerId : "",
      }),
    });
    CWUI.toast("Copiado", "info");
  }

  async function pasteActive(side) {
    const targetSource = side === "local" ? "local" : remoteSource();
    await api("/api/explorer/paste", {
      method: "POST",
      body: JSON.stringify({
        targetSource,
        targetPath: side === "local" ? state.localPath : state.remotePath,
        targetContainerId: side === "remote" && state.remoteTarget === "container" ? state.remoteContainerId : "",
      }),
    });
    CWUI.toast("Operação enfileirada", "success");
    state.transferLogPinned = false;
    showTransferLog();
    refreshTransferStatus();
  }

  async function compareFolders() {
    const rep = await api(
      `/api/explorer/compare?localPath=${encodeURIComponent(state.localPath)}&remotePath=${encodeURIComponent(state.remotePath)}${remoteApiExtra()}`
    );
    $("#compare-report").textContent = rep.text || rep.Text || JSON.stringify(rep, null, 2);
    $("#compare-dialog")?.showModal();
  }

  function visibleItems(side) {
    const entries = side === "local" ? state.localEntries : state.remoteEntries;
    return filterEntries(entries, side === "local" ? state.localFilter : state.remoteFilter)
      .filter((e) => e.name !== "..");
  }

  async function batchTransfer(direction) {
    const items = direction === "push" ? visibleItems("local") : visibleItems("remote");
    if (!items.length) {
      CWUI.toast("Nenhum item visível para transferir.", "error");
      return;
    }
    await api("/api/transfer/batch", {
      method: "POST",
      body: JSON.stringify({
        direction,
        localDir: state.localPath,
        remoteDir: state.remotePath,
        containerId: direction === "push" && state.remoteTarget === "container" ? state.remoteContainerId : (direction === "pull" && state.remoteTarget === "container" ? state.remoteContainerId : ""),
        items: items.map((e) => ({ name: e.name, path: e.path, isDir: !!e.isDir })),
      }),
    });
    state.transferLogPinned = false;
    showTransferLog();
    refreshTransferStatus();
    CWUI.toast(`Lote enfileirado (${items.length} itens)`, "success");
  }

  function pushPayload() {
    return {
      localPath: state.selLocal.path,
      remotePath: state.remotePath,
      containerId: state.remoteTarget === "container" ? state.remoteContainerId : undefined,
    };
  }

  function pullPayload() {
    return {
      localPath: state.localPath,
      remotePath: state.selRemote.path,
      containerId: state.remoteTarget === "container" ? state.remoteContainerId : undefined,
    };
  }

  const _btnSend = $("#btn-send");
  if (_btnSend) {
    _btnSend.replaceWith(_btnSend.cloneNode(true));
    $("#btn-send").addEventListener("click", async () => {
      if (!state.selLocal || state.selLocal.name === "..") {
        CWUI.toast(window.CWI18n?.t("explorer.toast.selectLocal") || "Selecione um arquivo ou pasta no painel local.", "error");
        return;
      }
      if (!(await CWConfirm(`Enviar para ${state.remotePath}?`))) return;
      try {
        await api("/api/transfer/push", { method: "POST", body: JSON.stringify(pushPayload()) });
        state.transferLogPinned = false;
        showTransferLog();
        refreshTransferStatus();
        CWUI.toast("Transferência enfileirada", "success");
      } catch (e) { CWUI.toast(e.message, "error"); }
    });
  }

  const _btnRecv = $("#btn-receive");
  if (_btnRecv) {
    _btnRecv.replaceWith(_btnRecv.cloneNode(true));
    $("#btn-receive").addEventListener("click", async () => {
      if (!state.selRemote || state.selRemote.name === "..") {
        CWUI.toast("Selecione um item no painel remoto.", "error");
        return;
      }
      if (!(await CWConfirm(`Receber para ${state.localPath}?`))) return;
      try {
        await api("/api/transfer/pull", { method: "POST", body: JSON.stringify(pullPayload()) });
        state.transferLogPinned = false;
        showTransferLog();
        refreshTransferStatus();
        CWUI.toast("Transferência enfileirada", "success");
      } catch (e) { CWUI.toast(e.message, "error"); }
    });
  }

  function fileEditApiUrl(side, filePath) {
    const q = encodeURIComponent(filePath);
    if (side === "local") return `/api/local/file?path=${q}`;
    let u = `/api/remote/file?path=${q}`;
    if (state.remoteTarget === "container" && state.remoteContainerId) {
      u += `&containerId=${encodeURIComponent(state.remoteContainerId)}`;
    }
    return u;
  }

  function showFileEditorError(msg) {
    const el = $("#file-editor-error");
    if (!el) return;
    if (msg) {
      el.textContent = msg;
      el.classList.remove("hidden");
    } else {
      el.textContent = "";
      el.classList.add("hidden");
    }
  }

  function setFileEditorMode(mode) {
    const ta = $("#file-editor-content");
    const imgWrap = $("#file-editor-image-wrap");
    const img = $("#file-editor-preview");
    const hint = $("#file-editor-hint");
    const saveBtn = $("#file-editor-save");
    const syncBtn = $("#file-editor-sync-external");
    if (mode === "image") {
      ta?.classList.add("hidden");
      imgWrap?.classList.remove("hidden");
      saveBtn?.classList.add("hidden");
      hint?.classList.remove("hidden");
      if (hint) hint.textContent = "Pré-visualização. Para editar a imagem, use um programa externo abaixo.";
    } else if (mode === "text") {
      ta?.classList.remove("hidden");
      imgWrap?.classList.add("hidden");
      saveBtn?.classList.remove("hidden");
      hint?.classList.add("hidden");
      if (img) img.removeAttribute("src");
    } else {
      ta?.classList.add("hidden");
      imgWrap?.classList.add("hidden");
      saveBtn?.classList.add("hidden");
      hint?.classList.remove("hidden");
      if (hint) hint.textContent = window.CWI18n?.t("explorer.editor.binary") || "Arquivo binário ou não suportado no editor web. Abra com um programa no seu PC.";
    }
    syncBtn?.classList.toggle("hidden", !state.fileEdit?.externalSessionId);
  }

  async function openFileExternal(editor) {
    const edit = state.fileEdit;
    if (!edit) return;
    showFileEditorError("");
    try {
      if (edit.side === "local") {
        const res = await api("/api/local/open-external", {
          method: "POST",
          body: JSON.stringify({ path: edit.path, editor }),
        });
        CWUI.toast(res.message || "Programa aberto.", "success");
        return;
      }
      const body = { path: edit.path, editor };
      if (state.remoteTarget === "container" && state.remoteContainerId) {
        body.containerId = state.remoteContainerId;
      }
      const res = await api("/api/remote/open-external", { method: "POST", body: JSON.stringify(body) });
      state.fileEdit.externalSessionId = res.sessionId || "";
      setFileEditorMode(state.fileEdit.mode || "binary");
      CWUI.toast(res.message || "Aberto no programa externo.", "success");
    } catch (e) {
      showFileEditorError(e.message || "Falha ao abrir externamente.");
    }
  }

  window.openFileEditor = async function openFileEditor(side, entry) {
    if (!entry || entry.isDir || entry.name === "..") return;
    if (side === "remote" && !state.ssh.connected) {
      CWUI.toast("Ligue-se ao SSH primeiro.", "error");
      return;
    }
    if (side === "remote" && state.remoteTarget === "container" && !state.remoteContainerId) {
      CWUI.toast("Selecione um contêiner.", "error");
      return;
    }
    const dlg = $("#file-editor-dialog");
    const ta = $("#file-editor-content");
    if (!dlg || !ta) return;

    state.fileEdit = { side, path: entry.path, name: entry.name, mode: "loading", externalSessionId: "" };
    $("#file-editor-title").textContent = entry.name;
    $("#file-editor-path").textContent = entry.path;
    ta.value = "";
    ta.disabled = true;
    showFileEditorError("");
    setFileEditorMode("text");
    dlg.showModal();

    try {
      const data = await api(fileEditApiUrl(side, entry.path));
      if (data.encoding === "base64" && (data.mimeType || "").startsWith("image/")) {
        state.fileEdit.mode = "image";
        const img = $("#file-editor-preview");
        if (img) img.src = `data:${data.mimeType};base64,${data.content}`;
        setFileEditorMode("image");
        return;
      }
      state.fileEdit.mode = "text";
      ta.value = data.content ?? "";
      ta.disabled = false;
      setFileEditorMode("text");
      ta.focus();
    } catch (e) {
      state.fileEdit.mode = "binary";
      setFileEditorMode("binary");
      showFileEditorError(e.message || "Não foi possível abrir como texto.");
    }
  };

  window.showCtxMenu = function showCtxMenu(ev, side, entry) {
    state.ctxSide = side;
    state.ctxEntry = entry;
    const menu = $("#explorer-ctx-menu");
    if (!menu) return;
    const openBtn = menu.querySelector('[data-action="open"]');
    const editBtn = menu.querySelector('[data-action="edit"]');
    if (openBtn) openBtn.hidden = !entry?.isDir;
    if (editBtn) editBtn.hidden = !entry || entry.isDir || entry.name === "..";
    menu.classList.remove("hidden");
    menu.style.left = `${ev.clientX}px`;
    menu.style.top = `${ev.clientY}px`;
  };

  function hideCtxMenu() {
    $("#explorer-ctx-menu")?.classList.add("hidden");
  }

  document.addEventListener("click", hideCtxMenu);

  $$(".explorer-menu-panel .btn, .explorer-menu-panel button").forEach((btn) => {
    btn.addEventListener("click", () => window.CWUI?.closeAllFloatMenus?.());
  });

  $("#explorer-ctx-menu")?.addEventListener("click", async (ev) => {
    const btn = ev.target.closest("[data-action]");
    if (!btn) return;
    const side = state.ctxSide;
    const entry = state.ctxEntry;
    hideCtxMenu();
    if (!side || !entry) return;
    const act = btn.dataset.action;
    if (act === "open" && entry.isDir) {
      if (side === "local") loadLocal(entry.path);
      else loadRemote(entry.path);
      return;
    }
    if (act === "edit" && !entry.isDir) {
      openFileEditor(side, entry);
      return;
    }
    if (act === "open-external" && !entry.isDir) {
      state.fileEdit = { side, path: entry.path, name: entry.name, mode: "binary", externalSessionId: "" };
      openFileExternal("notepad++");
      return;
    }
    if (act === "send" && side === "local") {
      state.selLocal = entry;
      $("#btn-send").click();
      return;
    }
    if (act === "receive" && side === "remote") {
      state.selRemote = entry;
      $("#btn-receive").click();
      return;
    }
    if (act === "copy") {
      if (side === "local") state.selLocal = entry;
      else state.selRemote = entry;
      await copyActive(side);
      return;
    }
    if (act === "paste") {
      await pasteActive(side);
      return;
    }
    if (act === "rename") {
      if (side === "local") state.selLocal = entry;
      else state.selRemote = entry;
      await renameActive(side);
      return;
    }
    if (act === "delete") {
      if (side === "local") state.selLocal = entry;
      else state.selRemote = entry;
      await deleteActive(side);
      return;
    }
    if (act === "mkdir") await mkdirActive(side);
  });

  $("#btn-new-folder")?.addEventListener("click", () => {
    const side = state.selRemote && !state.selLocal ? "remote" : "local";
    mkdirActive(side);
  });
  $("#btn-rename-item")?.addEventListener("click", () => renameActive(state.selRemote ? "remote" : "local"));
  $("#btn-delete-item")?.addEventListener("click", () => deleteActive(state.selRemote ? "remote" : "local"));
  $("#btn-copy-item")?.addEventListener("click", () => copyActive(state.selRemote ? "remote" : "local"));
  $("#btn-paste-item")?.addEventListener("click", () => pasteActive(state.selLocal ? "local" : "remote"));
  $("#btn-edit-file")?.addEventListener("click", () => {
    const side = state.selRemote && !state.selLocal ? "remote" : "local";
    const entry = side === "local" ? state.selLocal : state.selRemote;
    if (!entry || entry.isDir) {
      CWUI.toast(window.CWI18n?.t("explorer.toast.selectFile") || "Selecione um arquivo para editar.", "error");
      return;
    }
    openFileEditor(side, entry);
  });
  $("#btn-compare-folders")?.addEventListener("click", compareFolders);
  $("#btn-batch-send")?.addEventListener("click", () => batchTransfer("push"));
  $("#btn-batch-receive")?.addEventListener("click", () => batchTransfer("pull"));

  $("#local-filter")?.addEventListener("input", (e) => {
    state.localFilter = e.target.value;
    renderEntries($("#local-list"), state.localEntries, "local");
  });
  $("#remote-filter")?.addEventListener("input", (e) => {
    state.remoteFilter = e.target.value;
    renderEntries($("#remote-list"), state.remoteEntries, "remote");
  });

  $("#btn-sudo")?.addEventListener("click", async () => {
    if (!state.ssh.connected || remoteSource() !== "host") return;
    if (state.sudo?.enabled) {
      if (!(await CWConfirm("Desativar modo sudo?"))) return;
      try {
        await api("/api/ssh/sudo", { method: "POST", body: JSON.stringify({ action: "disable" }) });
        state.sudo = { enabled: false, user: "" };
        updateSudoButton();
        CWUI.toast("Sudo desactivado", "success");
        document.dispatchEvent(new CustomEvent("cw-sudo-changed"));
        if (state.remotePath) loadRemote(state.remotePath);
      } catch (e) {
        CWUI.toast(e.message, "error");
      }
      return;
    }
    clearSudoError();
    $("#sudo-pass").value = "";
    $("#sudo-dialog")?.showModal();
    $("#sudo-pass")?.focus();
  });

  function showSudoError(message) {
    const el = $("#sudo-error");
    if (!el) {
      CWUI.toast(message, "error");
      return;
    }
    el.textContent = message;
    el.classList.remove("hidden");
  }

  function clearSudoError() {
    const el = $("#sudo-error");
    if (!el) return;
    el.textContent = "";
    el.classList.add("hidden");
  }

  $("#sudo-cancel")?.addEventListener("click", () => {
    clearSudoError();
    $("#sudo-dialog")?.close();
  });

  async function activateSudo() {
    clearSudoError();
    const user = ($("#sudo-user")?.value || "root").trim() || "root";
    const password = $("#sudo-pass")?.value || "";
    if (!password) {
      showSudoError("Indique a senha sudo.");
      return;
    }
    if (!state.ssh.connected) {
      showSudoError("Ligue-se ao servidor SSH antes de activar o sudo.");
      return;
    }
    const btn = $("#sudo-submit");
    if (btn) {
      btn.disabled = true;
      btn.textContent = "A validar…";
    }
    try {
      const res = await api("/api/ssh/sudo", {
        method: "POST",
        body: JSON.stringify({ action: "enable", user, password }),
      });
      state.sudo = { enabled: true, user: res.user || user };
      updateSudoButton();
      $("#sudo-dialog")?.close();
      CWUI.toast(`Sudo ativo (${state.sudo.user})`, "success");
      document.dispatchEvent(new CustomEvent("cw-sudo-changed"));
      if (state.screen === "explorer") await loadRemote(state.remotePath || "/");
    } catch (e) {
      const msg = e.message || "Falha ao activar sudo";
      if (state.user) showSudoError(msg);
    } finally {
      if (btn) {
        btn.disabled = false;
        btn.textContent = "Activar sudo";
      }
    }
  }

  $("#sudo-submit")?.addEventListener("click", () => void activateSudo());
  $("#sudo-form")?.addEventListener("keydown", (ev) => {
    if (ev.key === "Enter") {
      ev.preventDefault();
      void activateSudo();
    }
  });

  $("#remote-target-host")?.addEventListener("click", () => setRemoteTarget("host"));
  $("#remote-target-container")?.addEventListener("click", () => setRemoteTarget("container"));
  $("#file-editor-cancel")?.addEventListener("click", () => {
    showFileEditorError("");
    $("#file-editor-dialog")?.close();
    state.fileEdit = null;
    $("#file-editor-preview")?.removeAttribute("src");
  });

  $("#file-editor-open-default")?.addEventListener("click", () => openFileExternal("default"));
  $("#file-editor-open-npp")?.addEventListener("click", () => openFileExternal("notepad++"));

  $("#file-editor-sync-external")?.addEventListener("click", async () => {
    const sid = state.fileEdit?.externalSessionId;
    if (!sid) return;
    try {
      const res = await api("/api/remote/sync-external", {
        method: "POST",
        body: JSON.stringify({ sessionId: sid }),
      });
      state.fileEdit.externalSessionId = "";
      setFileEditorMode(state.fileEdit.mode || "binary");
      CWUI.toast(res.message || "Sincronizado.", "success");
      await loadRemote(state.remotePath);
    } catch (e) {
      showFileEditorError(e.message || "Falha ao sincronizar.");
    }
  });

  $("#file-editor-save")?.addEventListener("click", async () => {
    const edit = state.fileEdit;
    const ta = $("#file-editor-content");
    const btn = $("#file-editor-save");
    if (!edit || !ta || ta.disabled) return;
    if (btn) {
      btn.disabled = true;
      btn.textContent = "Salvando…";
    }
    showFileEditorError("");
    try {
      const body = { path: edit.path, content: ta.value };
      if (edit.side === "remote" && state.remoteTarget === "container" && state.remoteContainerId) {
        body.containerId = state.remoteContainerId;
      }
      await api(edit.side === "local" ? "/api/local/file" : "/api/remote/file", {
        method: "PUT",
        body: JSON.stringify(body),
      });
      CWUI.toast("Arquivo salvo", "success");
      $("#file-editor-dialog")?.close();
      state.fileEdit = null;
      if (edit.side === "local") await loadLocal(state.localPath);
      else await loadRemote(state.remotePath);
    } catch (e) {
      showFileEditorError(e.message || "Falha ao salvar.");
    } finally {
      if (btn) {
        btn.disabled = false;
        btn.textContent = "Salvar";
      }
    }
  });

  $("#file-editor-content")?.addEventListener("keydown", (ev) => {
    if ((ev.ctrlKey || ev.metaKey) && ev.key === "s") {
      ev.preventDefault();
      $("#file-editor-save")?.click();
    }
  });

  $("#remote-container")?.addEventListener("change", (e) => {
    state.remoteContainerId = e.target.value;
    resetRemoteToRoot();
    if (state.remoteContainerId) loadRemote("/");
    else {
      const listEl = $("#remote-list");
      if (listEl) {
        state.remoteEntries = [];
        CWUI.emptyState(listEl, {
          icon: "🐳",
          title: "Selecione um contêiner",
          desc: "Escolha um contêiner em execução na lista acima.",
        });
      }
    }
  });

  $$(".icon-btn[data-action=fav-add]").forEach((btn) => {
    btn.addEventListener("click", () => addFavorite(btn.dataset.side === "local" ? "local" : "remote"));
  });

  const origShowScreen = window.showScreen;
  if (origShowScreen) {
    window.showScreen = function (name) {
      origShowScreen(name);
      if (name === "explorer") {
        refreshFavorites();
        setRemoteTarget(state.remoteTarget || "host");
        if (typeof refreshSudoUI === "function") refreshSudoUI();
      }
    };
  }

  const U = window.CWUI;
  if (U?.wireFloatingDropdown) {
    U.wireFloatingDropdown(document.querySelector(".explorer-menu"), ".explorer-menu-panel", "left", "explorer");
    U.wireFloatingDropdown($("#local-menu"), ".panel-menu-list", "right", "explorer");
    U.wireFloatingDropdown($("#remote-menu"), ".panel-menu-list", "right", "explorer");
  }

  if (state.user) refreshFavorites();

  // Delegação: garante cliques mesmo se o DOM do diálogo mudar
  document.addEventListener("click", (ev) => {
    const t = ev.target;
    if (t.closest?.("[data-action=sudo-cancel]")) {
      clearSudoError();
      $("#sudo-dialog")?.close();
      return;
    }
    if (t.id === "sudo-submit" || t.closest?.("#sudo-submit")) {
      ev.preventDefault();
      void activateSudo();
    }
  });

  document.addEventListener("keydown", (ev) => {
    const dlg = $("#sudo-dialog");
    if (!dlg?.open) return;
    if (ev.key === "Escape") {
      clearSudoError();
      dlg.close();
    }
  });
})();
