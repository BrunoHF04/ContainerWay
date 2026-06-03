/** Completude explorador — filtro avançado, Ctrl+C/V, upload pastas, histórico, sync, i18n, monitor externo. */
(function () {
  const U = window.CWUI;
  if (!U) return;

  function parsePanelFilter(raw) {
    const q = String(raw || "").trim();
    if (!q) return { text: "", ext: null, kind: null };
    let text = q;
    let ext = null;
    let kind = null;
    const extM = q.match(/\bext:\.?([a-z0-9]+)/i);
    if (extM) {
      ext = extM[1].toLowerCase();
      text = text.replace(extM[0], "").trim();
    }
    const tipoM = q.match(/\b(tipo|type):\s*(dir|pasta|folder|file|ficheiro|fichero|arquivo)/i);
    if (tipoM) {
      const v = tipoM[1].toLowerCase();
      kind = /dir|pasta|folder/.test(v) ? "dir" : "file";
      text = text.replace(tipoM[0], "").trim();
    }
    return { text: text.toLowerCase(), ext, kind };
  }

  window.cwPanelFilter = function cwPanelFilter(entries, q) {
    const spec = parsePanelFilter(q);
    if (!spec.text && !spec.ext && !spec.kind) return entries || [];
    return (entries || []).filter((e) => {
      if (e.name === "..") return true;
      if (spec.kind === "dir" && !e.isDir) return false;
      if (spec.kind === "file" && e.isDir) return false;
      if (spec.ext) {
        const dot = e.name.lastIndexOf(".");
        const ex = dot >= 0 ? e.name.slice(dot + 1).toLowerCase() : "";
        if (ex !== spec.ext.replace(/^\./, "")) return false;
      }
      if (spec.text && !e.name.toLowerCase().includes(spec.text)) return false;
      return true;
    });
  };

  async function collectEntryFiles(entry, basePath, out) {
    if (entry.isFile) {
      return new Promise((resolve) => {
        entry.file((file) => {
          if (basePath) {
            try {
              Object.defineProperty(file, "webkitRelativePath", { value: basePath + file.name, configurable: true });
            } catch {
              /* ignore */
            }
          }
          out.push(file);
          resolve();
        });
      });
    }
    if (entry.isDirectory) {
      const reader = entry.createReader();
      const readBatch = () =>
        new Promise((resolve) => {
          reader.readEntries(async (ents) => {
            if (!ents.length) {
              resolve();
              return;
            }
            for (const ch of ents) {
              await collectEntryFiles(ch, basePath + entry.name + "/", out);
            }
            await readBatch();
            resolve();
          });
        });
      await readBatch();
    }
  }

  window.cwCollectDropFiles = async function cwCollectDropFiles(dataTransfer) {
    const files = [];
    const items = dataTransfer.items;
    if (items?.length && items[0].webkitGetAsEntry) {
      for (let i = 0; i < items.length; i++) {
        const entry = items[i].webkitGetAsEntry?.();
        if (entry) await collectEntryFiles(entry, "", files);
      }
      if (files.length) return files;
    }
    return [...(dataTransfer.files || [])];
  };

  window.cwUploadToRemote = async function cwUploadToRemote(fileList) {
    const form = new FormData();
    form.append("remoteDir", state.remotePath || "/");
    if (state.remoteTarget === "container" && state.remoteContainerId) {
      form.append("containerId", state.remoteContainerId);
    }
    for (const f of fileList) {
      form.append("files", f);
      const rel = f.webkitRelativePath || f.name;
      if (rel && rel !== f.name) form.append("paths", rel);
    }
    const res = await fetch("/api/transfer/upload", { method: "POST", body: form, credentials: "same-origin" });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || res.statusText);
    return data;
  };

  // Modo compacto
  $("#btn-compact-explorer")?.addEventListener("click", () => {
    const view = $("#view-explorer");
    if (!view) return;
    view.classList.toggle("explorer-compact");
    const on = view.classList.contains("explorer-compact");
    window.CWWebPrefs?.set?.("explorerCompactToolbar", on);
  });
  if (window.CWWebPrefs?.get?.("explorerCompactToolbar")) {
    $("#view-explorer")?.classList.add("explorer-compact");
  }

  // Seletor idioma
  $("#explorer-lang")?.addEventListener("change", (e) => {
    window.CWI18n?.setLang?.(e.target.value);
  });
  const langSel = $("#explorer-lang");
  if (langSel && window.CWI18n) langSel.value = window.CWI18n.lang();

  function firstSelected(side) {
    const set = side === "local" ? state.selLocalPaths : state.selRemotePaths;
    const entries = side === "local" ? state.localEntries : state.remoteEntries;
    if (set?.size) {
      const path = set.values().next().value;
      const hit = (entries || []).find((e) => e.path === path);
      if (hit) return hit;
    }
    return side === "local" ? state.selLocal : state.selRemote;
  }

  // Sync espelhada
  $("#btn-sync-mirror")?.addEventListener("click", async () => {
    if (!state.ssh.connected) {
      U.toast("Conecte-se ao SSH.", "error");
      return;
    }
    const direction = await new Promise((resolve) => {
      const dlg = document.querySelector("#sync-mirror-dialog");
      if (!dlg) {
        resolve("both");
        return;
      }
      const done = (v) => {
        dlg.close();
        resolve(v);
      };
      dlg.querySelector("#sync-mirror-push")?.addEventListener("click", () => done("push"), { once: true });
      dlg.querySelector("#sync-mirror-pull")?.addEventListener("click", () => done("pull"), { once: true });
      dlg.querySelector("#sync-mirror-both")?.addEventListener("click", () => done("both"), { once: true });
      dlg.querySelector("#sync-mirror-cancel")?.addEventListener("click", () => done(null), { once: true });
      dlg.showModal();
    });
    if (!direction) return;
    try {
      const data = await api("/api/explorer/sync-mirror", {
        method: "POST",
        body: JSON.stringify({
          localPath: state.localPath,
          remotePath: state.remotePath,
          containerId: state.remoteTarget === "container" ? state.remoteContainerId : "",
          direction,
        }),
      });
      U.toast(
        window.CWI18n?.t?.("toast.syncQueued") || `Sync: ${data.count ?? "?"} tarefa(s)`,
        "success"
      );
      showTransferLog?.();
      refreshTransferStatus?.();
    } catch (e) {
      U.toast(e.message, "error");
    }
  });

  // Cancelar fila
  $("#btn-cancel-xfer")?.addEventListener("click", async () => {
    if (!(await CWConfirm("Cancelar todas as transferências pendentes?", { danger: true, ok: "Cancelar fila" })))
      return;
    try {
      const data = await api("/api/transfer/cancel", { method: "POST", body: "{}" });
      U.toast(
        `${window.CWI18n?.t?.("toast.queueCleared") || "Fila cancelada"} (${data.cleared ?? 0})`,
        "info"
      );
      refreshTransferStatus?.();
      refreshOpHistory();
    } catch (e) {
      U.toast(e.message, "error");
    }
  });

  // Histórico de operações
  async function refreshOpHistory() {
    const ul = $("#explorer-op-log");
    if (!ul || !state.ssh.connected) return;
    try {
      const data = await api("/api/explorer/operations");
      const lines = data.lines || [];
      ul.innerHTML = "";
      if (!lines.length) {
        ul.innerHTML = '<li class="muted">Sem operações recentes</li>';
        return;
      }
      for (const line of lines.slice(-40).reverse()) {
        const li = document.createElement("li");
        li.className = `op-${line.level || "info"}`;
        li.textContent = `${line.time ? new Date(line.time).toLocaleTimeString() : ""} ${line.message || ""}`.trim();
        ul.appendChild(li);
      }
    } catch {
      /* ignore */
    }
  }

  $("#btn-clear-op-log")?.addEventListener("click", async () => {
    await api("/api/explorer/operations", { method: "DELETE" });
    refreshOpHistory();
  });

  setInterval(() => {
    if (state.screen === "explorer" && state.ssh.connected) refreshOpHistory();
  }, 12000);

  const origRefreshXfer = window.refreshTransferStatus;
  if (typeof origRefreshXfer === "function") {
    window.refreshTransferStatus = async function () {
      await origRefreshXfer.apply(this, arguments);
      refreshOpHistory();
    };
  }

  // Ctrl+C / Ctrl+V no explorador
  document.addEventListener("keydown", async (e) => {
    if (state.screen !== "explorer") return;
    if (e.target.closest("dialog[open]")) return;
    if (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA" || e.target.tagName === "SELECT") return;
    const side = state.explorerFocus || "local";
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "c") {
      e.preventDefault();
      const sel = firstSelected(side);
      if (!sel || sel.name === "..") {
        U.toast("Selecione um item.", "error");
        return;
      }
      const source = side === "local" ? "local" : state.remoteTarget === "container" ? "container" : "host";
      await api("/api/explorer/clipboard", {
        method: "POST",
        body: JSON.stringify({
          source,
          path: sel.path,
          isDir: !!sel.isDir,
          containerId: side === "remote" && state.remoteTarget === "container" ? state.remoteContainerId : "",
        }),
      });
      U.toast("Copiado", "info");
    }
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "v") {
      e.preventDefault();
      const targetSource = side === "local" ? "local" : state.remoteTarget === "container" ? "container" : "host";
      await api("/api/explorer/paste", {
        method: "POST",
        body: JSON.stringify({
          targetSource,
          targetPath: side === "local" ? state.localPath : state.remotePath,
          targetContainerId: side === "remote" && state.remoteTarget === "container" ? state.remoteContainerId : "",
        }),
      });
      U.toast("Operação enfileirada", "success");
      if (typeof showTransferLog === "function") showTransferLog();
      refreshTransferStatus?.();
    }
  });

  // Monitor edição externa remota
  let externalPoll = null;
  function stopExternalPoll() {
    if (externalPoll) clearInterval(externalPoll);
    externalPoll = null;
  }
  function startExternalPoll() {
    stopExternalPoll();
    externalPoll = setInterval(async () => {
      const sid = state.fileEdit?.externalSessionId;
      if (!sid || state.fileEdit?.side !== "remote") return;
      try {
        const st = await api(`/api/remote/external-status?sessionId=${encodeURIComponent(sid)}`);
        const syncBtn = $("#file-editor-sync-external");
        const hint = $("#file-editor-hint");
        if (st.modified) {
          syncBtn?.classList.remove("hidden");
          syncBtn?.classList.add("pulse-hint");
          if (hint && !hint.dataset.externalWarn) {
            hint.dataset.externalWarn = "1";
            hint.textContent = window.CWI18n?.t?.("external.modified") || hint.textContent;
            hint.classList.remove("hidden");
          }
        }
      } catch {
        /* ignore */
      }
    }, 4000);
  }

  const dlgEditor = $("#file-editor-dialog");
  if (dlgEditor) {
    dlgEditor.addEventListener("close", stopExternalPoll);
  }
  const origOpenEditor = window.openFileEditor;
  if (origOpenEditor) {
    window.openFileEditor = async function (...args) {
      await origOpenEditor.apply(this, args);
      if (state.fileEdit?.externalSessionId) startExternalPoll();
      else stopExternalPoll();
    };
  }

  $("#file-editor-sync-external")?.addEventListener(
    "click",
    () => {
      $("#file-editor-sync-external")?.classList.remove("pulse-hint");
    },
    { capture: true }
  );

  document.addEventListener("cw-lang-change", () => window.CWI18n?.applyLabels?.());

  refreshOpHistory();
})();
