/** Otimizador docker-compose / Swarm no host SSH. */
(function () {
  const $ = (sel) => document.querySelector(sel);
  const tr = (k, v) => (window.CWI18n && window.CWI18n.t(k, v)) || k;
  const LS_ROOTS = "cw-copt-roots";
  const PRESET_ROOTS = ["/opt", "/opt/siplan", "/home", "/srv", "/var/www", "/etc/docker"];

  const escapeHtml = (s) => {
    const d = document.createElement("div");
    d.textContent = s ?? "";
    return d.innerHTML;
  };

  function normYamlText(s) {
    return String(s ?? "")
      .replace(/\r\n/g, "\n")
      .replace(/\r/g, "\n")
      .replace(/\u00a0/g, " ")
      .replace(/^\uFEFF/, "");
  }

  function stripYamlQuotes(v) {
    const t = v.trim();
    if (t.length >= 2) {
      const q = t[0];
      if ((q === "'" || q === '"') && t.endsWith(q)) return t.slice(1, -1);
    }
    return t;
  }

  /** Chave de comparação: ignora aspas e espaços no fim (mesmo conteúdo YAML). */
  function yamlLineKey(line) {
    let s = line.trimEnd();
    const hash = s.indexOf(" #");
    if (hash >= 0) s = s.slice(0, hash).trimEnd();
    const kv = s.match(/^(\s*)([A-Za-z0-9_.-]+):\s*(.*)$/);
    if (kv) {
      return `${kv[1]}${kv[2]}:${stripYamlQuotes(kv[3])}`;
    }
    const li = s.match(/^(\s*)-\s+(.*)$/);
    if (li) return `${li[1]}- ${stripYamlQuotes(li[2])}`;
    return s;
  }

  function linesEqual(a, b) {
    if (a === b) return true;
    if (a.trimEnd() === b.trimEnd()) return true;
    return yamlLineKey(a) === yamlLineKey(b);
  }

  /** Pares (i,j) de linhas idênticas via LCS (só para casar, não para empilhar adds/dels). */
  function lcsMatchPairs(a, b) {
    const n = a.length;
    const m = b.length;
    const dp = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
    for (let i = n - 1; i >= 0; i--) {
      for (let j = m - 1; j >= 0; j--) {
        dp[i][j] = linesEqual(a[i], b[j])
          ? dp[i + 1][j + 1] + 1
          : Math.max(dp[i + 1][j], dp[i][j + 1]);
      }
    }
    const pairs = [];
    let i = 0;
    let j = 0;
    while (i < n && j < m) {
      if (linesEqual(a[i], b[j])) {
        pairs.push({ i, j });
        i++;
        j++;
      } else if (dp[i + 1][j] > dp[i][j + 1]) {
        i++;
      } else if (dp[i][j + 1] > dp[i + 1][j]) {
        j++;
      } else {
        i++;
      }
    }
    return pairs;
  }

  /**
   * Diff espelhado: mesma linha nos dois painéis.
   * Iguais sem cor; alteração = vermelho|verde na mesma linha; só sobra pad no fim.
   */
  function computeAlignedYamlDiff(oldStr, newStr) {
    const a = normYamlText(oldStr).split("\n");
    const b = normYamlText(newStr).split("\n");
    const pairs = lcsMatchPairs(a, b);
    const left = [];
    const right = [];
    let i = 0;
    let j = 0;
    let p = 0;
    const pushDel = (text) => {
      left.push({ text, type: "del" });
      right.push({ text: "", type: "pad" });
    };
    const pushAdd = (text) => {
      left.push({ text: "", type: "pad" });
      right.push({ text, type: "add" });
    };
    const pushPair = (oldLine, newLine) => {
      if (linesEqual(oldLine, newLine)) {
        left.push({ text: oldLine, type: "same" });
        right.push({ text: newLine, type: "same" });
      } else {
        left.push({ text: oldLine, type: "del" });
        right.push({ text: newLine, type: "add" });
      }
    };
    while (i < a.length || j < b.length) {
      if (p < pairs.length && i === pairs[p].i && j === pairs[p].j) {
        left.push({ text: a[i], type: "same" });
        right.push({ text: b[j], type: "same" });
        i++;
        j++;
        p++;
        continue;
      }
      const nextI = p < pairs.length ? pairs[p].i : a.length;
      const nextJ = p < pairs.length ? pairs[p].j : b.length;
      if (i < nextI && j < nextJ) {
        pushPair(a[i], b[j]);
        i++;
        j++;
      } else if (i < nextI) {
        pushDel(a[i]);
        i++;
      } else if (j < nextJ) {
        pushAdd(b[j]);
        j++;
      }
    }
    return { left, right };
  }

  const SAVE_CHANGE_PREVIEW_MAX = 40;
  const SAVE_LINE_PREVIEW_MAX = 140;

  function truncateLine(s, max = SAVE_LINE_PREVIEW_MAX) {
    const t = String(s ?? "");
    return t.length > max ? `${t.slice(0, max)}…` : t;
  }

  /** Resumo das diferenças entre original e sugerido (para confirmação de gravação). */
  function summarizeYamlChanges(original, optimized) {
    const { left, right } = computeAlignedYamlDiff(original, optimized);
    const items = [];
    let added = 0;
    let removed = 0;
    let modified = 0;
    for (let i = 0; i < left.length; i++) {
      const l = left[i];
      const r = right[i];
      if (l.type === "pad" || r.type === "pad") continue;
      if (l.type === "del" && r.type === "add" && l.text && r.text) {
        modified++;
        items.push({ kind: "mod", from: l.text, to: r.text });
      } else if (l.type === "del" && l.text) {
        removed++;
        items.push({ kind: "del", line: l.text });
      } else if (r.type === "add" && r.text) {
        added++;
        items.push({ kind: "add", line: r.text });
      }
    }
    return {
      added,
      removed,
      modified,
      items,
      noChanges: items.length === 0,
    };
  }

  function formatChangePreview(summary) {
    const lines = [];
    const show = summary.items.slice(0, SAVE_CHANGE_PREVIEW_MAX);
    for (const it of show) {
      if (it.kind === "mod") {
        lines.push(`~ ${truncateLine(it.from)}`);
        lines.push(`→ ${truncateLine(it.to)}`);
      } else if (it.kind === "del") {
        lines.push(`- ${truncateLine(it.line)}`);
      } else if (it.kind === "add") {
        lines.push(`+ ${truncateLine(it.line)}`);
      }
    }
    const rest = summary.items.length - show.length;
    if (rest > 0) lines.push(tr("copt.save.more", { n: rest }));
    return lines.join("\n");
  }

  function currentOptimizedText() {
    const main = $("#copt-yaml-opt-pre");
    const fs = $("#copt-fs-yaml-opt-pre");
    const el = document.activeElement === fs ? fs : document.activeElement === main ? main : null;
    if (el) {
      if (el.querySelector(".copt-line")) return normYamlText(copt.yamlOptimized);
      return normYamlText(el.innerText || copt.yamlOptimized);
    }
    return normYamlText(copt.yamlOptimized);
  }

  function hasCoptPendingChanges() {
    if (!copt.yamlOriginal) return false;
    return copt.yamlOriginal !== currentOptimizedText();
  }

  function updateSaveUndoState() {
    const changed = hasCoptPendingChanges();
    const canSave = changed && canWrite() && !!copt.selectedPath;
    ["#copt-save", "#copt-fs-save"].forEach((sel) => {
      const btn = $(sel);
      if (btn) btn.disabled = !canSave;
    });
    ["#copt-undo", "#copt-fs-undo"].forEach((sel) => {
      const btn = $(sel);
      if (btn) btn.disabled = !changed;
    });
  }

  function revertCoptChanges() {
    if (!copt.yamlOriginal || !hasCoptPendingChanges()) return;
    flushOptEditor();
    copt.yamlOptimized = copt.yamlOriginal;
    renderYamlDiff(copt.yamlOriginal, copt.yamlOriginal);
    updateSaveUndoState();
    CWUI?.toast?.(tr("copt.undo.ok"), "success", 3500);
  }

  function confirmCoptSave(path, summary) {
    return new Promise((resolve) => {
      const dlg = $("#copt-save-dialog");
      if (!dlg) {
        resolve(window.confirm(`${tr("copt.save.confirm")}\n\n${formatChangePreview(summary)}`));
        return;
      }
      const pathEl = $("#copt-save-path");
      const sumEl = $("#copt-save-summary");
      const changesEl = $("#copt-save-changes");
      if (pathEl) pathEl.textContent = tr("copt.save.path", { path });
      if (sumEl) {
        sumEl.textContent = tr("copt.save.summary", {
          modified: summary.modified,
          added: summary.added,
          removed: summary.removed,
        });
      }
      if (changesEl) changesEl.textContent = formatChangePreview(summary);
      let settled = false;
      const done = (v) => {
        if (settled) return;
        settled = true;
        dlg.close();
        resolve(v);
      };
      $("#copt-save-ok").onclick = () => done(true);
      $("#copt-save-cancel").onclick = () => done(false);
      dlg.onclose = () => {
        if (!settled) done(false);
      };
      dlg.showModal();
    });
  }

  function diffLineHtml(line, rowIdx) {
    const rowAttr = rowIdx >= 0 ? ` data-copt-row="${rowIdx}"` : "";
    if (line.type === "pad") {
      return `<div class="copt-line copt-line--pad"${rowAttr} aria-hidden="true">\u00a0</div>`;
    }
    const show = escapeHtml(line.text);
    if (line.type === "del") return `<div class="copt-line copt-line--del"${rowAttr}>${show}</div>`;
    if (line.type === "add") return `<div class="copt-line copt-line--add"${rowAttr}>${show}</div>`;
    return `<div class="copt-line copt-line--same"${rowAttr}>${show}</div>`;
  }

  function collectDiffChangeRows(left, right) {
    const rows = [];
    for (let i = 0; i < left.length; i++) {
      const l = left[i];
      const r = right[i];
      if (l.type === "pad" && r.type === "pad") continue;
      if (l.type !== "same" || r.type !== "same") rows.push(i);
    }
    return rows;
  }

  function clearDiffFocus() {
    document.querySelectorAll(".copt-line--focus").forEach((el) => el.classList.remove("copt-line--focus"));
  }

  function updateDiffNavUI() {
    const n = copt.diffChanges.length;
    const has = n > 0;
    ["#copt-diff-prev", "#copt-diff-next"].forEach((sel) => {
      const btn = $(sel);
      if (btn) btn.disabled = !has;
    });
    const label = $("#copt-diff-nav-label");
    if (label) {
      if (!has) {
        label.classList.add("hidden");
        label.textContent = "";
      } else {
        label.classList.remove("hidden");
        const cur = copt.diffChangeCursor >= 0 ? copt.diffChangeCursor + 1 : 0;
        label.textContent = tr("copt.diff.nav", { cur, total: n });
      }
    }
  }

  function scrollDiffToRow(row) {
    clearDiffFocus();
    const panes = ["#copt-yaml-orig-pre", "#copt-yaml-opt-pre", "#copt-fs-yaml-orig-pre", "#copt-fs-yaml-opt-pre"];
    panes.forEach((sel) => {
      const pane = $(sel);
      if (!pane) return;
      const el = pane.querySelector(`[data-copt-row="${row}"]`);
      if (!el) return;
      el.classList.add("copt-line--focus");
      const paneRect = pane.getBoundingClientRect();
      const elRect = el.getBoundingClientRect();
      const delta = elRect.top - paneRect.top - paneRect.height / 2 + elRect.height / 2;
      pane.scrollTop += delta;
    });
    updateDiffNavUI();
  }

  function navigateDiff(delta) {
    const n = copt.diffChanges.length;
    if (!n) return;
    if (copt.diffChangeCursor < 0) {
      copt.diffChangeCursor = delta > 0 ? 0 : n - 1;
    } else {
      copt.diffChangeCursor = (copt.diffChangeCursor + delta + n) % n;
    }
    scrollDiffToRow(copt.diffChanges[copt.diffChangeCursor]);
  }

  function getOptYamlText() {
    return currentOptimizedText();
  }

  function getYamlTextFrom(el) {
    if (!el) return "";
    return normYamlText(el.innerText || "");
  }

  function renderYamlDiffTargets(origPre, optPre, original, optimized) {
    const orig = normYamlText(original);
    const opt = normYamlText(optimized);
    const { left, right } = computeAlignedYamlDiff(orig, opt);
    const isMain = origPre?.id === "copt-yaml-orig-pre";
    if (isMain) {
      copt.diffChanges = collectDiffChangeRows(left, right);
      copt.diffChangeCursor = -1;
      clearDiffFocus();
      updateDiffNavUI();
    }
    if (origPre) {
      origPre.innerHTML = left.map((line, i) => diffLineHtml(line, i)).join("");
      if (document.activeElement !== origPre) {
        origPre.scrollTop = 0;
        origPre.scrollLeft = 0;
      }
    }
    if (optPre && document.activeElement !== optPre) {
      optPre.innerHTML = right.map((line, i) => diffLineHtml(line, i)).join("");
      optPre.scrollTop = 0;
      optPre.scrollLeft = 0;
    }
  }

  function renderYamlDiff(original, optimized) {
    copt.yamlOriginal = normYamlText(original);
    copt.yamlOptimized = normYamlText(optimized);
    renderYamlDiffTargets($("#copt-yaml-orig-pre"), $("#copt-yaml-opt-pre"), copt.yamlOriginal, copt.yamlOptimized);
    const fsOrig = $("#copt-fs-yaml-orig-pre");
    const fsOpt = $("#copt-fs-yaml-opt-pre");
    if (fsOrig && fsOpt && $("#copt-fullscreen-dialog")?.open) {
      renderYamlDiffTargets(fsOrig, fsOpt, copt.yamlOriginal, copt.yamlOptimized);
    }
    updateSaveUndoState();
  }

  let diffScrollLock = false;
  /** Scroll sincronizado entre dois painéis YAML. */
  function bindDiffScrollSyncPair(origPre, optPre, tag) {
    if (!origPre || !optPre || origPre.dataset.scrollSync) return;
    origPre.dataset.scrollSync = tag || "1";
    optPre.dataset.scrollSync = tag || "1";
    const sync = (src, dst) => {
      if (diffScrollLock) return;
      diffScrollLock = true;
      dst.scrollTop = src.scrollTop;
      dst.scrollLeft = src.scrollLeft;
      requestAnimationFrame(() => {
        diffScrollLock = false;
      });
    };
    origPre.addEventListener("scroll", () => sync(origPre, optPre));
    optPre.addEventListener("scroll", () => sync(optPre, origPre));
  }

  function bindDiffScrollSync() {
    bindDiffScrollSyncPair($("#copt-yaml-orig-pre"), $("#copt-yaml-opt-pre"), "main");
    bindDiffScrollSyncPair($("#copt-fs-yaml-orig-pre"), $("#copt-fs-yaml-opt-pre"), "fs");
  }

  function bindOptYamlEditorFor(optPre, onBlur) {
    if (!optPre || optPre.dataset.editBound) return;
    optPre.dataset.editBound = "1";
    optPre.addEventListener("focus", () => {
      if (optPre.querySelector(".copt-line")) {
        optPre.textContent = getYamlTextFrom(optPre);
      }
    });
    optPre.addEventListener("blur", () => {
      if (typeof onBlur === "function") onBlur(getYamlTextFrom(optPre));
    });
  }

  function bindOptYamlEditor() {
    bindOptYamlEditorFor($("#copt-yaml-opt-pre"), (text) => {
      copt.yamlOptimized = text;
      renderYamlDiff(copt.yamlOriginal, copt.yamlOptimized);
    });
    bindOptYamlEditorFor($("#copt-fs-yaml-opt-pre"), (text) => {
      copt.yamlOptimized = text;
      renderYamlDiffTargets($("#copt-yaml-orig-pre"), $("#copt-yaml-opt-pre"), copt.yamlOriginal, copt.yamlOptimized);
      renderYamlDiffTargets($("#copt-fs-yaml-orig-pre"), $("#copt-fs-yaml-opt-pre"), copt.yamlOriginal, copt.yamlOptimized);
      updateSaveUndoState();
    });
  }

  function formatBytes(n) {
    if (n == null || n <= 0) return "—";
    const u = ["B", "KB", "MB", "GB", "TB"];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < u.length - 1) {
      v /= 1024;
      i++;
    }
    return `${v.toFixed(i ? 1 : 0)} ${u[i]}`;
  }

  const copt = {
    files: [],
    stacks: [],
    swarmActive: false,
    swarmNodes: 0,
    selectedPath: "",
    selectedKind: "",
    loading: false,
    analyzing: false,
    lastResult: null,
    mode: "auto",
    roots: [],
    fileFilter: "",
    folderBrowsePath: "/",
    yamlOriginal: "",
    yamlOptimized: "",
    externalSessionId: "",
    diffChanges: [],
    diffChangeCursor: -1,
  };

  let externalPoll = null;

  function stopExternalPoll() {
    if (externalPoll) clearInterval(externalPoll);
    externalPoll = null;
  }

  function updateSyncExternalBtns(showPulse) {
    const has = !!copt.externalSessionId;
    ["#copt-sync-external", "#copt-fs-sync-external"].forEach((sel) => {
      const btn = $(sel);
      if (!btn) return;
      btn.classList.toggle("hidden", !has);
      btn.classList.toggle("pulse-hint", !!showPulse && has);
    });
  }

  function setDiffToolsEnabled(on) {
    const fs = $("#copt-fullscreen");
    const npp = $("#copt-notepad");
    const fsNpp = $("#copt-fs-notepad");
    if (fs) fs.disabled = !on;
    if (npp) npp.disabled = !on;
    if (fsNpp) fsNpp.disabled = !on;
    if (!on) {
      copt.diffChanges = [];
      copt.diffChangeCursor = -1;
      clearDiffFocus();
      updateDiffNavUI();
    }
  }

  function renderCriticalBanner(findings) {
    const el = $("#copt-alert-banner");
    if (!el) return;
    const alerts = (findings || []).filter(
      (f) => f.severity === "warn" && (f.category === "java" || f.category === "memory")
    );
    if (!alerts.length) {
      el.classList.add("hidden");
      el.innerHTML = "";
      return;
    }
    el.classList.remove("hidden");
    const items = alerts
      .map((f) => `<li><strong>${escapeHtml(f.service)}</strong> — ${escapeHtml(f.message)}</li>`)
      .join("");
    el.innerHTML = `<p class="copt-alert-title">${escapeHtml(tr("copt.alert.title"))}</p><ul class="copt-alert-list">${items}</ul>`;
  }

  function resolveValidateMode() {
    const m = copt.lastResult?.mode || copt.mode;
    if (m === "swarm" || m === "compose") return m;
    return copt.lastResult?.detectedKind === "swarm" ? "swarm" : "compose";
  }

  function flushOptEditor() {
    const mainOpt = $("#copt-yaml-opt-pre");
    const fsOpt = $("#copt-fs-yaml-opt-pre");
    if (mainOpt && document.activeElement === mainOpt) mainOpt.blur();
    if (fsOpt && document.activeElement === fsOpt) fsOpt.blur();
    // Painel em modo diff (.copt-line): manter copt.yamlOptimized da API, não innerText do DOM.
    if (mainOpt && !mainOpt.querySelector(".copt-line")) {
      copt.yamlOptimized = getYamlTextFrom(mainOpt);
    } else if (fsOpt && !fsOpt.querySelector(".copt-line")) {
      copt.yamlOptimized = getYamlTextFrom(fsOpt);
    }
  }

  function openCoptFullscreen() {
    if (!copt.yamlOriginal && !copt.yamlOptimized) return;
    flushOptEditor();
    const dlg = $("#copt-fullscreen-dialog");
    const title = $("#copt-fullscreen-title");
    if (!dlg) return;
    if (title) title.textContent = copt.selectedPath || tr("copt.fullscreen");
    renderYamlDiffTargets($("#copt-fs-yaml-orig-pre"), $("#copt-fs-yaml-opt-pre"), copt.yamlOriginal, copt.yamlOptimized);
    updateSaveUndoState();
    updateSyncExternalBtns(false);
    dlg.showModal();
    bindDiffScrollSyncPair($("#copt-fs-yaml-orig-pre"), $("#copt-fs-yaml-opt-pre"), "fs");
  }

  function closeCoptFullscreen() {
    flushOptEditor();
    renderYamlDiff(copt.yamlOriginal, copt.yamlOptimized);
    $("#copt-fullscreen-dialog")?.close();
  }

  async function openCoptNotepad() {
    if (!copt.selectedPath || !sshOk()) return;
    try {
      const res = await api("/api/remote/open-external", {
        method: "POST",
        body: JSON.stringify({ path: copt.selectedPath, editor: "notepad++" }),
      });
      copt.externalSessionId = res.sessionId || "";
      updateSyncExternalBtns(false);
      startExternalPoll();
      CWUI?.toast?.(res.message || tr("copt.notepad.ok"), "success", 6000);
    } catch (e) {
      CWUI?.toast?.(e.message, "error");
    }
  }

  function startExternalPoll() {
    stopExternalPoll();
    if (!copt.externalSessionId) return;
    externalPoll = setInterval(async () => {
      if (!copt.externalSessionId) return;
      try {
        const st = await api(`/api/remote/external-status?sessionId=${encodeURIComponent(copt.externalSessionId)}`);
        if (st.modified) updateSyncExternalBtns(true);
      } catch {
        /* ignore */
      }
    }, 4000);
  }

  async function syncCoptExternal() {
    const sid = copt.externalSessionId;
    if (!sid || !canWrite()) {
      if (!canWrite()) CWUI?.toast?.(tr("common.noPerm"), "error");
      return;
    }
    try {
      const res = await api("/api/remote/sync-external", {
        method: "POST",
        body: JSON.stringify({ sessionId: sid }),
      });
      copt.externalSessionId = "";
      stopExternalPoll();
      updateSyncExternalBtns(false);
      CWUI?.toast?.(res.message || tr("copt.syncExternal.ok"), "success");
      await analyze();
    } catch (e) {
      CWUI?.toast?.(e.message, "error");
    }
  }

  function loadRoots() {
    try {
      const raw = localStorage.getItem(LS_ROOTS);
      if (raw) {
        const arr = JSON.parse(raw);
        if (Array.isArray(arr) && arr.length) return [...new Set(arr)];
      }
    } catch { /* ignore */ }
    const extra = typeof state !== "undefined" && state.remotePath && state.remotePath !== "/" ? [state.remotePath] : [];
    return [...new Set([...PRESET_ROOTS, ...extra])];
  }

  function saveRoots() {
    try {
      localStorage.setItem(LS_ROOTS, JSON.stringify(copt.roots));
    } catch { /* ignore */ }
  }

  function rootsQuery() {
    return copt.roots.join(" ");
  }

  function canWrite() {
    return typeof window.canAction === "function" ? window.canAction("files.write") : true;
  }

  function setUpdated(msg) {
    const el = $("#copt-updated");
    if (el) el.textContent = msg || "—";
  }

  function kindLabel(kind) {
    if (kind === "swarm") return tr("copt.kind.swarm");
    return tr("copt.kind.compose");
  }

  function kindIcon(kind) {
    return kind === "swarm" ? "🐝" : "📄";
  }

  function renderFolderChips() {
    const box = $("#copt-folder-chips");
    if (!box) return;
    box.innerHTML = copt.roots
      .map(
        (r) => `
      <span class="copt-chip${copt.folderBrowsePath === r ? " is-on" : ""}">
        <button type="button" class="copt-chip-main" data-root="${escapeHtml(r)}" title="${escapeHtml(r)}">${escapeHtml(r)}</button>
        <button type="button" class="copt-chip-x" data-remove="${escapeHtml(r)}" aria-label="${escapeHtml(tr("copt.removeFolder"))}">×</button>
      </span>`
      )
      .join("");
    box.querySelectorAll("[data-root]").forEach((btn) => {
      btn.addEventListener("click", () => {
        copt.folderBrowsePath = btn.getAttribute("data-root");
        void discover();
      });
    });
    box.querySelectorAll("[data-remove]").forEach((btn) => {
      btn.addEventListener("click", (e) => {
        e.stopPropagation();
        const p = btn.getAttribute("data-remove");
        copt.roots = copt.roots.filter((x) => x !== p);
        saveRoots();
        renderFolderChips();
      });
    });
  }

  function addRoot(path) {
    const p = (path || "").replace(/\/+$/, "") || "/";
    if (!copt.roots.includes(p)) {
      copt.roots.push(p);
      saveRoots();
      renderFolderChips();
    }
  }

  function filteredFiles() {
    const q = copt.fileFilter.trim().toLowerCase();
    if (!q) return copt.files;
    return copt.files.filter((f) => `${f.name} ${f.path} ${f.dir}`.toLowerCase().includes(q));
  }

  function renderStacks() {
    const box = $("#copt-stacks");
    if (!box) return;
    if (!copt.swarmActive || !copt.stacks.length) {
      box.classList.add("hidden");
      box.innerHTML = "";
      return;
    }
    box.classList.remove("hidden");
    box.innerHTML = `
      <p class="copt-stacks-label">${escapeHtml(tr("copt.stacks"))}</p>
      <div class="copt-stack-chips">${copt.stacks
        .map((s) => `<span class="copt-stack-chip">🐝 ${escapeHtml(s.name)}</span>`)
        .join("")}</div>`;
  }

  function renderSwarmBadge() {
    const el = $("#copt-swarm-badge");
    if (!el) return;
    if (!copt.swarmActive) {
      el.classList.add("hidden");
      return;
    }
    el.classList.remove("hidden");
    el.textContent = tr("copt.swarmOn", { n: copt.swarmNodes });
    el.className = "copt-swarm-badge copt-swarm-badge--on";
  }

  function renderFileList() {
    const list = $("#copt-file-list");
    if (!list) return;
    if (copt.loading) {
      list.innerHTML = `<div class="copt-empty">${escapeHtml(tr("common.loading"))}</div>`;
      return;
    }
    const rows = filteredFiles();
    if (!rows.length) {
      list.innerHTML = `<div class="copt-empty">${escapeHtml(tr(copt.files.length ? "copt.filter.empty" : "copt.files.empty"))}</div>`;
      return;
    }
    list.innerHTML = rows
      .map((f) => {
        const active = f.path === copt.selectedPath ? " is-active" : "";
        const kind = f.kind || "compose";
        return `
        <button type="button" class="copt-file-card${active}" data-path="${escapeHtml(f.path)}" data-kind="${escapeHtml(kind)}" role="listitem">
          <span class="copt-file-icon">${kindIcon(kind)}</span>
          <span class="copt-file-body">
            <span class="copt-file-name">${escapeHtml(f.name || f.path)}</span>
            <span class="copt-file-dir muted">${escapeHtml(f.dir || "")}</span>
          </span>
          <span class="copt-kind-badge copt-kind-badge--${kind}">${escapeHtml(kindLabel(kind))}</span>
        </button>`;
      })
      .join("");
    list.querySelectorAll("[data-path]").forEach((btn) => {
      btn.addEventListener("click", () => selectFile(btn.getAttribute("data-path"), btn.getAttribute("data-kind"), true));
      btn.addEventListener("dblclick", (e) => {
        e.preventDefault();
        void analyze();
      });
    });
  }

  function renderSelectedBar() {
    const bar = $("#copt-selected");
    if (!bar) return;
    if (!copt.selectedPath) {
      bar.className = "copt-selected-inline muted";
      bar.textContent = tr("copt.pickFile");
      return;
    }
    bar.className = "copt-selected-inline";
    bar.innerHTML = `${kindIcon(copt.selectedKind)} <strong>${escapeHtml(copt.selectedPath)}</strong>
      <span class="copt-kind-badge copt-kind-badge--${copt.selectedKind || "compose"}">${escapeHtml(kindLabel(copt.selectedKind))}</span>`;
  }

  let analyzeTimer = null;

  function setFindingsCollapsed(collapsed) {
    const wrap = $("#copt-findings-wrap");
    const btn = $("#copt-findings-toggle");
    if (!wrap || !btn) return;
    wrap.classList.toggle("is-collapsed", collapsed);
    btn.setAttribute("aria-expanded", collapsed ? "false" : "true");
    btn.textContent = collapsed ? "▸" : "▾";
    btn.title = tr(collapsed ? "copt.findings.expand" : "copt.findings.collapse");
  }

  function updateFindingsCount(n) {
    const el = $("#copt-findings-count");
    if (!el) return;
    if (n > 0) {
      el.classList.remove("hidden");
      el.textContent = tr("copt.findings.count", { n });
    } else {
      el.classList.add("hidden");
      el.textContent = "";
    }
  }

  function setFindingsMessage(msg, isError) {
    if (isError || !copt.lastResult?.findings?.length) renderCriticalBanner([]);
    const box = $("#copt-findings");
    if (!box) return;
    box.className = isError ? "copt-findings cw-scroll copt-findings--err" : "copt-findings cw-scroll muted";
    box.textContent = msg;
    updateFindingsCount(0);
    if (isError) setFindingsCollapsed(false);
    const badge = $("#copt-mode-badge");
    if (badge && !copt.lastResult) badge.classList.add("hidden");
  }

  function setEditorsLoading(on) {
    const origPre = $("#copt-yaml-orig-pre");
    const optPre = $("#copt-yaml-opt-pre");
    if (on) {
      if (origPre) origPre.textContent = tr("copt.loadingYaml");
      if (optPre) optPre.textContent = "";
      setFindingsMessage(tr("copt.analyzing"), false);
    }
  }

  function selectFile(path, kind, autoAnalyze = true) {
    copt.selectedPath = path || "";
    copt.selectedKind = kind || "compose";
    copt.lastResult = null;
    renderCriticalBanner([]);
    copt.externalSessionId = "";
    stopExternalPoll();
    updateSyncExternalBtns(false);
    setDiffToolsEnabled(false);
    const analyzeBtn = $("#copt-analyze");
    if (analyzeBtn) analyzeBtn.disabled = !copt.selectedPath || copt.analyzing;
    renderFileList();
    renderSelectedBar();
    if (!copt.selectedPath) {
      setFindingsMessage(tr("copt.findings.empty"), false);
      return;
    }
    if (autoAnalyze) {
      clearTimeout(analyzeTimer);
      analyzeTimer = setTimeout(() => void analyze(), 120);
    } else {
      setFindingsMessage(tr("copt.clickAnalyze"), false);
    }
  }

  function renderHost(host) {
    const box = $("#copt-host");
    if (!box) return;
    if (!host) {
      box.classList.add("hidden");
      box.innerHTML = "";
      return;
    }
    box.classList.remove("hidden");
    box.innerHTML = `<span class="copt-host-inline-label muted">${escapeHtml(tr("copt.host"))}:</span>
      <span>${escapeHtml(host.hostname || "—")}</span>
      <span class="muted">·</span>
      <span>${host.cpus || "?"} CPU</span>
      <span class="muted">·</span>
      <span>${formatBytes(host.memTotal)} (${Math.round(host.memPct || 0)}%)</span>
      <span class="muted">·</span>
      <span>${copt.lastResult?.serviceCount ?? "—"} ${escapeHtml(tr("copt.host.servicesShort"))}</span>`;
  }

  const sevClass = { info: "copt-finding--info", warn: "copt-finding--warn", suggest: "copt-finding--suggest" };

  function renderFindings(findings, result) {
    renderCriticalBanner(findings);
    const box = $("#copt-findings");
    const badge = $("#copt-mode-badge");
    if (badge && result) {
      const modeKey = result.mode === "swarm" ? "copt.mode.swarm" : "copt.mode.compose";
      badge.classList.remove("hidden");
      badge.textContent = `${tr(modeKey)} · ${kindLabel(result.detectedKind || "compose")}`;
      badge.className = `copt-kind-badge copt-kind-badge--${result.detectedKind || "compose"}`;
    }
    if (!box) return;
    if (!findings?.length) {
      renderCriticalBanner([]);
      box.className = "copt-findings muted";
      box.textContent = tr("copt.findings.none");
      updateFindingsCount(0);
      setFindingsCollapsed(true);
      return;
    }
    updateFindingsCount(findings.length);
    setFindingsCollapsed(true);
    box.className = "copt-findings cw-scroll";
    box.innerHTML = findings
      .map((f) => {
        const cls = sevClass[f.severity] || "";
        const cur = f.current ? `<span class="copt-finding-cur">${escapeHtml(f.current)}</span>` : "";
        const sug = f.suggested ? ` → <span class="copt-finding-sug">${escapeHtml(f.suggested)}</span>` : "";
        const doc = f.docRef
          ? ` <a href="${escapeHtml(f.docRef)}" target="_blank" rel="noopener" class="copt-doc">${escapeHtml(tr("copt.doc"))}</a>`
          : "";
        return `<article class="copt-finding ${cls}">
          <header><strong>${escapeHtml(f.service)}</strong> · ${escapeHtml(f.category)}${doc}</header>
          <p>${escapeHtml(f.message)}${cur}${sug}</p>
        </article>`;
      })
      .join("");
  }

  async function discover(rootsOverride) {
    if (!sshOk()) return;
    copt.loading = true;
    renderFileList();
    setUpdated(tr("copt.discovering"));
    const roots = (rootsOverride || rootsQuery()).trim();
    try {
      const data = await api(`/api/compose-opt/discover?roots=${encodeURIComponent(roots)}&max=60`);
      copt.files = data.files || [];
      copt.stacks = data.stacks || [];
      copt.swarmActive = !!data.swarmActive;
      copt.swarmNodes = data.swarmNodes || 0;
      renderStacks();
      renderSwarmBadge();
      if (copt.swarmActive && copt.mode === "auto") {
        syncModeTab("auto");
      }
      setUpdated(tr("copt.found", { n: copt.files.length }));
      if (copt.files.length && !copt.selectedPath) {
        const first = copt.files[0];
        selectFile(first.path, first.kind, true);
      } else if (copt.selectedPath) {
        selectFile(copt.selectedPath, copt.selectedKind, true);
      }
    } catch (e) {
      CWUI?.toast?.(e.message, "error");
      setUpdated(e.message);
      copt.files = [];
    } finally {
      copt.loading = false;
      renderFileList();
    }
  }

  async function analyze() {
    if (!copt.selectedPath || copt.analyzing) return;
    if (!sshOk()) return;
    copt.analyzing = true;
    const analyzeBtn = $("#copt-analyze");
    if (analyzeBtn) analyzeBtn.disabled = true;
    setUpdated(tr("copt.analyzing"));
    setEditorsLoading(true);
    try {
      const data = await api(
        `/api/compose-opt/analyze?path=${encodeURIComponent(copt.selectedPath)}&mode=${encodeURIComponent(copt.mode)}`
      );
      copt.lastResult = data;
      renderYamlDiff(data.original || "", data.optimized || "");
      renderHost(data.host);
      if ((data.findings || []).length) {
        renderFindings(data.findings, data);
      } else {
        setFindingsMessage(tr("copt.findings.none"), false);
        const badge = $("#copt-mode-badge");
        if (badge && data.mode) {
          badge.classList.remove("hidden");
          const modeKey = data.mode === "swarm" ? "copt.mode.swarm" : "copt.mode.compose";
          badge.textContent = `${tr(modeKey)} · ${kindLabel(data.detectedKind || "compose")}`;
        }
      }
      setDiffToolsEnabled(true);
      updateSaveUndoState();
      setUpdated(tr("copt.done", { n: (data.findings || []).length }));
    } catch (e) {
      const msg = e.message || tr("copt.analyze.fail");
      CWUI?.toast?.(msg, "error", 8000);
      setUpdated(msg);
      setFindingsMessage(msg, true);
      const origPre = $("#copt-yaml-orig-pre");
      const optPre = $("#copt-yaml-opt-pre");
      if (origPre) origPre.textContent = "";
      if (optPre) optPre.textContent = "";
      const saveBtn = $("#copt-save");
      if (saveBtn) saveBtn.disabled = true;
      setDiffToolsEnabled(false);
    } finally {
      copt.analyzing = false;
      if (analyzeBtn) analyzeBtn.disabled = !copt.selectedPath;
    }
  }

  async function saveOptimized() {
    if (!copt.selectedPath || !canWrite()) {
      CWUI?.toast?.(tr("common.noPerm"), "error");
      return;
    }
    flushOptEditor();
    const content = currentOptimizedText();
    if (!content?.trim()) return;
    const summary = summarizeYamlChanges(copt.yamlOriginal, content);
    if (summary.noChanges) {
      CWUI?.toast?.(tr("copt.save.noChanges"), "info");
      updateSaveUndoState();
      return;
    }
    const ok = await confirmCoptSave(copt.selectedPath, summary);
    if (!ok) return;
    const saveBtn = $("#copt-save");
    const fsSave = $("#copt-fs-save");
    if (saveBtn) saveBtn.disabled = true;
    if (fsSave) fsSave.disabled = true;
    try {
      setUpdated(tr("copt.validating"));
      await api("/api/compose-opt/validate", {
        method: "POST",
        body: JSON.stringify({ content, mode: resolveValidateMode() }),
      });
      await api("/api/remote/file", {
        method: "PUT",
        body: JSON.stringify({ path: copt.selectedPath, content }),
      });
      CWUI?.toast?.(tr("copt.save.ok"), "success");
      copt.yamlOriginal = content;
      copt.yamlOptimized = content;
      if (copt.lastResult) {
        copt.lastResult.original = content;
        copt.lastResult.optimized = content;
      }
      renderYamlDiff(content, content);
    } catch (e) {
      CWUI?.toast?.(e.message, "error");
      updateSaveUndoState();
    }
  }

  function copyYaml(side) {
    const text = side === "orig" ? copt.yamlOriginal : getOptYamlText();
    if (!text) return;
    navigator.clipboard.writeText(text).then(
      () => CWUI?.toast?.(tr("common.copied"), "success", 2000),
      () => CWUI?.toast?.(tr("common.copyFail"), "error")
    );
  }

  function syncModeTab(mode) {
    copt.mode = mode;
    document.querySelectorAll(".copt-mode-tab").forEach((tab) => {
      tab.classList.toggle("is-active", tab.getAttribute("data-mode") === mode);
    });
  }

  /* —— Diálogo de pastas —— */
  async function loadFolderDirs(path) {
    const box = $("#copt-folder-dirs");
    const bc = $("#copt-folder-bc");
    if (!box) return;
    box.innerHTML = `<p class="muted">${escapeHtml(tr("common.loading"))}</p>`;
    try {
      const data = await api(`/api/remote/list?path=${encodeURIComponent(path || "/")}`);
      copt.folderBrowsePath = data.path || path || "/";
      if (bc && window.CWUI?.renderBreadcrumbs) {
        CWUI.renderBreadcrumbs(bc, CWUI.pathToBreadcrumbs(copt.folderBrowsePath, false), (p) => loadFolderDirs(p));
      } else if (bc) {
        bc.textContent = copt.folderBrowsePath;
      }
      const dirs = (data.entries || []).filter((e) => e.isDir).sort((a, b) => (a.name || "").localeCompare(b.name || ""));
      if (!dirs.length) {
        box.innerHTML = `<p class="muted">${escapeHtml(tr("copt.folderDlg.noDirs"))}</p>`;
        return;
      }
      box.innerHTML = dirs
        .map(
          (d) => `
        <button type="button" class="copt-dir-item" data-path="${escapeHtml(d.path)}">
          <span class="copt-dir-icon">📁</span>
          <span>${escapeHtml(d.name)}</span>
        </button>`
        )
        .join("");
      box.querySelectorAll("[data-path]").forEach((btn) => {
        btn.addEventListener("click", () => loadFolderDirs(btn.getAttribute("data-path")));
      });
    } catch (e) {
      box.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
    }
  }

  function openFolderDialog() {
    if (!sshOk()) return;
    const dlg = $("#copt-folder-dialog");
    if (!dlg) return;
    loadFolderDirs(copt.folderBrowsePath || "/");
    dlg.showModal();
  }

  function closeFolderDialog() {
    $("#copt-folder-dialog")?.close();
  }

  /** Usa o state global de app.js (sessão SSH), não confundir com copt. */
  function sshOk() {
    if (typeof state === "undefined" || !state.ssh?.connected) {
      CWUI?.toast?.(tr("copt.needSsh"), "error");
      return false;
    }
    return true;
  }

  function setFoldersCollapsed(collapsed) {
    const toolbar = document.querySelector(".composeopt-module .copt-toolbar");
    const btn = $("#copt-folders-toggle");
    if (!toolbar || !btn) return;
    toolbar.classList.toggle("is-folders-collapsed", collapsed);
    btn.setAttribute("aria-expanded", collapsed ? "false" : "true");
    btn.textContent = collapsed ? "▸" : "▾";
    btn.title = tr(collapsed ? "copt.folders.expand" : "copt.folders.collapse");
  }

  function bindCollapseUI() {
    const findingsHead = document.querySelector(".copt-findings-head");
    const findingsToggle = () => {
      const wrap = $("#copt-findings-wrap");
      if (!wrap) return;
      setFindingsCollapsed(!wrap.classList.contains("is-collapsed"));
    };
    $("#copt-findings-toggle")?.addEventListener("click", (e) => {
      e.stopPropagation();
      findingsToggle();
    });
    findingsHead?.addEventListener("click", (e) => {
      if (e.target.closest("#copt-findings-toggle, a, button")) return;
      findingsToggle();
    });
    $("#copt-folders-toggle")?.addEventListener("click", () => {
      const toolbar = document.querySelector(".composeopt-module .copt-toolbar");
      setFoldersCollapsed(!toolbar?.classList.contains("is-folders-collapsed"));
    });
    setFindingsCollapsed(true);
    setFoldersCollapsed(true);
  }

  function bindUI() {
    copt.roots = loadRoots();
    renderFolderChips();
    renderSelectedBar();
    bindCollapseUI();

    $("#copt-discover")?.addEventListener("click", () => discover());
    $("#copt-analyze")?.addEventListener("click", (e) => {
      e.preventDefault();
      void analyze();
    });
    $("#copt-save")?.addEventListener("click", saveOptimized);
    $("#copt-copy-orig")?.addEventListener("click", () => copyYaml("orig"));
    $("#copt-copy-opt")?.addEventListener("click", () => copyYaml("opt"));
    $("#copt-fs-copy-orig")?.addEventListener("click", () => copyYaml("orig"));
    $("#copt-fs-copy-opt")?.addEventListener("click", () => copyYaml("opt"));
    $("#copt-diff-prev")?.addEventListener("click", () => navigateDiff(-1));
    $("#copt-diff-next")?.addEventListener("click", () => navigateDiff(1));
    $("#copt-fullscreen")?.addEventListener("click", openCoptFullscreen);
    $("#copt-fullscreen-close")?.addEventListener("click", closeCoptFullscreen);
    $("#copt-fullscreen-dialog")?.addEventListener("close", () => {
      flushOptEditor();
      renderYamlDiff(copt.yamlOriginal, copt.yamlOptimized);
    });
    $("#copt-fullscreen-dialog")?.addEventListener("click", (e) => {
      if (e.target === e.currentTarget) closeCoptFullscreen();
    });
    $("#copt-notepad")?.addEventListener("click", () => void openCoptNotepad());
    $("#copt-fs-notepad")?.addEventListener("click", () => void openCoptNotepad());
    $("#copt-sync-external")?.addEventListener("click", () => {
      $("#copt-sync-external")?.classList.remove("pulse-hint");
      void syncCoptExternal();
    });
    $("#copt-fs-sync-external")?.addEventListener("click", () => {
      $("#copt-fs-sync-external")?.classList.remove("pulse-hint");
      void syncCoptExternal();
    });
    $("#copt-fs-save")?.addEventListener("click", saveOptimized);
    $("#copt-undo")?.addEventListener("click", revertCoptChanges);
    $("#copt-fs-undo")?.addEventListener("click", revertCoptChanges);
    bindDiffScrollSync();
    bindOptYamlEditor();
    $("#copt-pick-folder")?.addEventListener("click", openFolderDialog);
    $("#copt-file-filter")?.addEventListener("input", (e) => {
      copt.fileFilter = e.target.value;
      renderFileList();
    });

    document.querySelectorAll(".copt-mode-tab").forEach((tab) => {
      tab.addEventListener("click", () => {
        syncModeTab(tab.getAttribute("data-mode"));
        if (copt.selectedPath) void analyze();
      });
    });

    $("#copt-folder-cancel")?.addEventListener("click", closeFolderDialog);
    $("#copt-folder-add")?.addEventListener("click", () => {
      addRoot(copt.folderBrowsePath);
      closeFolderDialog();
      CWUI?.toast?.(tr("copt.folderDlg.added"), "success", 2500);
    });
    $("#copt-folder-scan")?.addEventListener("click", () => {
      const p = copt.folderBrowsePath;
      closeFolderDialog();
      void discover(p);
    });

    document.addEventListener("cw-lang-change", () => {
      renderFolderChips();
      renderFileList();
      renderSelectedBar();
      renderStacks();
      renderSwarmBadge();
      const wrap = $("#copt-findings-wrap");
      if (wrap) setFindingsCollapsed(wrap.classList.contains("is-collapsed"));
      const toolbar = document.querySelector(".composeopt-module .copt-toolbar");
      if (toolbar) setFoldersCollapsed(toolbar.classList.contains("is-folders-collapsed"));
      if (copt.lastResult) {
        renderHost(copt.lastResult.host);
        renderFindings(copt.lastResult.findings, copt.lastResult);
        updateFindingsCount((copt.lastResult.findings || []).length);
      }
    });
  }

  window.composeOptOnScreenEnter = function () {
    copt.roots = loadRoots();
    renderFolderChips();
    if ($("#copt-auto-scan")?.checked && sshOk() && !copt.files.length) {
      void discover();
    }
  };

  window.initComposeOpt = function () {
    bindUI();
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", window.initComposeOpt);
  } else {
    window.initComposeOpt();
  }
})();
