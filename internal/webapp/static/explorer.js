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
      btn.textContent = "Sudo: contêiner";
      btn.title = "Sudo só está disponível no modo Host SFTP";
      return;
    }
    btn.title = on ? "Desactivar sudo" : "Activar sudo/root no host";
    btn.textContent = on ? `Sudo: ${state.sudo.user || "root"}` : "Sudo: inativo";
  }

  window.refreshSudoUI = async function refreshSudoUI() {
    if (!state.ssh.connected) {
      state.sudo = { enabled: false, user: "" };
      updateSudoButton();
      return;
    }
    try {
      const st = await api("/api/ssh/sudo");
      state.sudo = { enabled: !!st.enabled, user: st.user || "" };
    } catch {
      state.sudo = { enabled: false, user: "" };
    }
    updateSudoButton();
  };

  function filterEntries(entries, q) {
    const hay = (q || "").trim().toLowerCase();
    if (!hay) return entries || [];
    return (entries || []).filter((e) => e.name === ".." || e.name.toLowerCase().includes(hay));
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
        desc: q ? "Nenhum item corresponde ao filtro." : (side === "local" ? "Não há ficheiros nesta pasta local." : "Não há ficheiros neste diretório."),
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
    CWUI.skeletonList($("#local-list"), 8);
    const q = path ? `?path=${encodeURIComponent(path)}` : "";
    const data = await api("/api/local/list" + q);
    state.localPath = data.path;
    state.selLocal = null;
    const lp = $("#local-path");
    lp.textContent = data.path;
    lp.title = data.path;
    renderEntries($("#local-list"), data.entries, "local");
    if (typeof updateExplorerBreadcrumbs === "function") updateExplorerBreadcrumbs();
  };

  window.loadRemote = async function loadRemote(path) {
    if (!state.ssh.connected) return;
    if (state.remoteTarget === "container" && !state.remoteContainerId) {
      CWUI.toast("Selecione um contêiner.", "error");
      return;
    }
    CWUI.skeletonList($("#remote-list"), 8);
    const data = await api(`/api/remote/list?path=${encodeURIComponent(path || "/")}${remoteApiExtra()}`);
    state.remotePath = data.path;
    state.selRemote = null;
    const rp = $("#remote-path");
    rp.textContent = data.path;
    rp.title = data.path;
    renderEntries($("#remote-list"), data.entries, "remote");
    if (typeof updateExplorerBreadcrumbs === "function") updateExplorerBreadcrumbs();
  };

  function shortContainerLabel(c) {
    const name = (c.name || c.id || "").replace(/^\//, "");
    const short = name.length > 28 ? name.slice(0, 26) + "…" : name;
    const img = (c.image || "").split(":")[0];
    const imgShort = img.length > 20 ? img.slice(0, 18) + "…" : img;
    return imgShort ? `${short} · ${imgShort}` : short;
  }

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

  function setRemoteTarget(mode) {
    state.remoteTarget = mode;
    $$(".remote-target-btn").forEach((b) => {
      b.classList.toggle("active", b.dataset.target === mode);
    });
    const showC = mode === "container";
    $("#remote-container-wrap")?.classList.toggle("hidden", !showC);
    if (!state.ssh.connected) return;
    if (showC) {
      loadContainersSelect().then(() => {
        if (state.remoteContainerId) loadRemote(state.remotePath || "/");
      });
    } else {
      state.remoteContainerId = "";
      loadRemote(state.remotePath || "/");
    }
    updateSudoButton();
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
    const name = prompt("Nome da nova pasta:");
    if (!name?.trim()) return;
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
    const name = prompt("Novo nome:", sel.name);
    if (!name?.trim() || name === sel.name) return;
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
    if (!(await CWConfirm(`Apagar ${sel.name}?${rec ? " (pasta e conteúdo)" : ""}`, { danger: true, ok: "Apagar" }))) return;
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
        CWUI.toast("Selecione um ficheiro ou pasta no painel local.", "error");
        return;
      }
      if (!(await CWConfirm(`Enviar para ${state.remotePath}?`))) return;
      try {
        await api("/api/transfer/push", { method: "POST", body: JSON.stringify(pushPayload()) });
        showTransferLog();
        $("#transfer-progress-panel")?.classList.remove("hidden");
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
        showTransferLog();
        $("#transfer-progress-panel")?.classList.remove("hidden");
        refreshTransferStatus();
        CWUI.toast("Transferência enfileirada", "success");
      } catch (e) { CWUI.toast(e.message, "error"); }
    });
  }

  function showCtxMenu(ev, side, entry) {
    state.ctxSide = side;
    state.ctxEntry = entry;
    const menu = $("#explorer-ctx-menu");
    if (!menu) return;
    menu.classList.remove("hidden");
    menu.style.left = `${ev.clientX}px`;
    menu.style.top = `${ev.clientY}px`;
  }

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
      if (!(await CWConfirm("Desactivar modo sudo?"))) return;
      try {
        await api("/api/ssh/sudo", { method: "POST", body: JSON.stringify({ action: "disable" }) });
        state.sudo = { enabled: false, user: "" };
        updateSudoButton();
        CWUI.toast("Sudo desactivado", "success");
        if (state.remotePath) loadRemote(state.remotePath);
      } catch (e) {
        CWUI.toast(e.message, "error");
      }
      return;
    }
    $("#sudo-pass").value = "";
    $("#sudo-dialog")?.showModal();
    $("#sudo-pass")?.focus();
  });

  $("#sudo-cancel")?.addEventListener("click", () => $("#sudo-dialog")?.close());

  $("#sudo-form")?.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const user = ($("#sudo-user")?.value || "root").trim() || "root";
    const password = $("#sudo-pass")?.value || "";
    if (!password) {
      CWUI.toast("Indique a senha sudo.", "error");
      return;
    }
    try {
      const res = await api("/api/ssh/sudo", {
        method: "POST",
        body: JSON.stringify({ action: "enable", user, password }),
      });
      state.sudo = { enabled: true, user: res.user || user };
      updateSudoButton();
      $("#sudo-dialog")?.close();
      CWUI.toast(`Sudo activo (${state.sudo.user})`, "success");
      loadRemote(state.remotePath || "/");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  $("#remote-target-host")?.addEventListener("click", () => setRemoteTarget("host"));
  $("#remote-target-container")?.addEventListener("click", () => setRemoteTarget("container"));
  $("#remote-container")?.addEventListener("change", (e) => {
    state.remoteContainerId = e.target.value;
    if (state.remoteContainerId) loadRemote(state.remotePath || "/");
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

  refreshFavorites();

  const U = window.CWUI;
  if (U?.wireFloatingDropdown) {
    U.wireFloatingDropdown(document.querySelector(".explorer-menu"), ".explorer-menu-panel", "left", "explorer");
    U.wireFloatingDropdown($("#local-menu"), ".panel-menu-list", "right", "explorer");
    U.wireFloatingDropdown($("#remote-menu"), ".panel-menu-list", "right", "explorer");
  }
})();
