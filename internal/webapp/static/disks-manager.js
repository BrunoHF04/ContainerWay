/**
 * Módulo «Discos e armazenamento» — paridade com o desktop (Fyne).
 */
(function () {
  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

  function t(key, vars) {
    if (window.CWI18n?.t) return window.CWI18n.t(key, vars);
    return key;
  }

  function applyDisksStatic() {
    if (window.CWI18n?.applyLabels) window.CWI18n.applyLabels();
    void fetchSudoStatus();
  }

  const state = {
    tab: "assistant",
    rows: [],
    technical: "",
    lvs: "",
    lvRecords: [],
    vgStats: [],
    lvOptions: [],
    hostSel: null,
    loading: false,
    autoTimer: null,
    probeDebounce: null,
    files: {
      path: "/",
      parent: "",
      totalBytes: 0,
      entries: [],
      roots: [],
      mountRoots: [],
      loading: false,
      filter: "",
      progress: null,
      loadAbort: null,
    },
  };

  function escapeHtml(s) {
    return String(s ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function emptyDash(s) {
    const t = String(s ?? "").trim();
    return t || "—";
  }

  function indent(depth) {
    return "\u00a0\u00a0".repeat(Math.max(0, depth || 0));
  }

  function setTab(tab) {
    state.tab = tab;
    $$(".disks-module-tab").forEach((btn) => {
      const on = btn.dataset.disksTab === tab;
      btn.classList.toggle("active", on);
      btn.setAttribute("aria-selected", on ? "true" : "false");
    });
    $$(".disks-section").forEach((sec) => {
      const on = sec.id === `disks-section-${tab}`;
      sec.classList.toggle("active", on);
      if (on) window.CWMotion?.pulseEnter?.(sec, "section-enter");
    });
    if (tab === "files" && !state.files.entries.length && !state.files.loading) {
      void scanFilesPath(state.files.path || "/");
    }
  }

  function formatBytes(n) {
    const v = Number(n) || 0;
    if (v < 1024) return `${v} B`;
    const u = ["KiB", "MiB", "GiB", "TiB"];
    let x = v / 1024;
    let i = 0;
    while (x >= 1024 && i < u.length - 1) {
      x /= 1024;
      i++;
    }
    return `${x < 10 ? x.toFixed(1) : Math.round(x)} ${u[i]}`;
  }

  function renderFilesRoots(roots) {
    const sel = $("#disks-files-root");
    if (!sel) return;
    const list = roots?.length ? roots : [{ label: "Raiz /", path: "/" }];
    state.files.roots = list;
    sel.innerHTML = list
      .map((r) => `<option value="${escapeHtml(r.path)}">${escapeHtml(r.label)}</option>`)
      .join("");
    sel.value = state.files.path || "/";
    CWUI?.refreshSelect?.(sel);
  }

  function renderFilesBreadcrumb(dirPath) {
    const nav = $("#disks-files-breadcrumb");
    if (!nav) return;
    const parts = dirPath === "/" ? [] : dirPath.split("/").filter(Boolean);
    let acc = "";
    const crumbs = [
      `<button type="button" class="disks-crumb" data-path="/">/</button>`,
    ];
    for (const part of parts) {
      acc += "/" + part;
      const p = acc || "/";
      crumbs.push(
        `<span class="disks-crumb-sep">/</span><button type="button" class="disks-crumb" data-path="${escapeHtml(p)}">${escapeHtml(part)}</button>`
      );
    }
    nav.innerHTML = crumbs.join("");
  }

  function renderFilesList() {
    const list = $("#disks-files-list");
    const summary = $("#disks-files-summary");
    const upBtn = $("#disks-files-up");
    if (!list) return;
    const f = state.files;
    if (upBtn) upBtn.disabled = !f.parent || f.path === "/";
    if (summary) {
      const extra = f.truncated ? t("disks.files.truncated") : "";
      if (f.loading) {
        const prog = f.progress;
        summary.textContent =
          prog?.total > 0 && prog?.name
            ? t("disks.files.progress", {
                current: prog.current,
                total: prog.total,
                name: prog.name,
              })
            : prog?.phase === "total"
              ? t("disks.files.progressTotal")
              : t("disks.scanning");
      } else {
        summary.textContent = t("disks.files.summary", {
          path: f.path,
          size: formatBytes(f.totalBytes),
          n: f.entries.length,
          extra,
        });
      }
    }
    const q = (f.filter || "").trim().toLowerCase();
    const entries = q
      ? f.entries.filter((e) => e.name.toLowerCase().includes(q) || e.path.toLowerCase().includes(q))
      : f.entries;
    if (f.loading) {
      const prog = f.progress || {};
      const pct = Math.min(100, Math.max(0, prog.pct || 0));
      const indeterminate = !prog.total && prog.phase !== "total";
      const label =
        prog.total > 0 && prog.name
          ? t("disks.files.progress", {
              current: prog.current,
              total: prog.total,
              name: prog.name,
            })
          : prog.phase === "total"
            ? t("disks.files.progressTotal")
            : t("disks.calcSizes");
      list.innerHTML = `<div class="disks-files-loading" role="status">
        <p class="disks-files-loading-label">${escapeHtml(label)}</p>
        <div class="disks-files-progress-track${indeterminate ? " is-indeterminate" : ""}" role="progressbar" aria-valuenow="${pct}" aria-valuemin="0" aria-valuemax="100" aria-label="${escapeHtml(label)}">
          <div class="disks-files-progress-fill" style="width:${indeterminate ? 100 : pct}%"></div>
        </div>
      </div>`;
      return;
    }
    if (!entries.length) {
      list.innerHTML =
        `<div class="disks-files-row disks-files-empty">${escapeHtml(t("disks.files.empty"))}</div>`;
      return;
    }
    const maxBar = entries[0]?.sizeBytes || 1;
    list.innerHTML = entries
      .map((e) => {
        const pct = Math.min(100, Math.max(0, e.pct || 0));
        const barW = maxBar > 0 ? Math.round((e.sizeBytes / maxBar) * 100) : 0;
        const icon = e.isDir ? "📁" : "📄";
        const nameBtn = e.isDir
          ? `<button type="button" class="disks-files-name disks-files-enter" data-path="${escapeHtml(e.path)}">${icon} ${escapeHtml(e.name)}</button>`
          : `<span class="disks-files-name">${icon} ${escapeHtml(e.name)}</span>`;
        return `<div class="disks-files-row" role="listitem" data-path="${escapeHtml(e.path)}">
          <span class="disks-files-col-name">${nameBtn}</span>
          <span class="disks-files-col-bar"><span class="disks-files-bar" style="width:${barW}%"></span></span>
          <span class="disks-files-col-size">${escapeHtml(formatBytes(e.sizeBytes))}</span>
          <span class="disks-files-col-pct">${pct < 0.1 && e.sizeBytes > 0 ? "&lt;0,1" : pct.toFixed(1)}%</span>
          <span class="disks-files-col-actions">
            <button type="button" class="btn btn-ghost btn-sm disks-files-open" data-path="${escapeHtml(e.path)}" title="SFTP">${escapeHtml(t("disks.files.explorer"))}</button>
            <button type="button" class="btn btn-ghost btn-sm disks-files-del" data-path="${escapeHtml(e.path)}" data-dir="${e.isDir ? "1" : "0"}" data-name="${escapeHtml(e.name)}">${escapeHtml(t("disks.files.delete"))}</button>
          </span>
        </div>`;
      })
      .join("");
  }

  function applyFilesResult(data) {
    state.files.path = data.path || state.files.path;
    state.files.parent = data.parent || "";
    state.files.totalBytes = data.totalBytes || 0;
    state.files.entries = data.entries || [];
    state.files.truncated = !!data.truncated;
    if (data.roots?.length) renderFilesRoots(data.roots);
    renderFilesBreadcrumb(state.files.path);
  }

  async function scanFilesPath(dirPath) {
    state.files.loadAbort?.abort();
    const ac = new AbortController();
    state.files.loadAbort = ac;
    state.files.loading = true;
    state.files.progress = { phase: "connect", current: 0, total: 0, name: "", pct: 0 };
    state.files.path = dirPath || "/";
    renderFilesBreadcrumb(state.files.path);
    renderFilesList();

    let gotDone = false;
    try {
      const url = `/api/disks/usage?path=${encodeURIComponent(state.files.path)}&stream=1`;
      const res = await fetch(url, { credentials: "same-origin", signal: ac.signal });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || res.statusText || "Erro");
      }
      const reader = res.body?.getReader();
      if (!reader) throw new Error("stream indisponível");

      const dec = new TextDecoder();
      let buf = "";
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        let nl;
        while ((nl = buf.indexOf("\n")) >= 0) {
          const line = buf.slice(0, nl).trim();
          buf = buf.slice(nl + 1);
          if (!line) continue;
          const ev = JSON.parse(line);
          if (ev.type === "phase") {
            state.files.progress = { phase: ev.phase || "total", current: 0, total: 0, name: "", pct: 8 };
            renderFilesList();
          } else if (ev.type === "start") {
            state.files.progress = {
              phase: "scan",
              current: 0,
              total: ev.total || 0,
              name: "",
              pct: 12,
            };
            renderFilesList();
          } else if (ev.type === "progress") {
            state.files.progress = {
              phase: "scan",
              current: ev.current || 0,
              total: ev.total || 0,
              name: ev.name || "",
              pct: Math.max(12, Math.min(98, ev.pct || 0)),
            };
            renderFilesList();
          } else if (ev.type === "done") {
            applyFilesResult(ev);
            state.files.progress = { phase: "done", current: 1, total: 1, name: "", pct: 100 };
            gotDone = true;
            renderFilesList();
          } else if (ev.type === "error") {
            throw new Error(ev.error || "Falha ao analisar pasta");
          }
        }
      }
    } catch (e) {
      if (e.name === "AbortError") return;
      try {
        const data = await api(`/api/disks/usage?path=${encodeURIComponent(state.files.path)}`);
        applyFilesResult(data);
        gotDone = true;
      } catch (e2) {
        state.files.entries = [];
        CWUI.toast(e2.message || e.message || "Falha ao analisar pasta", "error");
      }
    } finally {
      if (state.files.loadAbort === ac) state.files.loadAbort = null;
      state.files.loading = false;
      state.files.progress = null;
      if (gotDone) renderFilesList();
      else if (!ac.signal.aborted) renderFilesList();
    }
  }

  async function deleteFilesEntry(entryPath, isDir, name) {
    const warn =
      entryPath === "/" || entryPath === "/bin" || entryPath === "/etc" || entryPath === "/usr"
        ? t("disks.confirm.sysWarn")
        : "";
    const ok = await CWConfirm(
      t("disks.confirm.delete", {
        name,
        extra: isDir ? t("disks.confirm.deleteDir") : "",
        warn,
      }),
      { danger: true, ok: t("disks.files.delete") }
    );
    if (!ok) return;
    try {
      await api("/api/remote/delete", {
        method: "POST",
        body: JSON.stringify({ path: entryPath, recursive: !!isDir }),
      });
      CWUI.toast(t("disks.toast.deleted"), "success");
      await scanFilesPath(state.files.path);
    } catch (e) {
      CWUI.toast(e.message || t("disks.sudo.inactive"), "error");
    }
  }

  function bindFilesEvents() {
    $("#disks-files-scan")?.addEventListener("click", () => void scanFilesPath(state.files.path || "/"));
    $("#disks-files-up")?.addEventListener("click", () => {
      if (state.files.parent) void scanFilesPath(state.files.parent);
    });
    $("#disks-files-root")?.addEventListener("change", (ev) => {
      const p = ev.target?.value;
      if (p) void scanFilesPath(p);
    });
    $("#disks-files-filter")?.addEventListener("input", (ev) => {
      state.files.filter = ev.target?.value || "";
      renderFilesList();
    });
    $("#disks-files-breadcrumb")?.addEventListener("click", (ev) => {
      const btn = ev.target.closest?.(".disks-crumb");
      if (!btn?.dataset.path) return;
      void scanFilesPath(btn.dataset.path);
    });
    $("#disks-files-list")?.addEventListener("click", (ev) => {
      const enter = ev.target.closest?.(".disks-files-enter");
      if (enter?.dataset.path) {
        void scanFilesPath(enter.dataset.path);
        return;
      }
      const open = ev.target.closest?.(".disks-files-open");
      if (open?.dataset.path) {
        if (typeof window.openRemoteExplorer === "function") {
          window.openRemoteExplorer(open.dataset.path);
        } else {
          CWUI.toast(open.dataset.path, "info");
        }
        return;
      }
      const del = ev.target.closest?.(".disks-files-del");
      if (del?.dataset.path) {
        void deleteFilesEntry(del.dataset.path, del.dataset.dir === "1", del.dataset.name || del.dataset.path);
      }
    });
  }

  function updateSudoBanner(sudo) {
    const status = $("#disks-sudo-status");
    const onBtn = $("#disks-sudo-on");
    const offBtn = $("#disks-sudo-off");
    if (!status) return;
    if (sudo?.enabled && sudo.user) {
      status.textContent = t("disks.sudo.chipOn", { user: sudo.user });
      status.title = t("disks.sudo.active", { user: sudo.user });
      status.classList.add("is-on");
      onBtn?.setAttribute("disabled", "disabled");
      offBtn?.classList.remove("hidden");
    } else {
      status.textContent = t("disks.sudo.chipOff");
      status.title = t("disks.sudo.inactive");
      status.classList.remove("is-on");
      onBtn?.removeAttribute("disabled");
      offBtn?.classList.add("hidden");
    }
  }

  async function fetchSudoStatus() {
    try {
      const st = await api("/api/ssh/sudo");
      updateSudoBanner(st);
      return st;
    } catch {
      updateSudoBanner({ enabled: false });
      return { enabled: false };
    }
  }

  function probeQuery() {
    const params = new URLSearchParams();
    params.set("sort", $("#disks-sort")?.value || "lsblk");
    params.set("filter", $("#disks-filter")?.value?.trim() || "");
    if ($("#disks-show-loop")?.checked) params.set("showLoop", "1");
    return params.toString();
  }

  function hostDiskPath(row) {
    if (!row?.devPath) return "";
    const lt = (row.lsblkType || "").toLowerCase();
    if (lt === "disk") return row.devPath;
    const idx = state.rows.indexOf(row);
    if (idx < 0) return "";
    for (let i = idx - 1; i >= 0; i--) {
      const r = state.rows[i];
      if ((r.lsblkType || "").toLowerCase() === "disk" && (r.depth || 0) < (row.depth || 0)) return r.devPath;
    }
    return "";
  }

  function renderHostList(rows) {
    const list = $("#disks-host-list");
    const smartBtn = $("#disks-host-smart");
    if (!list) return;
    if (!rows?.length) {
      list.innerHTML =
        `<div class="disks-row disks-row-empty" role="listitem"><span class="disks-col-dev">${escapeHtml(t("disks.noData"))}</span></div>`;
      if (smartBtn) smartBtn.disabled = true;
      return;
    }
    list.innerHTML = rows
      .map((r) => {
        const bar =
          r.hasDf && r.totalDf > 0
            ? `<div class="disks-usage-bar" role="progressbar" aria-valuenow="${Math.round(r.usePct)}" aria-valuemin="0" aria-valuemax="100"><div class="disks-usage-fill" style="width:${Math.min(100, Math.max(0, r.usePct))}%"></div></div>`
            : "";
        const sel = state.hostSel === r.devPath ? " is-selected" : "";
        const disk = hostDiskPath(r);
        return `<div class="disks-row${sel}" role="listitem" data-dev="${escapeHtml(r.devPath)}" data-disk="${escapeHtml(disk)}" tabindex="0">
          <span class="disks-col-dev" title="${escapeHtml(r.devPath)}">${indent(r.depth)}${escapeHtml(r.displayName)}</span>
          <span class="disks-col-type">${escapeHtml(r.typeLabel)}</span>
          <span class="disks-col-mount">${escapeHtml(emptyDash(r.mount))}</span>
          <span class="disks-col-usage">${bar}<span class="disks-usage-text">${escapeHtml(r.usageLine || "—")}</span></span>
        </div>`;
      })
      .join("");
    list.querySelectorAll(".disks-row[data-dev]").forEach((el) => {
      const pick = () => {
        state.hostSel = el.dataset.dev || null;
        const disk = el.dataset.disk || "";
        if (smartBtn) smartBtn.disabled = !disk;
        renderHostList(state.rows);
      };
      el.addEventListener("click", pick);
      el.addEventListener("keydown", (e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          pick();
        }
      });
    });
    if (smartBtn) {
      const row = state.rows.find((r) => r.devPath === state.hostSel);
      smartBtn.disabled = !hostDiskPath(row);
    }
  }

  function enhanceDiskControls() {
    const root = $("#view-disks");
    if (!root || !window.CWUI?.enhanceSelects) return;
    CWUI.enhanceSelects(root);
  }

  function renderLvSelect() {
    const sel = $("#disks-lv-select");
    if (!sel) return;
    const cur = sel.value;
    sel.innerHTML = (state.lvOptions || [])
      .map((o) => `<option value="${escapeHtml(o.path)}">${escapeHtml(o.label)}</option>`)
      .join("");
    const paths = state.lvOptions.map((o) => o.path);
    if (cur && paths.includes(cur)) sel.value = cur;
    else if (state.lvOptions[0]) sel.value = state.lvOptions[0].path;
    CWUI?.refreshSelect?.(sel);
  }

  function detailGrid(items, barHtml = "") {
    const cells = items
      .map(
        ([label, value]) =>
          `<div class="disks-detail-item"><dt>${escapeHtml(label)}</dt><dd title="${escapeHtml(value)}">${escapeHtml(value)}</dd></div>`
      )
      .join("");
    return `${barHtml}${cells}`;
  }

  function selectedLvPath() {
    const sel = $("#disks-lv-select");
    const manual = $("#disks-lv-path")?.value?.trim() || "";
    const opt = state.lvOptions.find((o) => o.path === sel?.value);
    if (opt?.path) return opt.path;
    if (!sel?.value) return manual;
    const label = sel?.selectedOptions?.[0]?.textContent || "";
    const idx = label.indexOf(" — ");
    return idx > 0 ? label.slice(0, idx).trim() : manual;
  }

  function refreshAssistantDetail() {
    const detail = $("#disks-assistant-detail");
    const vgHint = $("#disks-vg-hint");
    const pathInput = $("#disks-lv-path");
    if (!detail) return;

    const sel = $("#disks-lv-select");
    let dev = pathInput?.value?.trim() || "";
    if (sel?.value) {
      dev = sel.value;
      if (pathInput) pathInput.value = dev;
    }

    const row = state.rows.find((r) => r.devPath === dev);
    if (!dev) {
      detail.innerHTML = detailGrid([
        [t("disks.detail.mount"), "—"],
        [t("disks.detail.type"), "—"],
        [t("disks.detail.fs"), "—"],
        [t("disks.detail.usage"), "—"],
      ]);
      if (vgHint) vgHint.textContent = "";
      return;
    }
    if (!row) {
      detail.innerHTML = detailGrid([
        [t("disks.detail.mount"), "—"],
        [t("disks.detail.type"), "—"],
        [t("disks.detail.fs"), "—"],
        [t("disks.detail.usage"), "—"],
      ]);
    } else {
      const bar =
        row.hasDf && row.totalDf > 0
          ? `<div class="disks-assistant-bar" role="presentation"><div class="disks-usage-fill" style="width:${Math.min(100, Math.max(0, row.usePct))}%"></div></div>`
          : "";
      detail.innerHTML = detailGrid(
        [
          [t("disks.detail.mount"), emptyDash(row.mount)],
          [t("disks.detail.type"), emptyDash(row.typeLabel)],
          [t("disks.detail.fs"), emptyDash(row.fstype)],
          [t("disks.detail.usage"), row.usageLine || "—"],
        ],
        bar
      );
    }
    if (vgHint) {
      const vg = findVgForDev(dev);
      vgHint.textContent = vg
        ? t("disks.vg.found", { vg })
        : t("disks.vg.missing");
    }
    refreshVgSummary(dev);
    renderSnapList(dev);
    const analyzeBtn = $("#disks-analyze-mount");
    if (analyzeBtn) {
      const row = findRowForLv(dev);
      analyzeBtn.disabled = !row?.mount;
    }
  }

  function findVgForDev(dev) {
    for (const r of state.lvRecords || []) {
      if (r.path === dev && r.vg) return r.vg;
    }
    const lines = (state.lvs || "").split("\n");
    for (const line of lines) {
      const t = line.trim();
      if (!t || t.startsWith("LV Path") || t.startsWith("Path")) continue;
      const fields = t.split(/\s+/);
      if (fields[0] === dev && fields[1]) return fields[1];
    }
    return "";
  }

  function findRowForLv(dev) {
    return state.rows.find((r) => r.devPath === dev);
  }

  function refreshVgSummary(dev) {
    const box = $("#disks-vg-summary");
    if (!box) return;
    const vg = findVgForDev(dev);
    if (!vg) {
      box.textContent = "";
      return;
    }
    const st = (state.vgStats || []).find((x) => x.name === vg);
    if (!st) {
      box.textContent = t("disks.vg.summaryShort", { vg });
      return;
    }
    box.textContent = t("disks.vg.summary", {
      vg,
      size: st.sizeHuman || st.sizeGiB,
      free: st.freeHuman || st.freeGiB,
    });
  }

  function renderSnapList(dev) {
    const sel = $("#disks-snap-list");
    if (!sel) return;
    const snaps = (state.lvRecords || []).filter((r) => r.isSnapshot && r.origin === dev);
    const cur = sel.value;
    sel.innerHTML =
      `<option value="">—</option>` +
      snaps
        .map(
          (s) =>
            `<option value="${escapeHtml(s.path)}">${escapeHtml(s.lvName || s.path)} (${escapeHtml(s.sizeHuman || "")})</option>`
        )
        .join("");
    if (cur && snaps.some((s) => s.path === cur)) sel.value = cur;
    CWUI?.refreshSelect?.(sel);
    const rm = $("#disks-snap-remove");
    if (rm) rm.disabled = !sel.value;
  }

  function parseGiBInput() {
    const gStr = ($("#disks-add-gib")?.value || "").trim().replace(",", ".");
    return parseFloat(gStr, 10);
  }

  function assistantContext() {
    const lv = $("#disks-lv-path")?.value?.trim() || selectedLvPath();
    const fs = ($("#disks-fs-select")?.value || "ext4").toLowerCase();
    const gib = parseGiBInput();
    const sizeMode = $("#disks-size-mode")?.value === "absolute" ? "absolute" : "delta";
    const vg = findVgForDev(lv);
    const row = findRowForLv(lv);
    return { lv, fs, gib, sizeMode, vg, row, mount: row?.mount || "" };
  }

  async function requireSudoDisks() {
    const sudo = await fetchSudoStatus();
    if (!sudo.enabled) {
      CWUI.toast(t("disks.toast.needSudo"), "warn");
      return false;
    }
    return true;
  }

  async function diskPost(path, body, titleKey, okKey) {
    const res = await api(path, { method: "POST", body: JSON.stringify(body) });
    CWUI.toast(t(okKey), "success");
    showLvResultDialog(titleKey, res.output, okKey);
    await runProbe();
    return res;
  }

  function updateSizeModeLabel() {
    const mode = $("#disks-size-mode")?.value;
    const lbl = $("#disks-gib-label");
    if (!lbl) return;
    lbl.textContent = t(mode === "absolute" ? "disks.targetGib" : "disks.addGib");
  }

  function scheduleProbe() {
    clearTimeout(state.probeDebounce);
    state.probeDebounce = setTimeout(() => void runProbe(), 280);
  }

  async function runProbe() {
    if (state.loading) return;
    state.loading = true;
    const list = $("#disks-host-list");
    if (list && !state.rows.length) {
      list.innerHTML = `<div class="disks-row disks-row-loading">${escapeHtml(t("disks.loading"))}</div>`;
    }
    try {
      const data = await api(`/api/disks/probe?${probeQuery()}`);
      state.rows = data.rows || [];
      state.technical = data.technical || "";
      state.lvs = data.lvs || "";
      state.lvRecords = data.lvRecords || [];
      state.vgStats = data.vgStats || [];
      state.lvOptions = data.lvOptions || [];
      state.files.mountRoots = data.usageRoots || [];
      if (state.files.mountRoots.length) renderFilesRoots(state.files.mountRoots);
      const tech = $("#disks-technical-text");
      if (tech) tech.textContent = state.technical;
      const updated = $("#disks-updated-at");
      if (updated) {
        updated.textContent = data.updatedAt
          ? t("disks.updated", { time: data.updatedAt })
          : t("disks.updatedNow");
      }
      updateSudoBanner(data.sudo);
      renderHostList(state.rows);
      renderLvSelect();
      refreshAssistantDetail();
    } catch (e) {
      CWUI.toast(e.message || "Falha na sondagem", "error");
      if (list) list.innerHTML = `<div class="disks-row disks-row-empty"><span class="error">${escapeHtml(e.message)}</span></div>`;
    } finally {
      state.loading = false;
    }
  }

  function startAutoRefresh() {
    stopAutoRefresh();
    state.autoTimer = setInterval(() => {
      if (!$("#disks-auto-refresh")?.checked) return;
      if (state.tab !== "host") return;
      if (state.loading) return;
      void runProbe();
    }, 90000);
  }

  function stopAutoRefresh() {
    if (state.autoTimer) clearInterval(state.autoTimer);
    state.autoTimer = null;
  }

  async function activateSudoFromDisks() {
    if (typeof refreshSudoUI === "function") {
      $("#btn-sudo")?.click();
      return;
    }
    $("#sudo-dialog")?.showModal();
  }

  async function disableSudoFromDisks() {
    if (!(await CWConfirm(t("disks.confirm.sudoOff")))) return;
    try {
      await api("/api/ssh/sudo", { method: "POST", body: JSON.stringify({ action: "disable" }) });
      CWUI.toast(t("disks.toast.sudoOff"), "success");
      await fetchSudoStatus();
      void runProbe();
    } catch (e) {
      CWUI.toast(e.message || "Falha", "error");
    }
  }

  function showLvResultDialog(titleKey, output, fallbackMsg) {
    const outEl = $("#disks-extend-output");
    const dlg = $("#disks-extend-dialog");
    if (!outEl || !dlg) return;
    const h = dlg.querySelector("h3");
    if (h) h.textContent = t(titleKey);
    outEl.textContent = output || t(fallbackMsg);
    dlg.showModal();
  }

  async function extendLV() {
    const lv = $("#disks-lv-path")?.value?.trim() || selectedLvPath();
    if (!lv) {
      CWUI.toast("Indique o caminho do volume lógico.", "warn");
      return;
    }
    const sudo = await fetchSudoStatus();
    if (!sudo.enabled) {
      CWUI.toast("Active o sudo para ampliar volumes LVM.", "warn");
      return;
    }
    const gStr = ($("#disks-add-gib")?.value || "").trim().replace(",", ".");
    const gib = parseFloat(gStr, 10);
    if (!gib || gib <= 0) {
      CWUI.toast("Indique um valor positivo em GiB.", "warn");
      return;
    }
    const fs = $("#disks-fs-select")?.value || "ext4";
    const sizeMode = $("#disks-size-mode")?.value === "absolute" ? "absolute" : "delta";
    const ok = await CWConfirm(
      t(sizeMode === "absolute" ? "disks.confirm.extendAbs" : "disks.confirm.extend", { lv, gib, fs })
    );
    if (!ok) return;
    const btn = $("#disks-extend-lv");
    if (btn) btn.disabled = true;
    try {
      const res = await api("/api/disks/extend-lv", {
        method: "POST",
        body: JSON.stringify({ lv, gib, fs, sizeMode }),
      });
      CWUI.toast(t("disks.extend.ok"), "success");
      showLvResultDialog("disks.extend.title", res.output, "disks.extend.ok");
      await runProbe();
    } catch (e) {
      CWUI.toast(e.message || "Falha ao ampliar LV", "error");
    } finally {
      if (btn) btn.disabled = false;
    }
  }

  async function shrinkLV() {
    const lv = $("#disks-lv-path")?.value?.trim() || selectedLvPath();
    if (!lv) {
      CWUI.toast("Indique o caminho do volume lógico.", "warn");
      return;
    }
    const sudo = await fetchSudoStatus();
    if (!sudo.enabled) {
      CWUI.toast("Active o sudo para reduzir volumes LVM.", "warn");
      return;
    }
    const gStr = ($("#disks-add-gib")?.value || "").trim().replace(",", ".");
    const gib = parseFloat(gStr, 10);
    if (!gib || gib <= 0) {
      CWUI.toast("Indique um valor positivo em GiB.", "warn");
      return;
    }
    const fs = ($("#disks-fs-select")?.value || "ext4").toLowerCase();
    if (fs === "xfs") {
      CWUI.toast(t("disks.shrink.xfsUnsupported"), "error", 8000);
      return;
    }
    const sizeMode = $("#disks-size-mode")?.value === "absolute" ? "absolute" : "delta";
    const ok = await CWConfirm(
      t(sizeMode === "absolute" ? "disks.confirm.shrinkAbs" : "disks.confirm.shrink", { lv, gib, fs }),
      { danger: true, ok: t("disks.shrink") }
    );
    if (!ok) return;
    const btn = $("#disks-shrink-lv");
    if (btn) btn.disabled = true;
    try {
      const res = await api("/api/disks/shrink-lv", {
        method: "POST",
        body: JSON.stringify({ lv, gib, fs, sizeMode }),
      });
      CWUI.toast(t("disks.shrink.ok"), "success");
      showLvResultDialog("disks.shrink.title", res.output, "disks.shrink.ok");
      await runProbe();
    } catch (e) {
      CWUI.toast(e.message || "Falha ao reduzir LV", "error");
    } finally {
      if (btn) btn.disabled = false;
    }
  }

  function extractVgsBlock(txt) {
    const idx = txt.indexOf("===VGS===");
    if (idx < 0) return "";
    let rest = txt.slice(idx);
    const end = rest.indexOf("\n===PVS===");
    if (end > 0) rest = rest.slice(0, end);
    const end2 = rest.indexOf("\n\n===FIM===");
    if (end2 > 0) rest = rest.slice(0, end2);
    return rest.trim();
  }

  async function fsckLV() {
    const { lv, fs } = assistantContext();
    if (!lv) {
      CWUI.toast(t("disks.toast.needLv"), "warn");
      return;
    }
    if (!(await requireSudoDisks())) return;
    if (!(await CWConfirm(t("disks.confirm.fsck", { lv, fs })))) return;
    try {
      await diskPost("/api/disks/fsck", { lv, fs }, "disks.op.fsckTitle", "disks.op.fsckOk");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  async function createSnapshot() {
    const { lv, gib } = assistantContext();
    const name = ($("#disks-snap-name")?.value || "").trim();
    if (!lv || !name) {
      CWUI.toast(t("disks.toast.snapNeed"), "warn");
      return;
    }
    if (!gib || gib <= 0) {
      CWUI.toast(t("disks.toast.needGib"), "warn");
      return;
    }
    if (!(await requireSudoDisks())) return;
    if (!(await CWConfirm(t("disks.confirm.snapCreate", { lv, name, gib })))) return;
    try {
      await diskPost("/api/disks/snapshot-create", { lv, name, gib }, "disks.snap.createTitle", "disks.snap.createOk");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  async function removeSnapshot() {
    const snapPath = $("#disks-snap-list")?.value?.trim();
    if (!snapPath) {
      CWUI.toast(t("disks.toast.snapPick"), "warn");
      return;
    }
    if (!(await requireSudoDisks())) return;
    if (!(await CWConfirm(t("disks.confirm.snapRemove", { snap: snapPath }), { danger: true, ok: t("disks.snap.remove") })))
      return;
    try {
      await diskPost("/api/disks/snapshot-remove", { snapPath }, "disks.snap.removeTitle", "disks.snap.removeOk");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  async function resizeFsOnly(grow) {
    const { lv, fs, gib } = assistantContext();
    if (!lv || !gib || gib <= 0) {
      CWUI.toast(t("disks.toast.needGib"), "warn");
      return;
    }
    if (!grow && fs === "xfs") {
      CWUI.toast(t("disks.shrink.xfsUnsupported"), "error");
      return;
    }
    if (!(await requireSudoDisks())) return;
    const key = grow ? "disks.confirm.resizeFsGrow" : "disks.confirm.resizeFsShrink";
    if (!(await CWConfirm(t(key, { lv, gib, fs }), { danger: !grow }))) return;
    try {
      await diskPost("/api/disks/resize-fs", { lv, gib, fs, grow }, "disks.op.resizeFsTitle", "disks.op.resizeFsOk");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  async function fstrimLv() {
    const { lv, mount } = assistantContext();
    if (!lv) {
      CWUI.toast(t("disks.toast.needLv"), "warn");
      return;
    }
    if (!mount) {
      CWUI.toast(t("disks.toast.needMount"), "warn");
      return;
    }
    if (!(await requireSudoDisks())) return;
    if (!(await CWConfirm(t("disks.confirm.fstrim", { mount })))) return;
    try {
      await diskPost("/api/disks/fstrim", { lv }, "disks.op.fstrimTitle", "disks.op.fstrimOk");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  async function renameLv() {
    const { lv, vg } = assistantContext();
    if (!lv || !vg) {
      CWUI.toast(t("disks.toast.needLv"), "warn");
      return;
    }
    const newName = await CWUI?.nameInputDialog?.({
      title: t("disks.op.rename"),
      label: t("disks.rename.label"),
      ok: t("disks.rename.ok"),
    });
    if (!newName) return;
    if (!(await requireSudoDisks())) return;
    if (!(await CWConfirm(t("disks.confirm.rename", { lv, newName }), { danger: true }))) return;
    try {
      await diskPost("/api/disks/lv-rename", { lv, newName }, "disks.op.renameTitle", "disks.op.renameOk");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  async function createLv() {
    const { vg, gib, fs } = assistantContext();
    if (!vg) {
      CWUI.toast(t("disks.vg.missing"), "warn");
      return;
    }
    const name = await CWUI?.nameInputDialog?.({
      title: t("disks.op.createLv"),
      label: t("disks.createLv.label"),
    });
    if (!name) return;
    if (!gib || gib <= 0) {
      CWUI.toast(t("disks.toast.needGib"), "warn");
      return;
    }
    if (!(await requireSudoDisks())) return;
    const mkfs = await CWConfirm(t("disks.confirm.createLvMkfs", { vg, name, gib, fs }));
    if (!(await CWConfirm(t("disks.confirm.createLv", { vg, name, gib, mkfs: mkfs ? fs : "—" }), { danger: true }))) return;
    try {
      await diskPost("/api/disks/lv-create", { vg, name, gib, fs, mkfs }, "disks.op.createLvTitle", "disks.op.createLvOk");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  async function vgChange(activate) {
    const { vg } = assistantContext();
    if (!vg) {
      CWUI.toast(t("disks.vg.missing"), "warn");
      return;
    }
    if (!(await requireSudoDisks())) return;
    const key = activate ? "disks.confirm.vgAy" : "disks.confirm.vgAn";
    if (!(await CWConfirm(t(key, { vg }), { danger: !activate }))) return;
    try {
      await diskPost("/api/disks/vg-change", { vg, activate }, "disks.op.vgTitle", "disks.op.vgOk");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  function analyzeMount() {
    const { mount } = assistantContext();
    if (!mount) {
      CWUI.toast(t("disks.toast.needMount"), "warn");
      return;
    }
    setTab("files");
    void scanFilesPath(mount);
  }

  async function hostSmart() {
    const row = state.rows.find((r) => r.devPath === state.hostSel);
    const disk = hostDiskPath(row);
    if (!disk) {
      CWUI.toast(t("disks.toast.pickDisk"), "warn");
      return;
    }
    try {
      const res = await api(`/api/disks/smart?dev=${encodeURIComponent(disk)}`);
      showLvResultDialog("disks.op.smartTitle", res.output, "disks.op.smartOk");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  function jumpToVGS() {
    const tech = $("#disks-technical-text");
    const txt = tech?.textContent || state.technical || "";
    const block = extractVgsBlock(txt);
    if (!block) {
      CWUI.toast(t("disks.vgs.missing"), "warn");
      return;
    }
    const outEl = $("#disks-vgs-output");
    const dlg = $("#disks-vgs-dialog");
    if (outEl && dlg) {
      outEl.textContent = block;
      dlg.showModal();
      return;
    }
    setTab("technical");
    if (tech) {
      const idx = txt.indexOf("===VGS===");
      tech.scrollTop = idx > 20 ? idx - 20 : 0;
      tech.focus?.();
    }
  }

  function bindEvents() {
    $$(".disks-module-tab").forEach((btn) => {
      btn.addEventListener("click", () => setTab(btn.dataset.disksTab));
    });
    $("#disks-refresh")?.addEventListener("click", () => void runProbe());
    $("#disks-filter")?.addEventListener("input", scheduleProbe);
    $("#disks-sort")?.addEventListener("change", scheduleProbe);
    $("#disks-show-loop")?.addEventListener("change", scheduleProbe);
    $("#disks-lv-select")?.addEventListener("change", () => {
      const p = $("#disks-lv-select")?.value;
      const pathInput = $("#disks-lv-path");
      if (p && pathInput) pathInput.value = p;
      refreshAssistantDetail();
    });
    $("#disks-lv-path")?.addEventListener("input", refreshAssistantDetail);
    $("#disks-sudo-on")?.addEventListener("click", () => void activateSudoFromDisks());
    $("#disks-sudo-off")?.addEventListener("click", () => void disableSudoFromDisks());
    $("#disks-copy-path")?.addEventListener("click", async () => {
      const p = $("#disks-lv-path")?.value?.trim() || selectedLvPath();
      if (!p) return;
      try {
        await navigator.clipboard.writeText(p);
        CWUI.toast(t("disks.toast.pathCopied"), "success");
      } catch {
        CWUI.toast(p, "info");
      }
    });
    $("#disks-open-tech-vgs")?.addEventListener("click", jumpToVGS);
    $("#disks-jump-vgs")?.addEventListener("click", jumpToVGS);
    $("#disks-extend-lv")?.addEventListener("click", () => void extendLV());
    $("#disks-shrink-lv")?.addEventListener("click", () => void shrinkLV());
    $("#disks-size-mode")?.addEventListener("change", updateSizeModeLabel);
    $("#disks-snap-list")?.addEventListener("change", () => {
      const rm = $("#disks-snap-remove");
      if (rm) rm.disabled = !$("#disks-snap-list")?.value;
    });
    $("#disks-snap-create")?.addEventListener("click", () => void createSnapshot());
    $("#disks-snap-remove")?.addEventListener("click", () => void removeSnapshot());
    $("#disks-fsck")?.addEventListener("click", () => void fsckLV());
    $("#disks-resize-fs-grow")?.addEventListener("click", () => void resizeFsOnly(true));
    $("#disks-resize-fs-shrink")?.addEventListener("click", () => void resizeFsOnly(false));
    $("#disks-fstrim")?.addEventListener("click", () => void fstrimLv());
    $("#disks-lv-rename")?.addEventListener("click", () => void renameLv());
    $("#disks-lv-create")?.addEventListener("click", () => void createLv());
    $("#disks-vg-activate")?.addEventListener("click", () => void vgChange(true));
    $("#disks-vg-deactivate")?.addEventListener("click", () => void vgChange(false));
    $("#disks-analyze-mount")?.addEventListener("click", analyzeMount);
    $("#disks-host-smart")?.addEventListener("click", () => void hostSmart());
    $("#disks-extend-close")?.addEventListener("click", () => {
      $("#disks-extend-dialog")?.close();
      const h = $("#disks-extend-dialog")?.querySelector("h3");
      if (h) h.textContent = t("disks.extend.title");
    });
    $("#disks-vgs-close")?.addEventListener("click", () => $("#disks-vgs-dialog")?.close());
    $("#disks-vgs-goto-tech")?.addEventListener("click", () => {
      $("#disks-vgs-dialog")?.close();
      setTab("technical");
      const tech = $("#disks-technical-text");
      const idx = (tech?.textContent || "").indexOf("===VGS===");
      if (tech && idx >= 0) tech.scrollTop = idx > 20 ? idx - 20 : 0;
    });
    document.addEventListener("cw-lang-change", () => {
      applyDisksStatic();
      updateSizeModeLabel();
      refreshAssistantDetail();
      renderFilesList();
      renderHostList(state.rows);
    });
    $("#disks-auto-refresh")?.addEventListener("change", () => {
      if ($("#disks-auto-refresh")?.checked) startAutoRefresh();
      else stopAutoRefresh();
    });
    document.addEventListener("cw-sudo-changed", () => {
      void fetchSudoStatus();
      if (state.tab === "files") void scanFilesPath(state.files.path);
    });
    bindFilesEvents();
  }

  window.disksOnScreenEnter = function disksOnScreenEnter() {
    enhanceDiskControls();
    void fetchSudoStatus();
    if (!state.rows.length) void runProbe();
    if ($("#disks-auto-refresh")?.checked) startAutoRefresh();
  };

  window.disksOnScreenLeave = function disksOnScreenLeave() {
    stopAutoRefresh();
  };

  window.loadDisks = function loadDisks() {
    window.disksOnScreenEnter?.();
  };

  document.addEventListener("DOMContentLoaded", () => {
    bindEvents();
    applyDisksStatic();
    renderLvSelect();
    updateSizeModeLabel();
    enhanceDiskControls();
    renderFilesRoots([{ label: "Raiz /", path: "/" }]);
    renderFilesBreadcrumb("/");
  });
})();
