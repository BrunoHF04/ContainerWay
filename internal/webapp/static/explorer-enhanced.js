/** Melhorias do explorador web — fases A–D (atalhos, multi-seleção, drag, comparar, preview, dock). */
(function () {
  const U = window.CWUI;
  if (!U) return;

  Object.assign(state, {
    explorerFocus: "local",
    selLocalPaths: new Set(),
    selRemotePaths: new Set(),
    lastListIndex: { local: -1, remote: -1 },
    sort: {
      local: { col: "name", asc: true },
      remote: { col: "name", asc: true },
    },
    containerCatalog: [],
    compareReport: null,
    previewToken: 0,
  });

  function sidePanel(side) {
    return document.querySelector(`.panel[data-side="${side}"]`);
  }

  function listEl(side) {
    return side === "local" ? $("#local-list") : $("#remote-list");
  }

  function selectionSet(side) {
    return side === "local" ? state.selLocalPaths : state.selRemotePaths;
  }

  function syncPrimarySelection(side) {
    const set = selectionSet(side);
    const entries = side === "local" ? state.localEntries : state.remoteEntries;
    const firstPath = set.values().next().value;
    const entry = (entries || []).find((e) => e.path === firstPath) || null;
    if (side === "local") state.selLocal = entry;
    else state.selRemote = entry;
  }

  function clearSelection(side) {
    selectionSet(side).clear();
    state.lastListIndex[side] = -1;
    syncPrimarySelection(side);
    highlightRows(side);
    updatePanelStatus(side);
    schedulePreview(side);
  }

  function setSelection(side, entry, opts = {}) {
    const set = selectionSet(side);
    if (!opts.append) set.clear();
    if (entry && entry.name !== "..") set.add(entry.path);
    else if (!opts.append) set.clear();
    syncPrimarySelection(side);
    highlightRows(side);
    updatePanelStatus(side);
    schedulePreview(side);
  }

  function toggleSelection(side, entry) {
    const set = selectionSet(side);
    if (!entry || entry.name === "..") return;
    if (set.has(entry.path)) set.delete(entry.path);
    else set.add(entry.path);
    syncPrimarySelection(side);
    highlightRows(side);
    updatePanelStatus(side);
    schedulePreview(side);
  }

  function rangeSelect(side, entry, visible) {
    const set = selectionSet(side);
    const idx = visible.findIndex((e) => e.path === entry.path);
    const from = state.lastListIndex[side] >= 0 ? state.lastListIndex[side] : idx;
    if (idx < 0) return;
    const a = Math.min(from, idx);
    const b = Math.max(from, idx);
    for (let i = a; i <= b; i++) {
      const e = visible[i];
      if (e && e.name !== "..") set.add(e.path);
    }
    syncPrimarySelection(side);
    highlightRows(side);
    updatePanelStatus(side);
    schedulePreview(side);
  }

  function highlightRows() {
    ["local", "remote"].forEach((s) => {
      const set = selectionSet(s);
      const el = listEl(s);
      if (!el) return;
      el.querySelectorAll(".file-row").forEach((row) => {
        row.classList.toggle("selected", set.has(row.dataset.path));
      });
    });
    document.querySelectorAll(".panel[data-side]").forEach((p) => {
      p.classList.toggle("explorer-panel-active", p.dataset.side === state.explorerFocus);
    });
  }

  function formatModTime(iso) {
    if (!iso) return "";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return "";
    const pad = (n) => String(n).padStart(2, "0");
    return `${pad(d.getDate())}/${pad(d.getMonth() + 1)} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  function sortEntries(entries, side) {
    const { col, asc } = state.sort[side];
    const list = [...(entries || [])];
    const parent = list.find((e) => e.name === "..");
    const rest = list.filter((e) => e.name !== "..");
    rest.sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      let cmp = 0;
      if (col === "size") cmp = (a.size || 0) - (b.size || 0);
      else if (col === "modTime") cmp = String(a.modTime || "").localeCompare(String(b.modTime || ""));
      else cmp = a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
      return asc ? cmp : -cmp;
    });
    return parent ? [parent, ...rest] : rest;
  }

  function isPermissionError(msg) {
    return /permission denied|permissão negada|permiso denegado/i.test(String(msg || ""));
  }

  function showPanelAlert(side, message, showSudo) {
    const wrap = side === "remote" ? $("#remote-panel-alert") : $("#local-panel-alert");
    if (!wrap) return;
    if (!message) {
      wrap.classList.add("hidden");
      return;
    }
    const txt = $("#remote-panel-alert-text");
    if (txt) txt.textContent = message;
    else wrap.textContent = message;
    wrap.classList.remove("hidden");
    const sudoBtn = $("#remote-panel-alert-sudo");
    if (sudoBtn) sudoBtn.classList.toggle("hidden", !showSudo);
  }

  function refreshExplorerNav() {
    if (typeof window.updateExplorerBreadcrumbs === "function") window.updateExplorerBreadcrumbs();
  }

  function updatePanelStatus(side) {
    const foot = side === "local" ? $("#local-status") : $("#remote-status");
    if (!foot) return;
    const entries = side === "local" ? state.localEntries : state.remoteEntries;
    const n = (entries || []).filter((e) => e.name !== "..").length;
    const sel = selectionSet(side).size;
    foot.textContent = sel ? `${n} itens · ${sel} selecionado(s)` : `${n} itens`;
  }

  function visibleList(side) {
    const entries = side === "local" ? state.localEntries : state.remoteEntries;
    const q = side === "local" ? state.localFilter : state.remoteFilter;
    if (typeof window.cwPanelFilter === "function") return window.cwPanelFilter(entries || [], q);
    const hay = (q || "").trim().toLowerCase();
    let list = entries || [];
    if (hay) list = list.filter((e) => e.name === ".." || e.name.toLowerCase().includes(hay));
    return list;
  }

  function joinPath(base, name, side) {
    const sep = side === "local" && base.includes("\\") ? "\\" : "/";
    return base.replace(/[/\\]+$/, "") + sep + name;
  }

  function selectedEntries(side) {
    const set = selectionSet(side);
    const entries = side === "local" ? state.localEntries : state.remoteEntries;
    return (entries || []).filter((e) => set.has(e.path) && e.name !== "..");
  }

  function remoteApiExtra() {
    const cid = state.remoteTarget === "container" ? state.remoteContainerId : "";
    return cid ? `&containerId=${encodeURIComponent(cid)}` : "";
  }

  window.showRemotePermissionAlert = function (msg) {
    if (/permission denied|permissão negada/i.test(String(msg || "")) && state.remoteTarget === "host") {
      showPanelAlert("remote", msg, true);
    }
  };

  window.renderEntries = function renderEntriesEnhanced(container, entries, side) {
    container.classList.remove("skeleton-host");
    container.innerHTML = "";
    const sorted = sortEntries(entries, side);
    if (side === "local") state.localEntries = entries || [];
    else state.remoteEntries = entries || [];
    const q = side === "local" ? state.localFilter : state.remoteFilter;
    const list = typeof window.cwPanelFilter === "function"
      ? window.cwPanelFilter(sorted, q)
      : (() => {
          const hay = (q || "").trim().toLowerCase();
          if (!hay) return sorted;
          return sorted.filter((e) => e.name === ".." || e.name.toLowerCase().includes(hay));
        })();
    if (!list.length) {
      CWUI.emptyState(container, {
        icon: "📂",
        title: q ? "Sem resultados" : "Pasta vazia",
        desc: q ? "Nenhum item corresponde ao filtro." : (side === "local" ? "Não há arquivos nesta pasta local." : "Não há arquivos neste diretório."),
      });
      updatePanelStatus(side);
      return;
    }
    for (const e of list) {
      const row = document.createElement("div");
      row.className = "file-row" + (e.isDir ? " dir" : "");
      row.dataset.path = e.path;
      row.dataset.isDir = e.isDir ? "1" : "0";
      row.dataset.name = e.name;
      row.dataset.side = side;
      row.draggable = e.name !== "..";
      const icon = CWUI.fileIcon(e.name, e.isDir);
      const dateStr = e.isDir ? "" : formatModTime(e.modTime);
      const sizeStr = e.isDir ? "" : formatSize(e.size);
      row.innerHTML = `<span class="icon">${icon}</span><span class="name">${escapeHtml(e.name)}</span><span class="col-date">${dateStr}</span><span class="meta">${sizeStr}</span>`;
      row.addEventListener("click", (ev) => {
        state.explorerFocus = side;
        const vis = visibleList(side);
        const i = vis.findIndex((x) => x.path === e.path);
        if (ev.shiftKey && state.lastListIndex[side] >= 0) rangeSelect(side, e, vis);
        else if (ev.ctrlKey || ev.metaKey) toggleSelection(side, e);
        else {
          selectionSet(side).clear();
          setSelection(side, e);
        }
        state.lastListIndex[side] = i;
        highlightRows(side);
      });
      row.addEventListener("dblclick", () => {
        if (e.isDir) {
          if (side === "local") loadLocal(e.path);
          else loadRemote(e.path);
        } else if (e.name !== "..") openFileEditor(side, e);
      });
      row.addEventListener("contextmenu", (ev) => {
        ev.preventDefault();
        row.click();
        if (typeof window.showCtxMenu === "function") window.showCtxMenu(ev, side, e);
      });
      container.appendChild(row);
    }
    highlightRows(side);
    updatePanelStatus(side);
  };

  const origLoadLocal = window.loadLocal;
  window.loadLocal = async function loadLocalEnhanced(path) {
    clearSelection("local");
    await origLoadLocal(path);
    updatePanelStatus("local");
  };

  const origLoadRemote = window.loadRemote;
  window.loadRemote = async function loadRemoteEnhanced(path) {
    showPanelAlert("remote", null);
    clearSelection("remote");
    await origLoadRemote(path);
    refreshExplorerNav();
    updatePanelStatus("remote");
  };

  window.onExplorerContainersLoaded = function (list) {
    state.containerCatalog = (list || []).map((c) => {
      const name = (c.name || c.id || "").replace(/^\//, "");
      const short = name.length > 18 ? name.slice(0, 16) + "…" : name;
      return {
        id: c.id,
        idFull: c.idFull || c.id,
        name,
        image: c.image || "",
        label: short,
        title: `${c.name || c.id} — ${c.image}`,
      };
    });
  };

  async function promptName(opts) {
    return U.nameInputDialog(opts);
  }

  function rebindBtn(id, handler) {
    const btn = $(id);
    if (!btn) return;
    const n = btn.cloneNode(true);
    btn.replaceWith(n);
    n.addEventListener("click", handler);
  }

  rebindBtn("#btn-new-folder", async () => {
    const side = state.explorerFocus;
    const name = await promptName({ title: "Nova pasta", label: "Nome da pasta" });
    if (!name) return;
    const base = side === "local" ? state.localPath : state.remotePath;
    const path = joinPath(base, name, side);
    try {
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
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  rebindBtn("#btn-delete-item", async () => {
    const side = state.explorerFocus;
    const items = selectedEntries(side);
    if (!items.length) {
      CWUI.toast("Selecione um ou mais itens.", "error");
      return;
    }
    if (!(await CWConfirm(`Excluir ${items.length} item(ns)?`, { danger: true, ok: "Excluir" }))) return;
    try {
      for (const sel of items) {
        if (side === "local") {
          await api("/api/local/delete", { method: "POST", body: JSON.stringify({ path: sel.path, recursive: !!sel.isDir }) });
        } else {
          await api("/api/remote/delete", {
            method: "POST",
            body: JSON.stringify({ path: sel.path, recursive: !!sel.isDir, containerId: state.remoteContainerId || undefined }),
          });
        }
      }
      if (side === "local") await loadLocal(state.localPath);
      else await loadRemote(state.remotePath);
      clearSelection(side);
      CWUI.toast("Apagado", "success");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  rebindBtn("#btn-rename-item", async () => {
    const side = state.explorerFocus;
    const items = selectedEntries(side);
    const sel = items[0] || (side === "local" ? state.selLocal : state.selRemote);
    if (!sel || sel.name === "..") {
      CWUI.toast("Selecione um item.", "error");
      return;
    }
    const name = await promptName({ title: "Renomear", label: "Novo nome", value: sel.name });
    if (!name || name === sel.name) return;
    const dir = sel.path.replace(/[/\\][^/\\]+$/, "");
    const newPath = joinPath(dir, name, side);
    try {
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
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  async function transferPushEntries(items) {
    if (!items.length) {
      CWUI.toast("Nada selecionado no painel local.", "error");
      return;
    }
    await api("/api/transfer/batch", {
      method: "POST",
      body: JSON.stringify({
        direction: "push",
        localDir: state.localPath,
        remoteDir: state.remotePath,
        containerId: state.remoteTarget === "container" ? state.remoteContainerId : "",
        items: items.map((e) => ({ name: e.name, path: e.path, isDir: !!e.isDir })),
      }),
    });
    state.transferLogPinned = false;
    showTransferLog();
    refreshTransferStatus();
    CWUI.toast(`Enviado (${items.length})`, "success");
  }

  async function transferPullEntries(items) {
    if (!items.length) {
      CWUI.toast("Nada selecionado no painel remoto.", "error");
      return;
    }
    await api("/api/transfer/batch", {
      method: "POST",
      body: JSON.stringify({
        direction: "pull",
        localDir: state.localPath,
        remoteDir: state.remotePath,
        containerId: state.remoteTarget === "container" ? state.remoteContainerId : "",
        items: items.map((e) => ({ name: e.name, path: e.path, isDir: !!e.isDir })),
      }),
    });
    state.transferLogPinned = false;
    showTransferLog();
    refreshTransferStatus();
    CWUI.toast(`Recebido (${items.length})`, "success");
  }

  function visibleItems(side) {
    const entries = side === "local" ? state.localEntries : state.remoteEntries;
    const q = side === "local" ? state.localFilter : state.remoteFilter;
    const list =
      typeof window.cwPanelFilter === "function"
        ? window.cwPanelFilter(entries, q)
        : (entries || []).filter((e) => e.name !== ".." && (!q || e.name.toLowerCase().includes(String(q).toLowerCase())));
    return list.filter((e) => e.name !== "..");
  }

  async function batchTransfer(direction) {
    const items = direction === "push" ? visibleItems("local") : visibleItems("remote");
    if (!items.length) {
      CWUI.toast("Nenhum item visível para transferir.", "error");
      return;
    }
    const dest = direction === "push" ? state.remotePath : state.localPath;
    const msg =
      direction === "push"
        ? `Enviar ${items.length} item(ns) visíveis para ${dest}?`
        : `Receber ${items.length} item(ns) visíveis para ${dest}?`;
    if (!(await CWConfirm(msg))) return;
    await api("/api/transfer/batch", {
      method: "POST",
      body: JSON.stringify({
        direction,
        localDir: state.localPath,
        remoteDir: state.remotePath,
        containerId:
          state.remoteTarget === "container" ? state.remoteContainerId : "",
        items: items.map((e) => ({ name: e.name, path: e.path, isDir: !!e.isDir })),
      }),
    });
    state.transferLogPinned = false;
    showTransferLog();
    refreshTransferStatus();
    CWUI.toast(`Lote enfileirado (${items.length} itens)`, "success");
  }

  rebindBtn("#btn-batch-send", () => batchTransfer("push"));
  rebindBtn("#btn-batch-receive", () => batchTransfer("pull"));

  rebindBtn("#btn-send", async () => {
    const items = selectedEntries("local");
    if (!items.length && state.selLocal && state.selLocal.name !== "..") items.push(state.selLocal);
    if (!items.length) {
      CWUI.toast(window.CWI18n?.t("explorer.toast.selectLocal") || "Selecione um arquivo ou pasta no painel local.", "error");
      return;
    }
    if (!(await CWConfirm(`Enviar ${items.length} item(ns) para ${state.remotePath}?`))) return;
    try {
      await transferPushEntries(items);
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  rebindBtn("#btn-receive", async () => {
    const items = selectedEntries("remote");
    if (!items.length && state.selRemote && state.selRemote.name !== "..") items.push(state.selRemote);
    if (!items.length) {
      CWUI.toast("Selecione um item no painel remoto.", "error");
      return;
    }
    if (!(await CWConfirm(`Receber ${items.length} item(ns) para ${state.localPath}?`))) return;
    try {
      await transferPullEntries(items);
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  rebindBtn("#btn-compare-folders", async () => {
    const rep = await api(
      `/api/explorer/compare?localPath=${encodeURIComponent(state.localPath)}&remotePath=${encodeURIComponent(state.remotePath)}${remoteApiExtra()}`
    );
    state.compareReport = rep;
    $("#compare-report").textContent = rep.text || rep.Text || "";
    const sum = $("#compare-summary");
    if (sum) {
      sum.textContent = `Só local: ${(rep.onlyLeftItems || []).length} · Só remoto: ${(rep.onlyRightItems || []).length} · Diferentes: ${(rep.mismatchItems || []).length}`;
    }
    renderCompareSections(rep);
    $("#compare-dialog")?.showModal();
  });

  function renderCompareSections(rep) {
    const box = $("#compare-sections");
    if (!box) return;
    box.innerHTML = "";
    const addSection = (title, items, actionLabel, actionFn) => {
      if (!items?.length) return;
      const sec = document.createElement("section");
      sec.className = "compare-section";
      sec.innerHTML = `<h4>${escapeHtml(title)} (${items.length})</h4>`;
      const ul = document.createElement("ul");
      ul.className = "compare-item-list";
      for (const it of items) {
        const li = document.createElement("li");
        li.innerHTML = `<span>${escapeHtml(it.name)}${it.isDir ? " /" : ""}</span>`;
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "btn btn-ghost btn-sm";
        btn.textContent = actionLabel;
        btn.addEventListener("click", () => actionFn(it));
        li.appendChild(btn);
        ul.appendChild(li);
      }
      sec.appendChild(ul);
      box.appendChild(sec);
    };
    addSection("Só no local", rep.onlyLeftItems || [], "Enviar →", async (it) => {
      const path = joinPath(state.localPath, it.name, "local");
      await api("/api/transfer/push", {
        method: "POST",
        body: JSON.stringify({
          localPath: path,
          remotePath: joinPath(state.remotePath, it.name, "remote"),
          containerId: state.remoteTarget === "container" ? state.remoteContainerId : undefined,
        }),
      });
      CWUI.toast(`Enviar ${it.name} enfileirado`, "success");
      refreshTransferStatus();
    });
    addSection("Só no remoto", rep.onlyRightItems || [], "← Receber", async (it) => {
      const path = joinPath(state.remotePath, it.name, "remote");
      await api("/api/transfer/pull", {
        method: "POST",
        body: JSON.stringify({
          localPath: state.localPath,
          remotePath: path,
          containerId: state.remoteTarget === "container" ? state.remoteContainerId : undefined,
        }),
      });
      CWUI.toast(`Receber ${it.name} enfileirado`, "success");
      refreshTransferStatus();
    });
    addSectionMismatch(rep.mismatchItems || []);
  }

  function addSectionMismatch(items) {
    if (!items?.length) return;
    const box = $("#compare-sections");
    if (!box) return;
    const sec = document.createElement("section");
    sec.className = "compare-section";
    sec.innerHTML = `<h4>${escapeHtml(window.CWI18n?.t?.("compare.diff") || "Diferentes")} (${items.length})</h4>`;
    const ul = document.createElement("ul");
    ul.className = "compare-item-list";
    for (const it of items) {
      const li = document.createElement("li");
      li.innerHTML = `<span>${escapeHtml(it.name)}${it.isDir ? " /" : ""}</span>`;
      const push = document.createElement("button");
      push.type = "button";
      push.className = "btn btn-ghost btn-sm";
      push.textContent = "Enviar →";
      push.addEventListener("click", async () => {
        const path = joinPath(state.localPath, it.name, "local");
        await api("/api/transfer/push", {
          method: "POST",
          body: JSON.stringify({
            localPath: path,
            remotePath: joinPath(state.remotePath, it.name, "remote"),
            containerId: state.remoteTarget === "container" ? state.remoteContainerId : undefined,
          }),
        });
        CWUI.toast(`Enviar ${it.name} enfileirado`, "success");
        refreshTransferStatus();
      });
      const pull = document.createElement("button");
      pull.type = "button";
      pull.className = "btn btn-ghost btn-sm";
      pull.textContent = "← Receber";
      pull.addEventListener("click", async () => {
        const path = joinPath(state.remotePath, it.name, "remote");
        await api("/api/transfer/pull", {
          method: "POST",
          body: JSON.stringify({
            localPath: joinPath(state.localPath, it.name, "local"),
            remotePath: path,
            containerId: state.remoteTarget === "container" ? state.remoteContainerId : undefined,
          }),
        });
        CWUI.toast(`Receber ${it.name} enfileirado`, "success");
        refreshTransferStatus();
      });
      li.appendChild(push);
      li.appendChild(pull);
      ul.appendChild(li);
    }
    sec.appendChild(ul);
    box.appendChild(sec);
  }

  $("#compare-dialog-close")?.addEventListener("click", () => $("#compare-dialog")?.close());

  $("#remote-panel-alert-sudo")?.addEventListener("click", () => $("#btn-sudo")?.click());

  // Ordenação por coluna
  $$(".file-list-head .file-col-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      const head = btn.closest(".file-list-head");
      const side = head?.dataset.side;
      if (!side) return;
      const col = btn.dataset.col;
      if (state.sort[side].col === col) state.sort[side].asc = !state.sort[side].asc;
      else {
        state.sort[side].col = col;
        state.sort[side].asc = true;
      }
      head.querySelectorAll(".file-col-btn").forEach((b) => b.classList.toggle("active", b === btn));
      const entries = side === "local" ? state.localEntries : state.remoteEntries;
      renderEntries(listEl(side), entries, side);
    });
  });

  // Filtro de contêineres
  $("#remote-container-filter")?.addEventListener("input", (e) => {
    const q = e.target.value.trim().toLowerCase();
    const sel = $("#remote-container");
    if (!sel) return;
    const cur = sel.value;
    sel.innerHTML = "";
    const ph = document.createElement("option");
    ph.value = "";
    ph.textContent = "Selecione um contêiner…";
    sel.appendChild(ph);
    const list = (state.containerCatalog || []).filter((c) => {
      const hay = `${c.name} ${c.image} ${c.id}`.toLowerCase();
      return !q || hay.includes(q);
    });
    for (const c of list) {
      const opt = document.createElement("option");
      opt.value = c.idFull || c.id;
      opt.textContent = c.label;
      opt.title = c.title;
      sel.appendChild(opt);
    }
    if (cur && [...sel.options].some((o) => o.value === cur)) sel.value = cur;
  });

  const origLoadContainers = async function () {
    try {
      const data = await api("/api/docker/containers");
      state.containerCatalog = (data.containers || []).map((c) => ({
        id: c.id,
        idFull: c.idFull || c.id,
        name: c.name || c.id,
        image: c.image || "",
        label: (c.name || c.id).replace(/^\//, "").slice(0, 18),
        title: `${c.name || c.id} — ${c.image}`,
      }));
    } catch {
      state.containerCatalog = [];
    }
  };

  // Wrap setRemoteTarget to show container filter
  const origSetRemote = null;
  $("#remote-target-host")?.addEventListener("click", () => {
    $("#remote-container-filter")?.classList.add("hidden");
    refreshExplorerNav();
  });
  $("#remote-target-container")?.addEventListener("click", () => {
    $("#remote-container-filter")?.classList.remove("hidden");
    origLoadContainers();
    refreshExplorerNav();
  });

  // Drag entre painéis
  let dragPayload = null;
  document.addEventListener("dragstart", (e) => {
    const row = e.target.closest?.(".file-row");
    if (!row) return;
    const side = row.dataset.side;
    dragPayload = { side, path: row.dataset.path, name: row.dataset.name, isDir: row.dataset.isDir === "1" };
    e.dataTransfer.setData("text/plain", row.dataset.path);
    e.dataTransfer.effectAllowed = "copy";
  });
  document.addEventListener("dragend", () => {
    dragPayload = null;
    $$(".drop-zone").forEach((z) => z.classList.remove("drop-over"));
  });
  $$(".drop-zone").forEach((zone) => {
    zone.addEventListener("dragover", (e) => {
      e.preventDefault();
      zone.classList.add("drop-over");
      e.dataTransfer.dropEffect = "copy";
    });
    zone.addEventListener("dragleave", () => zone.classList.remove("drop-over"));
    zone.addEventListener("drop", async (e) => {
      e.preventDefault();
      zone.classList.remove("drop-over");
      const targetSide = zone.dataset.dropSide;
      if (e.dataTransfer.files?.length) {
        if (targetSide !== "remote") {
          CWUI.toast("Solte arquivos no painel Remoto para enviar.", "info");
          return;
        }
        if (!state.ssh.connected) return;
        const form = new FormData();
        form.append("remoteDir", state.remotePath || "/");
        if (state.remoteTarget === "container" && state.remoteContainerId) {
          form.append("containerId", state.remoteContainerId);
        }
        try {
          const fileList = window.cwCollectDropFiles
            ? await window.cwCollectDropFiles(e.dataTransfer)
            : [...e.dataTransfer.files];
          const data = window.cwUploadToRemote
            ? await window.cwUploadToRemote(fileList)
            : await (async () => {
                for (const f of fileList) form.append("files", f);
                const res = await fetch("/api/transfer/upload", { method: "POST", body: form, credentials: "same-origin" });
                const d = await res.json().catch(() => ({}));
                if (!res.ok) throw new Error(d.error || res.statusText);
                return d;
              })();
          CWUI.toast(`Upload enfileirado (${data.count || fileList.length || "?"})`, "success");
          showTransferLog();
          refreshTransferStatus();
          loadRemote(state.remotePath);
        } catch (err) {
          CWUI.toast(err.message, "error");
        }
        return;
      }
      if (!dragPayload) return;
      const from = dragPayload.side;
      if (from === targetSide) return;
      try {
        if (from === "local" && targetSide === "remote") {
          const entry = (state.localEntries || []).find((x) => x.path === dragPayload.path);
          if (entry) {
            if (!(await CWConfirm(`Enviar ${entry.name} para o remoto?`))) return;
            await transferPushEntries([entry]);
          }
        } else if (from === "remote" && targetSide === "local") {
          const entry = (state.remoteEntries || []).find((x) => x.path === dragPayload.path);
          if (entry) {
            if (!(await CWConfirm(`Receber ${entry.name} para o local?`))) return;
            await transferPullEntries([entry]);
          }
        }
      } catch (err) {
        CWUI.toast(err.message, "error");
      }
    });
  });

  const PREVIEW_INLINE_MAX = 12000;
  const PREVIEW_OPEN_KEY = "cw-preview-open";

  function isPreviewPanelOpen() {
    return localStorage.getItem(PREVIEW_OPEN_KEY) === "1";
  }

  function setPreviewPanelOpen(open) {
    if (open) localStorage.setItem(PREVIEW_OPEN_KEY, "1");
    else localStorage.removeItem(PREVIEW_OPEN_KEY);
  }

  function applyPreviewPanelVisibility() {
    const open = isPreviewPanelOpen();
    $("#explorer-preview")?.classList.toggle("hidden", !open);
    $("#explorer-splitter-preview")?.classList.toggle("hidden", !open);
    const btn = $("#btn-toggle-preview");
    if (btn) {
      btn.classList.toggle("is-active", open);
      btn.setAttribute("aria-pressed", open ? "true" : "false");
      btn.title = open ? "Ocultar pré-visualização" : "Mostrar pré-visualização";
    }
  }

  function setPreviewFullscreenEnabled(on) {
    const btn = $("#btn-preview-fullscreen");
    if (btn) btn.disabled = !on;
  }

  function renderPreviewInto(target, cache) {
    if (!target || !cache) return;
    target.innerHTML = "";
    target.classList.remove("muted");
    if (cache.kind === "image") {
      const img = document.createElement("img");
      img.className = "preview-img";
      img.alt = cache.name || "";
      img.src = cache.imgSrc;
      target.appendChild(img);
    } else if (cache.kind === "text") {
      const pre = document.createElement("pre");
      pre.className = "preview-text";
      pre.textContent = cache.text;
      target.appendChild(pre);
    } else {
      target.classList.add("muted");
      target.innerHTML = cache.html || "";
    }
  }

  function openPreviewFullscreen() {
    const cache = state.previewCache;
    if (!cache || cache.kind === "meta") return;
    const dlg = $("#preview-fullscreen-dialog");
    const body = $("#preview-fullscreen-body");
    const title = $("#preview-fullscreen-title");
    if (!dlg || !body) return;
    if (title) title.textContent = cache.name || "Pré-visualização";
    renderPreviewInto(body, cache);
    dlg.showModal();
  }

  // Pré-visualização
  function schedulePreview(side) {
    const token = ++state.previewToken;
    const items = selectedEntries(side);
    const entry = items.length === 1 ? items[0] : items.length === 0 ? (side === "local" ? state.selLocal : state.selRemote) : null;
    const panel = $("#explorer-preview");
    const content = $("#preview-content");
    if (!panel || !content) return;
    state.previewCache = null;
    setPreviewFullscreenEnabled(false);
    if (!entry || entry.isDir || entry.name === "..") {
      if (isPreviewPanelOpen()) content.innerHTML = '<span class="muted">Selecione um arquivo</span>';
      return;
    }
    const ext = entry.name.split(".").pop()?.toLowerCase() || "";
    const isImg = /^(png|jpe?g|gif|webp|svg|ico|bmp)$/.test(ext);
    const isTxt = /^(txt|md|json|ya?ml|xml|log|conf|ini|sh|bat|ps1|go|js|ts|css|html?|env)$/.test(ext);
    if (!isImg && !isTxt) {
      const html = `<p><strong>${escapeHtml(entry.name)}</strong></p><p class="muted">${entry.isDir ? "Pasta" : formatSize(entry.size)}</p>`;
      state.previewCache = { kind: "meta", name: entry.name, html };
      if (isPreviewPanelOpen()) content.innerHTML = html;
      return;
    }
    if (isPreviewPanelOpen()) content.innerHTML = '<span class="muted">A carregar…</span>';
    (async () => {
      try {
        let url;
        if (side === "local") url = `/api/local/file?path=${encodeURIComponent(entry.path)}`;
        else {
          url = `/api/remote/file?path=${encodeURIComponent(entry.path)}`;
          if (state.remoteTarget === "container" && state.remoteContainerId) {
            url += `&containerId=${encodeURIComponent(state.remoteContainerId)}`;
          }
        }
        const data = await api(url);
        if (token !== state.previewToken) return;
        if (data.encoding === "base64" && (data.mimeType || "").startsWith("image/")) {
          const imgSrc = `data:${data.mimeType};base64,${data.content}`;
          state.previewCache = { kind: "image", name: entry.name, imgSrc };
          setPreviewFullscreenEnabled(true);
        } else if (typeof data.content === "string") {
          state.previewCache = { kind: "text", name: entry.name, text: data.content };
          setPreviewFullscreenEnabled(true);
        }
        if (!isPreviewPanelOpen() || token !== state.previewToken || !state.previewCache) return;
        if (state.previewCache.kind === "text") {
          const full = state.previewCache.text;
          const truncated = full.length > PREVIEW_INLINE_MAX;
          const pre = document.createElement("pre");
          pre.className = "preview-text";
          pre.textContent = truncated ? full.slice(0, PREVIEW_INLINE_MAX) + "\n… (use tela cheia para ver tudo)" : full;
          content.innerHTML = "";
          content.appendChild(pre);
        } else {
          renderPreviewInto(content, state.previewCache);
        }
      } catch {
        if (token === state.previewToken && isPreviewPanelOpen()) {
          content.innerHTML = '<span class="muted">Pré-visualização indisponível</span>';
        }
      }
    })();
  }

  function togglePreviewPanel(forceOpen) {
    const open = forceOpen === true ? true : forceOpen === false ? false : !isPreviewPanelOpen();
    setPreviewPanelOpen(open);
    applyPreviewPanelVisibility();
    if (open) schedulePreview(state.explorerFocus || "local");
  }

  $("#btn-preview-fullscreen")?.addEventListener("click", openPreviewFullscreen);
  $("#btn-preview-fullscreen-close")?.addEventListener("click", () => $("#preview-fullscreen-dialog")?.close());
  $("#preview-fullscreen-dialog")?.addEventListener("click", (e) => {
    if (e.target === e.currentTarget) e.currentTarget.close();
  });
  $("#btn-preview-close")?.addEventListener("click", () => togglePreviewPanel(false));
  $("#btn-toggle-preview")?.addEventListener("click", () => togglePreviewPanel());
  $("#explorer-splitter-preview")?.addEventListener("dblclick", () => togglePreviewPanel());

  // Splitter vertical entre painéis
  (function initVSplitter() {
    const split = $("#explorer-splitter-v");
    const wrap = $("#explorer-panels");
    if (!split || !wrap) return;
    const key = "cw-explorer-split";
    const saved = localStorage.getItem(key);
    if (saved) wrap.style.gridTemplateColumns = saved;
    let dragging = false;
    split.addEventListener("mousedown", (e) => {
      dragging = true;
      e.preventDefault();
    });
    window.addEventListener("mousemove", (e) => {
      if (!dragging) return;
      const rect = wrap.getBoundingClientRect();
      const pct = Math.min(75, Math.max(25, ((e.clientX - rect.left) / rect.width) * 100));
      wrap.style.gridTemplateColumns = `${pct}% 5px ${100 - pct}%`;
    });
    window.addEventListener("mouseup", () => {
      if (!dragging) return;
      dragging = false;
      localStorage.setItem(key, wrap.style.gridTemplateColumns);
    });
  })();

  // Dock de transferências — altura redimensionável
  (function initXferDock() {
    const dock = $("#explorer-xfer-dock");
    const body = $("#explorer-xfer-dock-body");
    if (!dock || !body) return;
    const key = "cw-xfer-dock-h";
    const h = localStorage.getItem(key);
    if (h) body.style.maxHeight = h;
    function syncXferDockToggle() {
      const collapsed = dock.classList.contains("collapsed");
      const btn = $("#btn-xfer-dock-toggle");
      if (btn) {
        btn.textContent = collapsed ? "▴" : "▾";
        btn.title = collapsed ? "Expandir" : "Recolher";
        btn.setAttribute("aria-expanded", collapsed ? "false" : "true");
      }
    }
    $("#btn-xfer-dock-toggle")?.addEventListener("click", () => {
      dock.classList.toggle("collapsed");
      syncXferDockToggle();
    });
    syncXferDockToggle();
    window.syncXferDockToggle = syncXferDockToggle;
    let resizing = false;
    const handle = document.createElement("div");
    handle.className = "explorer-xfer-dock-resize";
    dock.insertBefore(handle, body);
    handle.addEventListener("mousedown", (e) => {
      resizing = true;
      e.preventDefault();
    });
    window.addEventListener("mousemove", (e) => {
      if (!resizing) return;
      const rect = dock.getBoundingClientRect();
      const maxH = Math.min(280, window.innerHeight - rect.top - 40);
      const nh = Math.max(48, maxH - (e.clientY - rect.top));
      body.style.maxHeight = `${nh}px`;
    });
    window.addEventListener("mouseup", () => {
      if (!resizing) return;
      resizing = false;
      localStorage.setItem(key, body.style.maxHeight);
    });
  })();

  // Atalhos de teclado no explorador
  document.addEventListener("keydown", (e) => {
    if (state.screen !== "explorer") return;
    if (e.target.closest("dialog[open]")) return;
    if (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA" || e.target.tagName === "SELECT") return;
    const side = state.explorerFocus;
    if (e.key === "Tab") {
      e.preventDefault();
      state.explorerFocus = side === "local" ? "remote" : "local";
      highlightRows(state.explorerFocus);
      return;
    }
    if (e.key === "F5") {
      e.preventDefault();
      if (side === "local") loadLocal(state.localPath);
      else loadRemote(state.remotePath);
      return;
    }
    if (e.key === "Backspace") {
      e.preventDefault();
      const entries = side === "local" ? state.localEntries : state.remoteEntries;
      const parent = (entries || []).find((x) => x.name === "..");
      if (parent) {
        if (side === "local") loadLocal(parent.path);
        else loadRemote(parent.path);
      }
      return;
    }
    if (e.key === "F2") {
      e.preventDefault();
      $("#btn-rename-item")?.click();
      return;
    }
    if (e.key === "Delete") {
      e.preventDefault();
      $("#btn-delete-item")?.click();
      return;
    }
    if (e.key === "Enter") {
      const sel = side === "local" ? state.selLocal : state.selRemote;
      if (!sel) return;
      e.preventDefault();
      if (sel.isDir) {
        if (side === "local") loadLocal(sel.path);
        else loadRemote(sel.path);
      } else openFileEditor(side, sel);
      return;
    }
    if (e.key === "F6") {
      e.preventDefault();
      if (e.shiftKey) {
        if (side === "local") $("#btn-batch-send")?.click();
        else $("#btn-batch-receive")?.click();
      } else if (side === "local") $("#btn-send")?.click();
      else $("#btn-receive")?.click();
    }
  });

  // Patch loadRemote catch for permission in explorer.js - handled via mutation observer

  const origShow = window.showScreen;
  if (origShow) {
    window.showScreen = function (n) {
      origShow(n);
      if (n === "explorer") {
        refreshExplorerNav();
        applyPreviewPanelVisibility();
      }
    };
  }

  // Hook container load to fill catalog
  const remoteSel = $("#remote-container");
  if (remoteSel) {
    const mo = new MutationObserver(() => {
      if (state.remoteTarget === "container" && !state.containerCatalog.length) origLoadContainers();
    });
    mo.observe(remoteSel, { childList: true });
  }

  applyPreviewPanelVisibility();
  refreshExplorerNav();
})();
