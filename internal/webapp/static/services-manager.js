/** Serviços systemd no host SSH (web). */
(function () {
  const $ = (sel) => document.querySelector(sel);
  const tr = (k, v) => (window.CWI18n && window.CWI18n.t(k, v)) || k;

  const escapeHtml = (s) => {
    const d = document.createElement("div");
    d.textContent = s ?? "";
    return d.innerHTML;
  };

  const state = {
    rows: [],
    loading: false,
    filter: "",
    stateFilter: "all",
    sort: "name",
    autoTimer: null,
    logsUnit: null,
  };

  function localeForSort() {
    const code = window.CWI18n?.lang?.() || "pt";
    return code === "en" ? "en" : code === "es" ? "es" : "pt-BR";
  }

  function isRunning(row) {
    const a = (row.active || "").toLowerCase();
    return a === "active" || a === "activating" || a === "reloading";
  }

  function isFailed(row) {
    return (row.active || "").toLowerCase() === "failed";
  }

  function bootState(row) {
    return (row.enabled || row.loadState || "").toLowerCase();
  }

  function bootFixed(row) {
    const b = bootState(row);
    return b === "static" || b === "masked";
  }

  function stateBadge(row) {
    const a = (row.active || "—").toLowerCase();
    let cls = "svc-badge";
    if (isRunning(row)) cls += " svc-badge--ok";
    else if (isFailed(row)) cls += " svc-badge--err";
    else cls += " svc-badge--off";
    const sub = row.sub && row.sub !== "dead" ? ` · ${row.sub}` : "";
    return `<span class="${cls}">${escapeHtml(row.active || "—")}${escapeHtml(sub)}</span>`;
  }

  const bootKeyMap = {
    enabled: "svc.boot.enabled",
    disabled: "svc.boot.disabled",
    static: "svc.boot.static",
    masked: "svc.boot.masked",
    indirect: "svc.boot.indirect",
  };

  function bootBadge(row) {
    const e = (row.enabled || row.loadState || "—").toLowerCase();
    let cls = "svc-badge svc-badge--boot";
    if (e === "enabled") cls += " svc-badge--ok";
    else if (e === "masked" || e === "disabled") cls += " svc-badge--warn";
    const label = bootKeyMap[e] ? tr(bootKeyMap[e]) : row.enabled || row.loadState || "—";
    return `<span class="${cls}">${escapeHtml(label)}</span>`;
  }

  function filteredRows() {
    const q = state.filter.trim().toLowerCase();
    let list = [...state.rows];
    if (state.stateFilter === "active") list = list.filter(isRunning);
    else if (state.stateFilter === "inactive") list = list.filter((r) => !isRunning(r) && !isFailed(r));
    else if (state.stateFilter === "failed") list = list.filter(isFailed);
    if (q) {
      list = list.filter((r) => {
        const hay = `${r.unit} ${r.description}`.toLowerCase();
        return hay.includes(q);
      });
    }
    const loc = localeForSort();
    if (state.sort === "state") {
      list.sort((a, b) => {
        const sa = (a.active || "").toLowerCase();
        const sb = (b.active || "").toLowerCase();
        if (sa !== sb) return sa.localeCompare(sb, loc);
        return (a.unit || "").localeCompare(b.unit || "", loc);
      });
    } else {
      list.sort((a, b) => (a.unit || "").localeCompare(b.unit || "", loc));
    }
    return list;
  }

  function renderList() {
    const list = $("#svc-list");
    const summary = $("#svc-summary");
    if (!list) return;
    const rows = filteredRows();
    const running = state.rows.filter(isRunning).length;
    const failed = state.rows.filter(isFailed).length;
    if (summary) {
      summary.textContent = tr("svc.summary", {
        visible: rows.length,
        running,
        failed,
        total: state.rows.length,
      });
    }
    if (state.loading) {
      list.innerHTML = `<div class="svc-row svc-row-empty" role="listitem"><span class="svc-col-name">${escapeHtml(tr("svc.loading"))}</span></div>`;
      return;
    }
    if (!rows.length) {
      list.innerHTML = `<div class="svc-row svc-row-empty" role="listitem"><span class="svc-col-name">${escapeHtml(tr("svc.empty"))}</span></div>`;
      return;
    }
    const canCtrl = typeof canAction === "function" ? canAction("services.control") : true;
    list.innerHTML = rows
      .map((r) => {
        const run = isRunning(r);
        const unit = r.unit || "";
        const short = unit.replace(/\.service$/, "");
        const boot = bootState(r);
        const bootLocked = bootFixed(r);
        const bootOn = boot === "enabled";
        const bootOff = boot === "disabled";
        const actions = canCtrl
          ? `<span class="svc-actions-group svc-actions-now" title="${escapeHtml(tr("svc.group.nowTitle"))}">
              <button type="button" class="btn btn-ghost btn-sm svc-act" data-act="start" data-unit="${escapeHtml(unit)}" ${run ? "disabled" : ""} title="${escapeHtml(tr("svc.act.startTitle"))}">${escapeHtml(tr("svc.act.start"))}</button>
              <button type="button" class="btn btn-ghost btn-sm svc-act" data-act="stop" data-unit="${escapeHtml(unit)}" ${!run ? "disabled" : ""} title="${escapeHtml(tr("svc.act.stopTitle"))}">${escapeHtml(tr("svc.act.stop"))}</button>
              <button type="button" class="btn btn-ghost btn-sm svc-act" data-act="restart" data-unit="${escapeHtml(unit)}" title="${escapeHtml(tr("svc.act.restartTitle"))}">${escapeHtml(tr("svc.act.restart"))}</button>
            </span>
            <span class="svc-actions-group svc-actions-boot" title="${escapeHtml(tr("svc.group.bootTitle"))}">
              <button type="button" class="btn btn-ghost btn-sm svc-act svc-act-boot" data-act="enable" data-unit="${escapeHtml(unit)}" ${bootLocked || bootOn ? "disabled" : ""} title="${escapeHtml(tr("svc.act.enableTitle"))}">${escapeHtml(tr("svc.act.enable"))}</button>
              <button type="button" class="btn btn-ghost btn-sm svc-act svc-act-boot" data-act="disable" data-unit="${escapeHtml(unit)}" ${bootLocked || bootOff ? "disabled" : ""} title="${escapeHtml(tr("svc.act.disableTitle"))}">${escapeHtml(tr("svc.act.disable"))}</button>
            </span>`
          : `<span class="muted">${escapeHtml(tr("common.noPerm"))}</span>`;
        return `<div class="svc-row" role="listitem" data-unit="${escapeHtml(unit)}">
          <span class="svc-col-name" title="${escapeHtml(unit)}"><strong>${escapeHtml(short)}</strong><span class="svc-unit-suffix muted">.service</span></span>
          <span class="svc-col-state">${stateBadge(r)}</span>
          <span class="svc-col-boot" title="${escapeHtml(tr("svc.boot.colTitle"))}">${bootBadge(r)}</span>
          <span class="svc-col-desc" title="${escapeHtml(r.description || "")}">${escapeHtml(r.description || "—")}</span>
          <span class="svc-col-actions">
            <button type="button" class="btn btn-ghost btn-sm svc-logs" data-unit="${escapeHtml(unit)}" title="${escapeHtml(tr("svc.logs"))}">${escapeHtml(tr("svc.logs"))}</button>
            ${actions}
          </span>
        </div>`;
      })
      .join("");
  }

  function updateSudoBanner(sudo) {
    const status = $("#svc-sudo-status");
    const onBtn = $("#svc-sudo-on");
    const offBtn = $("#svc-sudo-off");
    if (!status) return;
    if (sudo?.enabled && sudo.user) {
      status.textContent = tr("svc.sudo.active", { user: sudo.user });
      status.title = tr("svc.sudo.activeTitle", { user: sudo.user });
      status.classList.add("is-on");
      onBtn?.setAttribute("disabled", "disabled");
      offBtn?.classList.remove("hidden");
    } else {
      status.textContent = tr("svc.sudo.inactive");
      status.title = tr("svc.sudo.inactiveTitle");
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

  async function loadServicesList() {
    if (state.loading) return;
    state.loading = true;
    renderList();
    try {
      const data = await api("/api/services");
      state.rows = Array.isArray(data.services) ? data.services : [];
      const at = $("#svc-updated-at");
      if (at) {
        const d = data.updatedAt ? new Date(data.updatedAt) : new Date();
        at.textContent = d.toLocaleTimeString(localeForSort());
      }
    } catch (e) {
      state.rows = [];
      CWUI.toast(e.message || tr("svc.toast.listFail"), "error");
    } finally {
      state.loading = false;
      renderList();
    }
  }

  async function serviceAction(unit, action) {
    const actionKeys = {
      start: "svc.confirm.start",
      stop: "svc.confirm.stop",
      restart: "svc.confirm.restart",
      enable: "svc.confirm.enable",
      disable: "svc.confirm.disable",
    };
    const actionLabel = tr(actionKeys[action] || action);
    const ok = await CWConfirm(tr("svc.confirm.body", { action: actionLabel, unit }), {
      ok: action === "stop" || action === "disable" ? tr("common.confirm") : tr("common.yes"),
      danger: action === "stop" || action === "disable",
    });
    if (!ok) return;
    try {
      await api("/api/services/control", {
        method: "POST",
        body: JSON.stringify({ unit, action }),
      });
      CWUI.toast(tr("svc.toast.ok", { unit, action: actionLabel }), "success");
      await loadServicesList();
    } catch (e) {
      CWUI.toast(e.message || tr("svc.toast.opFail"), "error");
    }
  }

  async function openLogs(unit) {
    state.logsUnit = unit;
    const dlg = $("#svc-logs-dialog");
    const title = $("#svc-logs-title");
    const body = $("#svc-logs-body");
    if (title) title.textContent = tr("svc.logs.titleUnit", { unit });
    if (body) body.textContent = tr("common.loading");
    dlg?.showModal();
    try {
      const data = await api(`/api/services/logs?unit=${encodeURIComponent(unit)}&lines=120`);
      if (body) body.textContent = data.log || tr("common.empty");
    } catch (e) {
      if (body) body.textContent = e.message || tr("svc.toast.fail");
    }
  }

  function startAutoRefresh() {
    stopAutoRefresh();
    state.autoTimer = setInterval(() => {
      if (!$("#svc-auto-refresh")?.checked) return;
      if (!document.querySelector("#view-services.active")) return;
      if (state.loading) return;
      void loadServicesList();
    }, 30000);
  }

  function stopAutoRefresh() {
    if (state.autoTimer) clearInterval(state.autoTimer);
    state.autoTimer = null;
  }

  function bindEvents() {
    $("#svc-refresh")?.addEventListener("click", () => void loadServicesList());
    $("#svc-filter")?.addEventListener("input", (ev) => {
      state.filter = ev.target.value || "";
      renderList();
    });
    $("#svc-state-filter")?.addEventListener("change", (ev) => {
      state.stateFilter = ev.target.value || "all";
      renderList();
    });
    $("#svc-sort")?.addEventListener("change", (ev) => {
      state.sort = ev.target.value || "name";
      renderList();
    });
    $("#svc-sudo-on")?.addEventListener("click", () => {
      if (typeof refreshSudoUI === "function") $("#btn-sudo")?.click();
      else $("#sudo-dialog")?.showModal();
    });
    $("#svc-sudo-off")?.addEventListener("click", async () => {
      if (!(await CWConfirm(tr("svc.confirm.sudoOff")))) return;
      try {
        await api("/api/ssh/sudo", { method: "POST", body: JSON.stringify({ action: "disable" }) });
        CWUI.toast(tr("svc.toast.sudoOff"), "success");
        await fetchSudoStatus();
      } catch (e) {
        CWUI.toast(e.message || tr("svc.toast.fail"), "error");
      }
    });
    $("#svc-auto-refresh")?.addEventListener("change", () => {
      if ($("#svc-auto-refresh")?.checked) startAutoRefresh();
      else stopAutoRefresh();
    });
    $("#svc-list")?.addEventListener("click", (ev) => {
      const act = ev.target.closest?.(".svc-act");
      if (act?.dataset.unit && act.dataset.act) {
        void serviceAction(act.dataset.unit, act.dataset.act);
        return;
      }
      const logs = ev.target.closest?.(".svc-logs");
      if (logs?.dataset.unit) void openLogs(logs.dataset.unit);
    });
    $("#svc-logs-refresh")?.addEventListener("click", () => {
      if (state.logsUnit) void openLogs(state.logsUnit);
    });
    $("#svc-logs-close")?.addEventListener("click", () => $("#svc-logs-dialog")?.close());
    document.addEventListener("cw-sudo-changed", () => void fetchSudoStatus());
    document.addEventListener("cw-lang-change", () => {
      window.CWI18n?.applyLabels?.();
      renderList();
      void fetchSudoStatus();
    });
  }

  window.servicesOnScreenLeave = function servicesOnScreenLeave() {
    stopAutoRefresh();
  };

  window.loadServices = function loadServices() {
    void fetchSudoStatus();
    void loadServicesList();
    if ($("#svc-auto-refresh")?.checked) startAutoRefresh();
  };

  document.addEventListener("DOMContentLoaded", bindEvents);
})();
