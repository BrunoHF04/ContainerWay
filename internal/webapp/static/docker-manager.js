/** Gerenciador de contêineres Docker (web). */
(function () {
  const $ = (sel) => document.querySelector(sel);
  const $$ = (sel) => document.querySelectorAll(sel);
  const escapeHtml = (s) => {
    const d = document.createElement("div");
    d.textContent = s ?? "";
    return d.innerHTML;
  };

  const DOCKER_VIEW_KEY = "cw-docker-view";
  const DOCKER_SORT_KEY = "cw-docker-sort";
  const DOCKER_QUICK_KEY = "cw-docker-quick";
  const DOCKER_SHOW_ALL_KEY = "cw-docker-show-all";
  const DOCKER_GROUP_KEY = "cw-docker-group-compose";
  const DOCKER_PINNED_KEY = "cw-docker-pinned";
  const DOCKER_POLL_KEY = "cw-docker-poll-ms";
  const DOCKER_TAB_KEY = "cw-docker-tab";

  const dockerState = {
    moduleTab: "containers",
    images: [],
    volumes: [],
    imagesFilter: "",
    imagesDanglingOnly: false,
    volumesFilter: "",
    containers: [],
    selected: new Set(),
    pinned: new Set(),
    collapsedGroups: new Set(),
    metricsHistory: new Map(),
    statsModalHistory: new Map(),
    logsRaw: "",
    focusId: null,
    pollMs: 8000,
    statsCtx: null,
    statsPoll: null,
    filter: "",
    quickFilter: "all",
    sort: "name",
    viewMode: "list",
    showAll: false,
    groupCompose: false,
    autoRefresh: true,
    pollTimer: null,
    logsCtx: null,
    logsPoll: null,
    execTerm: null,
    execFit: null,
    execSocket: null,
    lastCriticalCount: 0,
    metricsReady: false,
  };

  function containerId(c) {
    return c.idFull || c.id;
  }

  function findContainer(id) {
    if (!id) return null;
    const q = String(id).toLowerCase();
    return (
      dockerState.containers.find((c) => {
        const full = containerId(c);
        return full === id || c.id === id || full.toLowerCase() === q || (c.id && c.id.toLowerCase() === q) || full.toLowerCase().startsWith(q);
      }) || null
    );
  }

  function loadPrefs() {
    const view = localStorage.getItem(DOCKER_VIEW_KEY);
    if (view === "grid" || view === "list") dockerState.viewMode = view;
    const sort = localStorage.getItem(DOCKER_SORT_KEY);
    if (sort) dockerState.sort = sort;
    const quick = localStorage.getItem(DOCKER_QUICK_KEY);
    if (quick) dockerState.quickFilter = quick;
    dockerState.showAll = localStorage.getItem(DOCKER_SHOW_ALL_KEY) === "1";
    dockerState.groupCompose = localStorage.getItem(DOCKER_GROUP_KEY) === "1";
    const poll = parseInt(localStorage.getItem(DOCKER_POLL_KEY) || "8000", 10);
    if ([5000, 8000, 15000, 30000, 60000].includes(poll)) dockerState.pollMs = poll;
    try {
      const pins = JSON.parse(localStorage.getItem(DOCKER_PINNED_KEY) || "[]");
      if (Array.isArray(pins)) dockerState.pinned = new Set(pins);
    } catch {
      dockerState.pinned = new Set();
    }
    const tab = localStorage.getItem(DOCKER_TAB_KEY);
    if (tab === "images" || tab === "volumes" || tab === "containers") dockerState.moduleTab = tab;
  }

  function setDockerModuleTab(tab) {
    const next = tab === "images" || tab === "volumes" ? tab : "containers";
    const changed = dockerState.moduleTab !== next;
    dockerState.moduleTab = next;
    savePref(DOCKER_TAB_KEY, next);
    if (changed && next !== "containers") clearContainerFocus();
    syncDockerModuleTabs();
    refreshDockerModule();
  }

  function syncDockerModuleTabs() {
    const tab = dockerState.moduleTab;
    $$(".docker-module-tab").forEach((btn) => {
      const on = btn.dataset.dockerTab === tab;
      btn.classList.toggle("active", on);
      btn.setAttribute("aria-selected", on ? "true" : "false");
    });
    $("#docker-section-containers")?.classList.toggle("hidden", tab !== "containers");
    $("#docker-section-images")?.classList.toggle("hidden", tab !== "images");
    $("#docker-section-volumes")?.classList.toggle("hidden", tab !== "volumes");
  }

  function refreshDockerModule() {
    if (dockerState.moduleTab === "images") loadDockerImages();
    else if (dockerState.moduleTab === "volumes") loadDockerVolumes();
    else loadDockerContainers();
  }

  function savePref(key, value) {
    try {
      localStorage.setItem(key, value);
    } catch {
      /* ignore */
    }
  }

  function loadViewMode() {
    loadPrefs();
  }

  async function copyText(text, label) {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      CWUI.toast(`${label || "Texto"} copiado`, "success");
    } catch {
      CWUI.toast("Não foi possível copiar", "error");
    }
  }

  function setDockerHash(id) {
    const want = id ? `docker=${encodeURIComponent(id)}` : "";
    const cur = (location.hash || "").replace(/^#/, "");
    if (cur === want) return;
    const base = location.pathname + location.search;
    history.replaceState(null, "", want ? `${base}#${want}` : base);
  }

  function applyDockerHash() {
    const raw = (location.hash || "").replace(/^#/, "");
    if (!raw.startsWith("docker=")) return;
    const token = decodeURIComponent(raw.slice(7));
    const c = findContainer(token);
    if (c) selectContainer(c, { skipHash: true });
  }

  function isPinned(c) {
    return dockerState.pinned.has(containerId(c));
  }

  function togglePin(c, btn) {
    const id = containerId(c);
    if (dockerState.pinned.has(id)) dockerState.pinned.delete(id);
    else dockerState.pinned.add(id);
    savePref(DOCKER_PINNED_KEY, JSON.stringify([...dockerState.pinned]));
    if (btn) btn.classList.toggle("docker-pin-active", dockerState.pinned.has(id));
    btn?.setAttribute("aria-pressed", dockerState.pinned.has(id) ? "true" : "false");
    renderContainers();
  }

  function createExplorerBtn(c) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "btn btn-ghost btn-sm docker-explore-btn";
    btn.title = "Explorar ficheiros";
    btn.setAttribute("aria-label", "Explorar ficheiros");
    btn.textContent = "📁";
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      openInExplorer(c);
    });
    return btn;
  }

  function createPinButton(c) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "docker-pin-btn" + (isPinned(c) ? " docker-pin-active" : "");
    btn.title = isPinned(c) ? "Remover dos fixados" : "Fixar no topo";
    btn.setAttribute("aria-label", btn.title);
    btn.setAttribute("aria-pressed", isPinned(c) ? "true" : "false");
    btn.textContent = "★";
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      togglePin(c, btn);
    });
    return btn;
  }

  function matchQuickFilter(c) {
    const q = dockerState.quickFilter;
    if (q === "all") return true;
    const running = c.running || c.restarting;
    if (q === "running") return running;
    if (q === "stopped") return !running;
    const h = containerHealth(c);
    if (q === "warn") return h === "warn";
    if (q === "critical") return h === "critical";
    if (q === "compose") return !!(c.composeService || c.composeProject);
    if (q === "swarm") return !!c.swarmService;
    return true;
  }

  function getDisplayContainers() {
    let list = dockerState.containers.filter((c) => matchQuickFilter(c) && matchFilter(c, dockerState.filter));
    list = sortContainers(list);
    if (dockerState.pinned.size) {
      const pinned = [];
      const rest = [];
      for (const c of list) {
        if (isPinned(c)) pinned.push(c);
        else rest.push(c);
      }
      list = [...pinned, ...rest];
    }
    return list;
  }

  function computeHostSummary() {
    const all = dockerState.containers;
    let running = 0;
    let stopped = 0;
    let warn = 0;
    let critical = 0;
    let cpuSum = 0;
    let memSum = 0;
    let metricN = 0;
    for (const c of all) {
      if (c.running || c.restarting) running++;
      else stopped++;
      const h = containerHealth(c);
      if (h === "warn") warn++;
      if (h === "critical") critical++;
      if (c.metrics && (c.running || c.restarting)) {
        cpuSum += c.metrics.cpuPercent || 0;
        memSum += c.metrics.memPercent || 0;
        metricN++;
      }
    }
    return { total: all.length, running, stopped, warn, critical, cpuAvg: metricN ? cpuSum / metricN : null, memAvg: metricN ? memSum / metricN : null };
  }

  function updateSummary() {
    const el = $("#docker-summary");
    if (!el) return;
    const s = computeHostSummary();
    const shown = getDisplayContainers().length;
    const parts = [
      `<span class="docker-summary-chip"><strong>${s.running}</strong> em execução</span>`,
      s.stopped ? `<span class="docker-summary-chip"><strong>${s.stopped}</strong> parados</span>` : "",
      s.warn ? `<span class="docker-summary-chip docker-summary-warn"><strong>${s.warn}</strong> alerta</span>` : "",
      s.critical ? `<span class="docker-summary-chip docker-summary-critical"><strong>${s.critical}</strong> críticos</span>` : "",
    ].filter(Boolean);
    if (s.cpuAvg != null) {
      parts.push(`<span class="docker-summary-chip muted">CPU média ${s.cpuAvg.toFixed(0)}%</span>`);
      parts.push(`<span class="docker-summary-chip muted">RAM média ${s.memAvg.toFixed(0)}%</span>`);
    }
    if (shown !== s.total) {
      parts.push(`<span class="docker-summary-chip muted">${shown} visíveis</span>`);
    }
    el.innerHTML = parts.join("");
    el.classList.toggle("hidden", !parts.length);
  }

  function healthBadgeHtml(c) {
    const h = (c.health || "").toLowerCase();
    if (!h) return "";
    const cls = h === "healthy" ? "docker-health-badge-ok" : h === "unhealthy" ? "docker-health-badge-bad" : "docker-health-badge-pending";
    const label = h === "healthy" ? "healthy" : h === "unhealthy" ? "unhealthy" : "starting";
    return `<span class="badge docker-health-badge ${cls}" title="Healthcheck Docker">${label}</span>`;
  }

  function portsHtml(c, maxLen = 48) {
    if (!c.ports) return "";
    return `<div class="docker-ports muted" title="${escapeHtml(c.ports)}">🔌 ${escapeHtml(truncate(c.ports, maxLen))}</div>`;
  }

  function recordMetricsSample(c) {
    if (!c.metrics) return;
    const id = containerId(c);
    let arr = dockerState.metricsHistory.get(id) || [];
    arr.push({ t: Date.now(), cpu: c.metrics.cpuPercent ?? 0, mem: c.metrics.memPercent ?? 0 });
    if (arr.length > 36) arr = arr.slice(-36);
    dockerState.metricsHistory.set(id, arr);
  }

  function setQuickFilter(key) {
    dockerState.quickFilter = key;
    savePref(DOCKER_QUICK_KEY, key);
    $$(".docker-filter-chip").forEach((btn) => {
      btn.classList.toggle("active", btn.dataset.quick === key);
    });
    renderContainers();
  }

  function syncPrefsToUI() {
    const sortEl = $("#docker-sort");
    if (sortEl) sortEl.value = dockerState.sort;
    const showAll = $("#docker-show-all");
    if (showAll) showAll.checked = dockerState.showAll;
    const group = $("#docker-group-compose");
    if (group) group.checked = dockerState.groupCompose;
    const pollSel = $("#docker-poll-interval");
    if (pollSel) pollSel.value = String(dockerState.pollMs);
    $$(".docker-filter-chip").forEach((btn) => {
      btn.classList.toggle("active", btn.dataset.quick === dockerState.quickFilter);
    });
  }

  function setViewMode(mode) {
    if (dockerState.focusId) return;
    dockerState.viewMode = mode === "grid" ? "grid" : "list";
    localStorage.setItem(DOCKER_VIEW_KEY, dockerState.viewMode);
    applyViewClasses();
    renderContainers();
  }

  function applyViewClasses() {
    const box = $("#docker-list");
    if (box && !dockerState.groupCompose) {
      box.classList.toggle("docker-view-grid", dockerState.viewMode === "grid" && !dockerState.focusId);
      box.classList.toggle("docker-view-list", dockerState.viewMode === "list" || !!dockerState.focusId);
    }
    $$(".docker-view-toggle .view-toggle-btn").forEach((btn) => {
      btn.classList.toggle("active", btn.dataset.view === dockerState.viewMode);
    });
    const toggle = $(".docker-view-toggle");
    if (toggle) toggle.classList.toggle("hidden", !!dockerState.focusId);
  }

  function setSplitLayout(active) {
    const ws = $("#docker-workspace");
    const pane = $("#docker-detail-pane");
    ws?.classList.toggle("docker-split-active", active);
    pane?.classList.toggle("hidden", !active);
    $("#docker-master-toolbar")?.classList.toggle("hidden", !active);
    if (active && pane) {
      pane.classList.remove("docker-detail-enter");
      void pane.offsetWidth;
      pane.classList.add("docker-detail-enter");
    }
  }

  function selectContainer(c, opts = {}) {
    dockerState.focusId = containerId(c);
    if (!opts.skipHash) setDockerHash(dockerState.focusId);
    setSplitLayout(true);
    applyViewClasses();
    renderContainers();
    renderDetailPanel(c);
  }

  function clearContainerFocus() {
    dockerState.focusId = null;
    setDockerHash(null);
    setSplitLayout(false);
    applyViewClasses();
    renderContainers();
    const pane = $("#docker-detail-content");
    if (pane) pane.innerHTML = "";
  }

  function stateBadge(c) {
    const st = (c.state || "").toLowerCase();
    let cls = "docker-state";
    if (st === "running") cls += " docker-state-run";
    else if (st === "restarting") cls += " docker-state-restart";
    else if (st === "paused") cls += " docker-state-pause";
    else cls += " docker-state-stop";
    return `<span class="${cls}" title="${escapeHtml(c.stateLabel || c.state)}"></span>`;
  }

  function truncate(s, n) {
    if (!s || s.length <= n) return s || "";
    return s.slice(0, n - 1) + "…";
  }

  function matchFilter(c, q) {
    q = (q || "").trim().toLowerCase();
    if (!q) return true;
    const hay = [c.displayName, c.name, c.id, c.idFull, c.image, c.status, c.stateLabel].join(" ").toLowerCase();
    return hay.includes(q);
  }

  function sortContainers(list) {
    const key = dockerState.sort;
    const copy = [...list];
    copy.sort((a, b) => {
      const na = (a.displayName || a.name || a.id || "").toLowerCase();
      const nb = (b.displayName || b.name || b.id || "").toLowerCase();
      if (key === "name-desc") return nb.localeCompare(na);
      if (key === "name") return na.localeCompare(nb);
      if (key === "cpu") return (b.metrics?.cpuPercent || 0) - (a.metrics?.cpuPercent || 0);
      if (key === "mem") return (b.metrics?.memPercent || 0) - (a.metrics?.memPercent || 0);
      if (key === "restarts") return (b.metrics?.restartCount || 0) - (a.metrics?.restartCount || 0);
      if (key === "state") {
        const ar = a.running || a.restarting ? 0 : 1;
        const br = b.running || b.restarting ? 0 : 1;
        if (ar !== br) return ar - br;
        return na.localeCompare(nb);
      }
      return na.localeCompare(nb);
    });
    return copy;
  }

  function formatBytes(n) {
    if (n == null) return "—";
    const u = ["B", "KiB", "MiB", "GiB"];
    let v = Number(n);
    let i = 0;
    while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
    return `${v.toFixed(i ? 1 : 0)} ${u[i]}`;
  }

  function getContainerActions(c) {
    const id = containerId(c);
    const items = [
      { label: "Copiar ID", fn: () => copyText(id, "ID") },
      { label: "Copiar nome", fn: () => copyText(c.displayName || c.name || id, "Nome") },
      { divider: true },
      { label: "Ver logs", fn: () => openLogs(c) },
      { label: "Estatísticas", fn: () => openStats(c) },
      { label: "Explorar ficheiros", fn: () => openInExplorer(c) },
      { label: "Consola interativa", fn: () => openExec(c), hidden: !(c.running || c.restarting) },
      { divider: true },
    ];
    if (c.composeProject || c.composeService) {
      items.push({ label: "Reiniciar projeto Compose", fn: () => restartComposeProject(c) });
    }
    items.push({ label: "Criar regra de reinício automático", fn: () => openAutomationRuleForContainer(c) });
    items.push({ divider: true });
    const st = (c.state || "").toLowerCase();
    if (st === "paused") {
      items.push({ label: "Retomar", fn: () => lifecycle(c, "unpause") });
      items.push({ label: "Parar", fn: () => lifecycle(c, "stop") });
    } else if (c.running || c.restarting) {
      items.push({ label: "Reiniciar", fn: () => restartOne(c) });
      items.push({ label: "Pausar", fn: () => lifecycle(c, "pause") });
      items.push({ label: "Parar", fn: () => lifecycle(c, "stop") });
    } else {
      items.push({ label: "Iniciar", fn: () => lifecycle(c, "start") });
    }
    items.push({ divider: true });
    items.push({ label: "Remover contêiner", fn: () => removeOne(c), danger: true });
    return items.filter((it) => !it.hidden);
  }

  function createToolsMenu(c, extraClass = "") {
    const details = document.createElement("details");
    details.className = `panel-menu docker-tools-menu ${extraClass}`.trim();
    const summary = document.createElement("summary");
    summary.className = "btn btn-ghost btn-sm panel-menu-btn docker-tools-btn";
    summary.textContent = "Ferramentas";
    summary.setAttribute("aria-haspopup", "menu");
    summary.addEventListener("click", (e) => e.stopPropagation());
    summary.addEventListener("keydown", (e) => {
      if (e.key === " " || e.key === "Enter") e.stopPropagation();
    });
    const list = document.createElement("div");
    list.className = "panel-menu-list docker-tools-float";
    list.setAttribute("role", "menu");
    for (const item of getContainerActions(c)) {
      if (item.divider) {
        const sep = document.createElement("span");
        sep.className = "panel-menu-divider";
        sep.setAttribute("role", "separator");
        list.appendChild(sep);
        continue;
      }
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "panel-menu-item" + (item.danger ? " panel-menu-item-danger" : "");
      btn.textContent = item.label;
      btn.setAttribute("role", "menuitem");
      btn.addEventListener("click", (e) => {
        e.stopPropagation();
        details.removeAttribute("open");
        item.fn();
      });
      list.appendChild(btn);
    }
    details.append(summary, list);
    details.addEventListener("click", (e) => e.stopPropagation());
    if (window.CWUI?.wireFloatingDropdown) {
      CWUI.wireFloatingDropdown(details, ".panel-menu-list", "right", "docker");
    }
    return details;
  }

  function updateMasterCheckbox() {
    const master = $("#docker-select-all-master");
    const masterLbl = $(".docker-select-all-master");
    if (masterLbl) masterLbl.classList.toggle("hidden", !!dockerState.focusId);
    if (!master || dockerState.focusId) return;
    const visible = getDisplayContainers();
    const nVisible = visible.length;
    const nSelected = visible.filter((c) => dockerState.selected.has(containerId(c))).length;
    master.checked = nVisible > 0 && nSelected === nVisible;
    master.indeterminate = nSelected > 0 && nSelected < nVisible;
  }

  function updateBatchBar() {
    const bar = $("#docker-batch-bar");
    const n = dockerState.selected.size;
    if (!bar) return;
    bar.classList.toggle("hidden", n === 0 || !!dockerState.focusId);
    const lbl = $("#docker-batch-count");
    if (lbl) lbl.textContent = `${n} selecionado${n === 1 ? "" : "s"}`;
    updateMasterCheckbox();
  }

  function toggleSelectAllMaster(checked) {
    if (checked) selectAllVisible();
    else {
      dockerState.selected.clear();
      updateBatchBar();
      renderContainers();
    }
  }

  function updateStatusBar(meta) {
    const el = $("#docker-status-bar");
    if (!el) return;
    const n = meta?.count ?? dockerState.containers.length;
    const t = meta?.updatedAt ? new Date(meta.updatedAt).toLocaleTimeString() : new Date().toLocaleTimeString();
    if (dockerState.focusId) {
      const c = findContainer(dockerState.focusId);
      const name = c?.displayName || c?.name || dockerState.focusId;
      el.textContent = `${name} · ${n} no host · ${t}`;
    } else {
      el.textContent = `${n} contêiner${n === 1 ? "" : "es"} · atualizado às ${t}`;
    }
    updateSummary();
  }

  async function dockerApi(path, options = {}) {
    return api(path, options);
  }

  async function fetchContainers() {
    const q = new URLSearchParams();
    if (dockerState.showAll) q.set("all", "1");
    q.set("metrics", "1");
    return dockerApi(`/api/docker/containers?${q}`);
  }

  /** ok | warn (≥75%) | critical (≥90%) — alinhado ao alertLevel da API */
  function containerHealth(c) {
    const level = c.metrics?.alertLevel;
    if (level >= 2) return "critical";
    if (level >= 1) return "warn";
    const cpu = c.metrics?.cpuPercent ?? 0;
    const mem = c.metrics?.memPercent ?? 0;
    if (cpu >= 90 || mem >= 90) return "critical";
    if (cpu >= 75 || mem >= 75) return "warn";
    return "ok";
  }

  function applyContainerTheme(el, c, index = 0) {
    const health = containerHealth(c);
    el.classList.add(`docker-health-${health}`);
    const st = (c.state || "unknown").toLowerCase();
    el.classList.add(`docker-life-${st.replace(/[^a-z0-9_-]/g, "") || "unknown"}`);
    if (!(c.running || c.restarting)) el.classList.add("docker-life-stopped");
    if (isPinned(c)) el.classList.add("docker-item-pinned");
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (!reduced) el.style.setProperty("--docker-enter-delay", `${Math.min(index, 14) * 42}ms`);
    return health;
  }

  function metricsBlock(c, compact) {
    const cpu = c.metrics?.cpuPercent ?? null;
    const mem = c.metrics?.memPercent ?? null;
    if (!c.metrics) return `<span class="muted docker-metrics-pending">métricas indisponíveis</span>`;
    const barW = compact ? "3.5rem" : "4rem";
    return `<div class="docker-metrics${compact ? " docker-metrics-compact" : ""}">
      <span class="docker-metric" title="CPU"><span class="docker-bar" style="width:${Math.min(100, cpu || 0)}%;max-width:${barW}"></span><span class="docker-metric-val">CPU ${(cpu ?? 0).toFixed(1)}%</span></span>
      <span class="docker-metric" title="RAM"><span class="docker-bar mem" style="width:${Math.min(100, mem || 0)}%;max-width:${barW}"></span><span class="docker-metric-val">RAM ${(mem ?? 0).toFixed(0)}%</span></span>
    </div>`;
  }

  function bindCheckbox(el) {
    el.querySelector(".docker-check input")?.addEventListener("change", (e) => {
      e.stopPropagation();
      const cid = e.target.dataset.id;
      if (e.target.checked) dockerState.selected.add(cid);
      else dockerState.selected.delete(cid);
      updateBatchBar();
    });
  }

  function bindCardSelect(el, c) {
    el.addEventListener("click", (e) => {
      if (e.target.closest(".docker-check, .docker-tools-menu, .panel-menu, .docker-pin-btn, .docker-explore-btn, button, summary, a, input, label")) return;
      selectContainer(c);
    });
    el.classList.add("docker-selectable");
    if (dockerState.focusId === containerId(c)) el.classList.add("docker-item-active");
  }

  function createCompactCard(c, index = 0) {
    const id = containerId(c);
    const card = document.createElement("article");
    card.className = "docker-card docker-card-compact data-row";
    applyContainerTheme(card, c, index);
    const title = c.displayName || c.name || c.id;
    card.innerHTML = `
      <header class="docker-card-head">
        <label class="docker-check" onclick="event.stopPropagation()"><input type="checkbox" data-id="${escapeHtml(id)}" ${dockerState.selected.has(id) ? "checked" : ""} aria-label="Selecionar para lote" /></label>
        <div class="docker-card-head-text">
          <div class="docker-row-title">${stateBadge(c)}<strong>${escapeHtml(title)}</strong>${healthBadgeHtml(c)}</div>
          <div class="docker-card-tags">
            ${c.swarmService ? '<span class="badge docker-tag">Swarm</span>' : ""}
            ${c.composeService ? '<span class="badge docker-tag">Compose</span>' : ""}
            ${c.composeProject ? `<span class="badge docker-tag docker-tag-project" title="Projeto Compose">${escapeHtml(truncate(c.composeProject, 20))}</span>` : ""}
          </div>
        </div>
        <div class="docker-card-head-actions"></div>
      </header>
      <div class="docker-card-status muted">${escapeHtml(c.status || c.stateLabel || "")}</div>
      ${portsHtml(c, 40)}
      ${metricsBlock(c, true)}
    `;
    card.querySelector(".docker-card-head-actions")?.appendChild(createPinButton(c));
    const foot = document.createElement("footer");
    foot.className = "docker-card-foot";
    foot.appendChild(createExplorerBtn(c));
    foot.appendChild(createToolsMenu(c));
    card.appendChild(foot);
    bindCheckbox(card);
    bindCardSelect(card, c);
    return card;
  }

  function createListRow(c, index = 0) {
    const id = containerId(c);
    const row = document.createElement("article");
    row.className = "docker-row data-row";
    applyContainerTheme(row, c, index);
    const title = c.displayName || c.name || c.id;
    const subName = c.name && c.name !== title ? truncate(c.name, 56) : "";
    row.innerHTML = `
      <label class="docker-check" onclick="event.stopPropagation()"><input type="checkbox" data-id="${escapeHtml(id)}" ${dockerState.selected.has(id) ? "checked" : ""} aria-label="Selecionar para lote" /></label>
      <div class="docker-row-main">
        <div class="docker-row-title">${stateBadge(c)}<strong title="${escapeHtml(c.name || title)}">${escapeHtml(title)}</strong>
          ${healthBadgeHtml(c)}
          ${c.swarmService ? '<span class="badge docker-tag">Swarm</span>' : ""}
          ${c.composeService ? '<span class="badge docker-tag">Compose</span>' : ""}
        </div>
        ${subName ? `<div class="docker-row-sub muted" title="${escapeHtml(c.name)}">${escapeHtml(subName)}</div>` : ""}
        <div class="docker-row-meta muted">${escapeHtml(truncate(c.image, 72))} · ${escapeHtml(c.status || c.stateLabel || "")}</div>
        ${portsHtml(c, 64)}
        ${metricsBlock(c, false)}
      </div>`;
    const foot = document.createElement("div");
    foot.className = "docker-row-tools";
    foot.appendChild(createPinButton(c));
    foot.appendChild(createExplorerBtn(c));
    foot.appendChild(createToolsMenu(c));
    row.appendChild(foot);
    bindCheckbox(row);
    bindCardSelect(row, c);
    return row;
  }

  function createGridCard(c, index = 0) {
    const id = containerId(c);
    const card = document.createElement("article");
    card.className = "docker-card data-row";
    applyContainerTheme(card, c, index);
    const title = c.displayName || c.name || c.id;
    card.innerHTML = `
      <header class="docker-card-head">
        <label class="docker-check" onclick="event.stopPropagation()"><input type="checkbox" data-id="${escapeHtml(id)}" ${dockerState.selected.has(id) ? "checked" : ""} aria-label="Selecionar para lote" /></label>
        <div class="docker-card-head-text">
          <div class="docker-row-title">${stateBadge(c)}<strong title="${escapeHtml(c.name || title)}">${escapeHtml(title)}</strong>${healthBadgeHtml(c)}</div>
          <div class="docker-card-tags">
            ${c.swarmService ? '<span class="badge docker-tag">Swarm</span>' : ""}
            ${c.composeService ? '<span class="badge docker-tag">Compose</span>' : ""}
            ${c.composeProject ? `<span class="badge docker-tag docker-tag-project">${escapeHtml(truncate(c.composeProject, 16))}</span>` : ""}
          </div>
        </div>
        <div class="docker-card-head-actions"></div>
      </header>
      <div class="docker-card-meta muted" title="${escapeHtml(c.image)}">${escapeHtml(truncate(c.image, 48))}</div>
      <div class="docker-card-status muted">${escapeHtml(c.status || c.stateLabel || "")}</div>
      ${portsHtml(c, 40)}
      ${metricsBlock(c, true)}
    `;
    card.querySelector(".docker-card-head-actions")?.appendChild(createPinButton(c));
    const foot = document.createElement("footer");
    foot.className = "docker-card-foot";
    foot.appendChild(createExplorerBtn(c));
    foot.appendChild(createToolsMenu(c));
    card.appendChild(foot);
    bindCheckbox(card);
    bindCardSelect(card, c);
    return card;
  }

  function composeGroupKey(c) {
    if (c.composeProject) return c.composeProject;
    if (c.composeService) return "Compose (sem projeto)";
    return "— Outros";
  }

  function isRealComposeGroup(key) {
    return key !== "— Outros" && key !== "Compose (sem projeto)";
  }

  async function restartComposeProject(c) {
    const id = containerId(c);
    const project = c.composeProject || c.composeService || id;
    if (!(await CWConfirm(`Recriar todos os serviços do projeto Compose «${project}»?\n\nIsto executa compose up --force-recreate no host.`))) return;
    try {
      await dockerApi("/api/docker/compose/restart-project", {
        method: "POST",
        body: JSON.stringify({ id }),
      });
      CWUI.toast(`Projeto «${project}» reiniciado`, "success");
      loadDocker();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  function openAutomationRuleForContainer(c) {
    const target = (c.name || "").replace(/^\//, "") || c.displayName || c.id;
    sessionStorage.setItem(
      "cw-docker-auto-prefill",
      JSON.stringify({
        target,
        name: `Reinício automático: ${c.displayName || target}`,
      })
    );
    if (typeof showScreen === "function") showScreen("automations");
    else CWUI.toast("Abra a Central de automações no menu", "info");
  }

  function syncGroupControlsVisibility() {
    $("#docker-group-controls")?.classList.toggle("hidden", !dockerState.groupCompose || !!dockerState.focusId);
  }

  function expandAllGroups() {
    dockerState.collapsedGroups.clear();
    renderContainers();
  }

  function collapseAllGroups() {
    const filtered = getDisplayContainers();
    const keys = new Set(filtered.map(composeGroupKey));
    dockerState.collapsedGroups = keys;
    renderContainers();
  }

  function appendContainerNode(parent, c, index, isGrid) {
    parent.appendChild(isGrid ? createGridCard(c, index) : createListRow(c, index));
  }

  function renderGroupedList(box, filtered, isGrid) {
    const groups = new Map();
    for (const c of filtered) {
      const key = composeGroupKey(c);
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(c);
    }
    const keys = [...groups.keys()].sort((a, b) => {
      if (a === "— Outros") return 1;
      if (b === "— Outros") return -1;
      return a.localeCompare(b);
    });
    for (const key of keys) {
      const items = groups.get(key);
      const section = document.createElement("section");
      section.className = "docker-group";
      const collapsed = dockerState.collapsedGroups.has(key);
      const head = document.createElement("div");
      head.className = "docker-group-head";
      head.setAttribute("aria-expanded", collapsed ? "false" : "true");
      const toggle = document.createElement("button");
      toggle.type = "button";
      toggle.className = "docker-group-toggle";
      toggle.innerHTML = `<span class="docker-group-chevron">${collapsed ? "▸" : "▾"}</span><span class="docker-group-name">${escapeHtml(key)}</span><span class="muted docker-group-count">${items.length}</span>`;
      toggle.addEventListener("click", () => {
        if (dockerState.collapsedGroups.has(key)) dockerState.collapsedGroups.delete(key);
        else dockerState.collapsedGroups.add(key);
        renderContainers();
      });
      head.appendChild(toggle);
      if (isRealComposeGroup(key) && items[0]) {
        const restartBtn = document.createElement("button");
        restartBtn.type = "button";
        restartBtn.className = "btn btn-ghost btn-sm docker-group-restart";
        restartBtn.textContent = "Reiniciar projeto";
        restartBtn.addEventListener("click", (e) => {
          e.stopPropagation();
          restartComposeProject(items[0]);
        });
        head.appendChild(restartBtn);
      }
      const body = document.createElement("div");
      body.className = `docker-group-body${isGrid ? " docker-group-grid" : " docker-group-list"}`;
      if (collapsed) body.classList.add("hidden");
      items.forEach((c, i) => appendContainerNode(body, c, i, isGrid));
      section.append(head, body);
      box.appendChild(section);
    }
  }

  function renderContainers() {
    const box = $("#docker-list");
    if (!box) return;
    box.classList.remove("skeleton-host");
    applyViewClasses();

    const filtered = getDisplayContainers();

    if (dockerState.focusId) {
      const focused = findContainer(dockerState.focusId);
      if (!focused || !matchFilter(focused, dockerState.filter)) {
        clearContainerFocus();
        return;
      }
      box.innerHTML = "";
      box.appendChild(createCompactCard(focused, 0));
      updateBatchBar();
      renderDetailPanel(focused);
      return;
    }

    if (!filtered.length) {
      const hasQuick = dockerState.quickFilter !== "all";
      const hasText = !!dockerState.filter.trim();
      let desc = "Não há contêineres em execução. Active «Incluir parados».";
      if (hasText || hasQuick) desc = "Ajuste os filtros rápidos ou a pesquisa.";
      else if (dockerState.showAll) desc = "Não há contêineres neste host.";
      CWUI.emptyState(box, {
        icon: "🐳",
        title: hasText || hasQuick ? "Nenhum resultado" : "Nenhum contêiner",
        desc,
        actionLabel: "Atualizar",
        onAction: () => loadDocker(),
      });
      updateBatchBar();
      updateSummary();
      return;
    }

    box.innerHTML = "";
    const isGrid = dockerState.viewMode === "grid";
    if (dockerState.groupCompose && !dockerState.focusId) {
      box.classList.remove("docker-view-grid", "docker-view-list");
      renderGroupedList(box, filtered, isGrid);
    } else {
      filtered.forEach((c, index) => appendContainerNode(box, c, index, isGrid));
    }
    updateBatchBar();
    updateSummary();
  }

  function selectAllVisible() {
    for (const c of getDisplayContainers()) dockerState.selected.add(containerId(c));
    updateBatchBar();
    renderContainers();
  }

  function detailField(label, value, mono) {
    const v = value == null || value === "" ? "—" : String(value);
    return `<div class="docker-detail-field"><dt>${escapeHtml(label)}</dt><dd class="${mono ? "mono" : ""}">${escapeHtml(v)}</dd></div>`;
  }

  function recordStatsModalSample(id, m) {
    if (!m) return;
    let arr = dockerState.statsModalHistory.get(id) || [];
    arr.push({ t: Date.now(), cpu: m.cpuPercent ?? 0, mem: m.memPercent ?? 0 });
    if (arr.length > 60) arr = arr.slice(-60);
    dockerState.statsModalHistory.set(id, arr);
  }

  function sparklineFromHistory(historyMap, id, field, label, large) {
    const arr = historyMap.get(id) || [];
    if (arr.length < 2) return "";
    const vals = arr.map((x) => x[field]);
    const max = Math.max(...vals, 1);
    const bars = vals
      .map((v) => `<span style="height:${Math.max(8, Math.round((v / max) * 100))}%"></span>`)
      .join("");
    const cls = large ? "docker-sparkline docker-sparkline-lg" : "docker-sparkline";
    return `<div class="docker-sparkline-wrap${large ? " docker-sparkline-wrap-lg" : ""}"><span class="muted docker-sparkline-label">${escapeHtml(label)}</span><div class="${cls}" title="${arr.length} amostras">${bars}</div></div>`;
  }

  function renderMetricsSection(m, sparkId, opts = {}) {
    if (!m) return '<p class="muted">Métricas indisponíveis.</p>';
    const large = !!opts.large;
    const hist = opts.useStatsHistory ? dockerState.statsModalHistory : dockerState.metricsHistory;
    const sparks =
      sparkId && (hist.get(sparkId)?.length || 0) >= 2
        ? `<div class="docker-sparklines${large ? " docker-sparklines-lg" : ""}">${sparklineFromHistory(hist, sparkId, "cpu", "CPU", large)}${sparklineFromHistory(hist, sparkId, "mem", "RAM", large)}</div>`
        : "";
    return `
      ${sparks}
      <div class="docker-detail-metrics">
        <div class="docker-stat-card"><span>CPU</span><strong>${(m.cpuPercent ?? 0).toFixed(1)}%</strong></div>
        <div class="docker-stat-card"><span>RAM</span><strong>${formatBytes(m.memUsage)} / ${formatBytes(m.memLimit)}</strong><small class="muted">${(m.memPercent ?? 0).toFixed(0)}%</small></div>
        <div class="docker-stat-card"><span>Rede</span><strong>↓ ${formatBytes(m.netRx)} · ↑ ${formatBytes(m.netTx)}</strong></div>
        <div class="docker-stat-card"><span>Disco</span><strong>↓ ${formatBytes(m.diskRead)} · ↑ ${formatBytes(m.diskWrite)}</strong></div>
        <div class="docker-stat-card"><span>PIDs</span><strong>${m.pids ?? "—"}</strong></div>
        <div class="docker-stat-card"><span>Uptime</span><strong>${escapeHtml(m.uptime || "—")}</strong></div>
        <div class="docker-stat-card"><span>Reinícios</span><strong>${m.restartCount ?? 0}</strong></div>
      </div>`;
  }

  async function renderDetailPanel(c) {
    const pane = $("#docker-detail-content");
    const outer = $("#docker-detail-pane");
    if (!pane || !c) return;
    const title = c.displayName || c.name || c.id;
    const id = containerId(c);
    const health = containerHealth(c);
    pane.classList.remove("docker-health-ok", "docker-health-warn", "docker-health-critical");
    pane.classList.add(`docker-health-${health}`);
    if (outer) {
      outer.classList.remove("docker-health-ok", "docker-health-warn", "docker-health-critical");
      outer.classList.add(`docker-health-${health}`);
    }
    pane.classList.remove("docker-detail-body-enter");
    void pane.offsetWidth;
    pane.classList.add("docker-detail-body-enter");
    pane.innerHTML = `
      <header class="docker-detail-head glass-bar-inline">
        <div class="docker-detail-head-text">
          <div class="docker-detail-title-row">${stateBadge(c)}<h3>${escapeHtml(title)}</h3></div>
          <p class="muted">${escapeHtml(c.stateLabel || c.state || "")} · ${escapeHtml(c.status || "")}</p>
        </div>
        <div class="docker-detail-head-actions"></div>
      </header>
      <div class="docker-detail-body">
        <section class="docker-detail-section">
          <h4>Recursos</h4>
          ${renderMetricsSection(c.metrics, id)}
        </section>
        <section class="docker-detail-section">
          <h4>Identificação</h4>
          <dl class="docker-detail-dl">
            ${detailField("Nome Docker", c.name, true)}
            ${detailField("ID", c.id, true)}
            ${detailField("ID completo", id, true)}
            ${detailField("Imagem", c.image, true)}
            ${detailField("Portas", c.ports || "—", true)}
            ${detailField("Healthcheck", c.health || "—")}
            ${detailField("Projeto Compose", c.composeProject)}
            ${detailField("Serviço Swarm", c.swarmService)}
            ${detailField("Serviço Compose", c.composeService)}
          </dl>
        </section>
        <section class="docker-detail-section" id="docker-detail-inspect-wrap">
          <h4>Configuração</h4>
          <p class="muted">A carregar inspect…</p>
        </section>
      </div>`;

    const headActions = pane.querySelector(".docker-detail-head-actions");
    if (headActions) {
      if (c.composeProject || c.composeService) {
        const composeBtn = document.createElement("button");
        composeBtn.type = "button";
        composeBtn.className = "btn btn-ghost btn-sm";
        composeBtn.textContent = "Reiniciar projeto";
        composeBtn.addEventListener("click", () => restartComposeProject(c));
        headActions.appendChild(composeBtn);
      }
      const autoBtn = document.createElement("button");
      autoBtn.type = "button";
      autoBtn.className = "btn btn-ghost btn-sm";
      autoBtn.textContent = "Regra automática";
      autoBtn.addEventListener("click", () => openAutomationRuleForContainer(c));
      headActions.appendChild(autoBtn);
      const copyId = document.createElement("button");
      copyId.type = "button";
      copyId.className = "btn btn-ghost btn-sm";
      copyId.textContent = "Copiar ID";
      copyId.addEventListener("click", () => copyText(id, "ID"));
      const copyName = document.createElement("button");
      copyName.type = "button";
      copyName.className = "btn btn-ghost btn-sm";
      copyName.textContent = "Copiar nome";
      copyName.addEventListener("click", () => copyText(c.displayName || c.name || id, "Nome"));
      headActions.append(copyId, copyName);
      headActions.appendChild(createToolsMenu(c, "docker-tools-detail"));
      const closeBtn = document.createElement("button");
      closeBtn.type = "button";
      closeBtn.className = "btn btn-ghost btn-sm";
      closeBtn.id = "docker-detail-close";
      closeBtn.title = "Fechar detalhes";
      closeBtn.textContent = "×";
      closeBtn.addEventListener("click", clearContainerFocus);
      headActions.appendChild(closeBtn);
    }

    const wrap = pane.querySelector("#docker-detail-inspect-wrap");
    try {
      const data = await dockerApi(`/api/docker/inspect?id=${encodeURIComponent(id)}`);
      const ins = data.inspect || {};
      let labelsHtml = "";
      const labels = ins.labels || {};
      const keys = Object.keys(labels).sort();
      if (keys.length) {
        labelsHtml = `<div class="docker-detail-labels"><h5>Labels</h5><dl class="docker-detail-dl">${keys.map((k) => detailField(k, labels[k], true)).join("")}</dl></div>`;
      }
      wrap.innerHTML = `
        <h4>Configuração</h4>
        <dl class="docker-detail-dl">
          ${detailField("Hostname", ins.hostname)}
          ${detailField("Política de reinício", ins.restartPolicy)}
          ${detailField("Contagem de reinícios", ins.restartCount)}
          ${detailField("Iniciado em", ins.startedAt, true)}
          ${detailField("Código de saída", ins.exitCode != null ? ins.exitCode : "—")}
        </dl>
        ${labelsHtml}`;
    } catch (e) {
      wrap.innerHTML = `<h4>Configuração</h4><p class="error">${escapeHtml(e.message)}</p>`;
    }
  }

  async function loadDocker() {
    refreshDockerModule();
  }

  async function loadDockerContainers() {
    const box = $("#docker-list");
    if (!box) return;
    if (!dockerState.focusId) CWUI.skeletonList(box, 5);
    try {
      const data = await fetchContainers();
      dockerState.containers = data.containers || [];
      for (const c of dockerState.containers) recordMetricsSample(c);
      updateStatusBar(data);
      const summary = computeHostSummary();
      if (dockerState.metricsReady && summary.critical > dockerState.lastCriticalCount) {
        CWUI.toast(
          `${summary.critical - dockerState.lastCriticalCount} contêiner(es) passaram a estado crítico (≥90% RAM/CPU)`,
          "warn"
        );
      }
      dockerState.lastCriticalCount = summary.critical;
      dockerState.metricsReady = true;
      renderContainers();
      syncGroupControlsVisibility();
      if (!dockerState.focusId) applyDockerHash();
      if (typeof window.onExplorerContainersLoaded === "function") {
        window.onExplorerContainersLoaded(dockerState.containers);
      }
    } catch (e) {
      box.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
    }
  }

  async function lifecycle(c, action, extra = {}) {
    const id = containerId(c);
    const labels = { stop: "Parar", start: "Iniciar", pause: "Pausar", unpause: "Retomar", remove: "Remover" };
    const danger = action === "remove";
    if (!(await CWConfirm(`${labels[action] || action} ${c.displayName || c.name || id}?`, { danger }))) return;
    try {
      await dockerApi(`/api/docker/${action}`, {
        method: "POST",
        body: JSON.stringify({ id, ...extra }),
      });
      CWUI.toast(`${labels[action] || action} concluído`, "success");
      dockerState.selected.delete(id);
      if (action === "remove") clearContainerFocus();
      loadDocker();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  async function restartOne(c) {
    const id = containerId(c);
    if (!(await CWConfirm(`Reiniciar ${c.displayName || c.name || id}?`))) return;
    try {
      await dockerApi("/api/docker/restart", { method: "POST", body: JSON.stringify({ id }) });
      CWUI.toast("Contêiner reiniciado", "success");
      loadDocker();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  async function removeOne(c) {
    const id = containerId(c);
    const msg = (c.running || c.restarting)
      ? `O contêiner está em execução. Remover à força?`
      : `Remover permanentemente ${c.displayName || c.name || id}?`;
    if (!(await CWConfirm(msg, { danger: true }))) return;
    try {
      await dockerApi("/api/docker/remove", {
        method: "POST",
        body: JSON.stringify({ id, force: true }),
      });
      CWUI.toast("Contêiner removido", "success");
      dockerState.selected.delete(id);
      clearContainerFocus();
      loadDocker();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  function showBatchResult(action, okCount, errors) {
    const dlg = $("#docker-batch-result-dialog");
    const title = $("#docker-batch-result-title");
    const summary = $("#docker-batch-result-summary");
    const body = $("#docker-batch-result-body");
    if (!dlg) return;
    const labels = { restart: "Reinício", stop: "Paragem", remove: "Remoção", start: "Início" };
    if (title) title.textContent = `${labels[action] || action} em lote`;
    const errList = errors || [];
    if (summary) {
      summary.textContent =
        errList.length === 0
          ? `${okCount} contêiner(es) processado(s) com sucesso.`
          : `${okCount} com sucesso · ${errList.length} com erro.`;
      summary.className = errList.length ? "warn" : "muted";
    }
    if (body) {
      if (errList.length) {
        body.textContent = errList.join("\n");
        body.classList.remove("hidden");
      } else {
        body.textContent = "";
        body.classList.add("hidden");
      }
    }
    dlg.showModal();
  }

  async function batchAction(action) {
    const ids = [...dockerState.selected];
    if (!ids.length) return;
    const labels = { restart: "Reiniciar", stop: "Parar", remove: "Remover", start: "Iniciar" };
    if (!(await CWConfirm(`${labels[action]} ${ids.length} contêiner(es)?`, { danger: action === "remove" }))) return;
    const batchErrors = [];
    let okCount = 0;
    try {
      if (action === "restart") {
        const res = await dockerApi("/api/docker/restart-batch", { method: "POST", body: JSON.stringify({ ids }) });
        okCount = res.restarted || 0;
        batchErrors.push(...(res.errors || []));
      } else {
        for (const id of ids) {
          try {
            await dockerApi(`/api/docker/${action}`, {
              method: "POST",
              body: JSON.stringify({ id, force: action === "remove" }),
            });
            okCount++;
          } catch (e) {
            const c = findContainer(id);
            const name = c?.displayName || c?.name || id;
            batchErrors.push(`${name}: ${e.message}`);
          }
        }
      }
      dockerState.selected.clear();
      loadDocker();
      if (batchErrors.length) showBatchResult(action, okCount, batchErrors);
      else CWUI.toast(`${labels[action]} em lote: ${okCount} concluído(s)`, "success");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  function parseLogLines(raw) {
    const lines = [];
    const parts = String(raw || "").split(/\n\[stderr\]\n/);
    const push = (text, stream) => {
      for (const line of text.split("\n")) {
        lines.push({ stream, text: line });
      }
    };
    if (parts.length === 2) {
      push(parts[0], "stdout");
      push(parts[1], "stderr");
    } else {
      push(raw || "", "stdout");
    }
    return lines;
  }

  function classifyLogLine(text, stream) {
    if (stream === "stderr") return "docker-log-stderr";
    if (/\b(ERROR|FATAL|PANIC|CRITICAL|Exception|Traceback)\b/i.test(text)) return "docker-log-error";
    if (/\b(WARN|WARNING)\b/i.test(text)) return "docker-log-warn";
    return "";
  }

  function highlightSearch(text, query) {
    if (!query) return escapeHtml(text);
    const q = query.toLowerCase();
    const lower = text.toLowerCase();
    let out = "";
    let i = 0;
    while (i < text.length) {
      const idx = lower.indexOf(q, i);
      if (idx === -1) {
        out += escapeHtml(text.slice(i));
        break;
      }
      out += escapeHtml(text.slice(i, idx));
      out += `<mark class="docker-log-mark">${escapeHtml(text.slice(idx, idx + query.length))}</mark>`;
      i = idx + query.length;
    }
    return out;
  }

  function renderLogsHtml(raw, query) {
    const q = (query || "").trim().toLowerCase();
    const lines = parseLogLines(raw);
    const html = [];
    let shown = 0;
    for (const { stream, text } of lines) {
      if (q && !text.toLowerCase().includes(q)) continue;
      shown++;
      const cls = classifyLogLine(text, stream);
      html.push(`<span class="docker-log-line ${cls}">${highlightSearch(text, q)}</span>`);
    }
    if (!shown) return '<span class="muted docker-log-line">Nenhuma linha corresponde ao filtro.</span>';
    return html.join("\n");
  }

  async function resolveContainerStartedAt(c) {
    if (c._startedAt !== undefined) return c._startedAt;
    try {
      const data = await dockerApi(`/api/docker/inspect?id=${encodeURIComponent(containerId(c))}`);
      c._startedAt = data.inspect?.startedAt || "";
    } catch {
      c._startedAt = "";
    }
    return c._startedAt;
  }

  function applyLogsToBody() {
    const body = $("#docker-logs-body");
    if (!body) return;
    const query = $("#docker-logs-search")?.value || "";
    body.innerHTML = renderLogsHtml(dockerState.logsRaw, query);
    const wrap = $("#docker-logs-wrap");
    body.classList.toggle("docker-logs-wrap", wrap?.checked !== false);
    body.classList.toggle("docker-logs-nowrap", wrap?.checked === false);
  }

  function openLogs(c) {
    const dlg = $("#docker-logs-dialog");
    const body = $("#docker-logs-body");
    const title = $("#docker-logs-title");
    if (!dlg || !body) return;
    dockerState.logsCtx = c;
    dockerState.logsRaw = "";
    c._startedAt = undefined;
    title.textContent = `Logs — ${c.displayName || c.name || c.id}`;
    body.textContent = "A carregar…";
    const search = $("#docker-logs-search");
    if (search) search.value = "";
    dlg.showModal();
    loadLogsPanel(true);
    if (dockerState.logsPoll) clearInterval(dockerState.logsPoll);
    let live = true;
    const autoBtn = $("#docker-logs-auto");
    if (autoBtn) {
      autoBtn.textContent = "Pausar live";
      autoBtn.onclick = () => {
        live = !live;
        autoBtn.textContent = live ? "Pausar live" : "Retomar live";
      };
    }
    dockerState.logsPoll = setInterval(() => { if (live && dlg.open) loadLogsPanel(false); }, 2500);
    dlg.onclose = () => {
      clearInterval(dockerState.logsPoll);
      dockerState.logsPoll = null;
      dockerState.logsCtx = null;
    };
  }

  async function loadLogsPanel(forceScroll) {
    const c = dockerState.logsCtx;
    if (!c) return;
    const id = containerId(c);
    const tail = $("#docker-logs-tail")?.value || "500";
    const status = $("#docker-logs-status");
    const body = $("#docker-logs-body");
    const atBottom = body && !forceScroll && body.scrollHeight - body.scrollTop - body.clientHeight < 48;
    try {
      let url = `/api/docker/logs?id=${encodeURIComponent(id)}&tail=${encodeURIComponent(tail)}`;
      if ($("#docker-logs-since")?.checked) {
        const since = await resolveContainerStartedAt(c);
        if (since) url += `&since=${encodeURIComponent(since)}`;
      }
      const data = await dockerApi(url);
      dockerState.logsRaw = data.logs || "(vazio)";
      applyLogsToBody();
      if (body && (forceScroll || atBottom)) body.scrollTop = body.scrollHeight;
      const q = $("#docker-logs-search")?.value.trim();
      const lineCount = parseLogLines(dockerState.logsRaw).length;
      const shown = q
        ? parseLogLines(dockerState.logsRaw).filter((l) => l.text.toLowerCase().includes(q.toLowerCase())).length
        : lineCount;
      if (status) {
        status.textContent = `Atualizado ${new Date().toLocaleTimeString()} · ${shown}/${lineCount} linhas`;
      }
    } catch (e) {
      if (status) status.textContent = e.message;
    }
  }

  function closeStatsPanel() {
    if (dockerState.statsPoll) {
      clearInterval(dockerState.statsPoll);
      dockerState.statsPoll = null;
    }
    dockerState.statsCtx = null;
  }

  async function loadStatsPanel() {
    const c = dockerState.statsCtx;
    const body = $("#docker-stats-body");
    if (!c || !body) return;
    const id = containerId(c);
    try {
      const data = await dockerApi(`/api/docker/stats?id=${encodeURIComponent(id)}`);
      const m = data.metrics || c.metrics;
      if (!m) {
        body.innerHTML = "<p class=\"muted\">Sem métricas (contêiner parado?).</p>";
        return;
      }
      recordStatsModalSample(id, m);
      body.innerHTML = renderMetricsSection(m, id, { large: true, useStatsHistory: true });
    } catch (e) {
      body.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
    }
  }

  async function openStats(c) {
    const dlg = $("#docker-stats-dialog");
    const body = $("#docker-stats-body");
    const title = $("#docker-stats-title");
    if (!dlg || !body) return;
    closeStatsPanel();
    dockerState.statsCtx = c;
    dockerState.statsModalHistory.set(containerId(c), dockerState.metricsHistory.get(containerId(c))?.slice() || []);
    title.textContent = `Stats — ${c.displayName || c.name || c.id}`;
    body.innerHTML = '<p class="muted">A carregar…</p>';
    dlg.showModal();
    await loadStatsPanel();
    dockerState.statsPoll = setInterval(() => {
      if (dlg.open) loadStatsPanel();
    }, 2000);
    dlg.onclose = () => closeStatsPanel();
  }

  async function openInspect(c) {
    selectContainer(c);
  }

  function closeExec() {
    if (dockerState.execSocket) {
      dockerState.execSocket.close();
      dockerState.execSocket = null;
    }
    if (dockerState.execTerm) {
      dockerState.execTerm.dispose();
      dockerState.execTerm = null;
      dockerState.execFit = null;
    }
  }

  function dockerExecWSUrl(id) {
    const p = location.protocol === "https:" ? "wss:" : "ws:";
    return `${p}//${location.host}/api/docker/exec/ws?id=${encodeURIComponent(id)}`;
  }

  function openExec(c) {
    const dlg = $("#docker-exec-dialog");
    const box = $("#docker-exec-terminal");
    const title = $("#docker-exec-title");
    if (!dlg || !box) return;
    closeExec();
    title.textContent = `Consola — ${c.displayName || c.name || c.id}`;
    dlg.showModal();
    if (!window.Terminal) {
      box.textContent = "Terminal xterm não carregado.";
      return;
    }
    dockerState.execTerm = new Terminal({
      theme: { background: "#0f1419", foreground: "#e8eef7", cursor: "#3b82f6" },
      fontSize: 13,
    });
    dockerState.execFit = new (window.FitAddon?.FitAddon || FitAddon.FitAddon)();
    dockerState.execTerm.loadAddon(dockerState.execFit);
    dockerState.execTerm.open(box);
    dockerState.execFit.fit();
    const id = containerId(c);
    const ws = new WebSocket(dockerExecWSUrl(id));
    dockerState.execSocket = ws;
    ws.onopen = () => dockerState.execTerm.writeln("\r\n\x1b[32mLigado ao contêiner.\x1b[0m\r\n");
    ws.onmessage = (ev) => dockerState.execTerm.write(ev.data);
    ws.onclose = () => dockerState.execTerm.writeln("\r\n\x1b[33mSessão terminada.\x1b[0m\r\n");
    dockerState.execTerm.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(data);
    });
    dlg.onclose = () => closeExec();
  }

  function openInExplorer(c) {
    if (typeof window.openDockerContainerInExplorer === "function") {
      window.openDockerContainerInExplorer(c);
      return;
    }
    showScreen("explorer");
    CWUI.toast("Abra o explorador e seleccione o contêiner no painel remoto.", "info");
  }

  function matchImagesFilter(img, q) {
    q = (q || "").trim().toLowerCase();
    if (dockerState.imagesDanglingOnly && !img.dangling) return false;
    if (!q) return true;
    const hay = [img.display, img.id, ...(img.tags || [])].join(" ").toLowerCase();
    return hay.includes(q);
  }

  function renderImages() {
    const host = $("#docker-images-list");
    if (!host) return;
    const filtered = dockerState.images.filter((img) => matchImagesFilter(img, dockerState.imagesFilter));
    if (!filtered.length) {
      CWUI.emptyState(host, {
        icon: "📦",
        title: dockerState.imagesFilter || dockerState.imagesDanglingOnly ? "Nenhum resultado" : "Nenhuma imagem",
        desc: "Ajuste o filtro ou actualize a lista.",
        actionLabel: "Atualizar",
        onAction: loadDockerImages,
      });
      return;
    }
    const rows = filtered
      .map((img) => {
        const tags = (img.tags || []).slice(0, 3).join(", ") || img.display;
        const moreTags = (img.tags || []).length > 3 ? ` +${img.tags.length - 3}` : "";
        return `<tr class="${img.dangling ? "docker-resource-dangling" : ""}">
          <td class="docker-resource-name" title="${escapeHtml((img.tags || []).join("\n"))}">${escapeHtml(tags)}${escapeHtml(moreTags)}</td>
          <td class="mono muted">${escapeHtml(img.id)}</td>
          <td>${escapeHtml(img.sizeHuman)}</td>
          <td class="muted">${escapeHtml(img.createdLabel)}</td>
          <td>${img.containers}</td>
          <td class="docker-resource-actions">
            <button type="button" class="btn btn-ghost btn-sm" data-copy-image="${escapeHtml(img.idFull || img.id)}">Copiar ID</button>
            <button type="button" class="btn btn-ghost btn-sm btn-danger" data-remove-image="${escapeHtml(img.idFull || img.id)}" data-image-label="${escapeHtml(img.display)}">Remover</button>
          </td>
        </tr>`;
      })
      .join("");
    host.innerHTML = `<div class="docker-resource-table-wrap glass-card"><table class="docker-resource-table">
      <thead><tr><th>Tag / nome</th><th>ID</th><th>Tamanho</th><th>Criada</th><th>Em uso</th><th></th></tr></thead>
      <tbody>${rows}</tbody></table></div>`;
    host.querySelectorAll("[data-copy-image]").forEach((btn) => {
      btn.addEventListener("click", () => copyText(btn.dataset.copyImage, "ID da imagem"));
    });
    host.querySelectorAll("[data-remove-image]").forEach((btn) => {
      btn.addEventListener("click", () => removeDockerImage(btn.dataset.removeImage, btn.dataset.imageLabel));
    });
  }

  async function loadDockerImages() {
    const host = $("#docker-images-list");
    const status = $("#docker-images-status");
    if (!host) return;
    CWUI.skeletonList(host, 6);
    try {
      const data = await dockerApi("/api/docker/images");
      dockerState.images = data.images || [];
      if (status) {
        status.textContent = `${data.count ?? dockerState.images.length} imagens · ${data.totalHuman || "—"} · ${new Date(data.updatedAt || Date.now()).toLocaleTimeString()}`;
      }
      renderImages();
    } catch (e) {
      host.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
    }
  }

  async function removeDockerImage(id, label) {
    if (!(await CWConfirm(`Remover imagem «${label || id}»?`, { danger: true }))) return;
    const img = dockerState.images.find((i) => (i.idFull || i.id) === id);
    let useForce = false;
    if (img && img.containers > 0) {
      if (!(await CWConfirm("A imagem está em uso por contêineres. Remover à força?", { danger: true }))) return;
      useForce = true;
    }
    try {
      await dockerApi("/api/docker/images/remove", {
        method: "POST",
        body: JSON.stringify({ id, force: useForce }),
      });
      CWUI.toast("Imagem removida", "success");
      loadDockerImages();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  function matchVolumesFilter(vol, q) {
    q = (q || "").trim().toLowerCase();
    if (!q) return true;
    const hay = [vol.name, vol.driver, vol.mountpoint].join(" ").toLowerCase();
    return hay.includes(q);
  }

  function renderVolumes() {
    const host = $("#docker-volumes-list");
    if (!host) return;
    const filtered = dockerState.volumes.filter((v) => matchVolumesFilter(v, dockerState.volumesFilter));
    if (!filtered.length) {
      CWUI.emptyState(host, {
        icon: "💾",
        title: dockerState.volumesFilter ? "Nenhum resultado" : "Nenhum volume",
        desc: "Não há volumes neste host.",
        actionLabel: "Atualizar",
        onAction: loadDockerVolumes,
      });
      return;
    }
    const rows = filtered
      .map(
        (v) => `<tr>
          <td class="docker-resource-name" title="${escapeHtml(v.name)}">${escapeHtml(v.name)}</td>
          <td>${escapeHtml(v.driver || "—")}</td>
          <td class="mono muted docker-resource-mount">${escapeHtml(truncate(v.mountpoint || "—", 48))}</td>
          <td class="muted">${escapeHtml(v.createdLabel || "—")}</td>
          <td class="docker-resource-actions">
            <button type="button" class="btn btn-ghost btn-sm" data-copy-vol="${escapeHtml(v.name)}">Copiar nome</button>
            <button type="button" class="btn btn-ghost btn-sm btn-danger" data-remove-vol="${escapeHtml(v.name)}">Remover</button>
          </td>
        </tr>`
      )
      .join("");
    host.innerHTML = `<div class="docker-resource-table-wrap glass-card"><table class="docker-resource-table">
      <thead><tr><th>Nome</th><th>Driver</th><th>Mountpoint</th><th>Criado</th><th></th></tr></thead>
      <tbody>${rows}</tbody></table></div>`;
    host.querySelectorAll("[data-copy-vol]").forEach((btn) => {
      btn.addEventListener("click", () => copyText(btn.dataset.copyVol, "Nome"));
    });
    host.querySelectorAll("[data-remove-vol]").forEach((btn) => {
      btn.addEventListener("click", () => removeDockerVolume(btn.dataset.removeVol));
    });
  }

  async function loadDockerVolumes() {
    const host = $("#docker-volumes-list");
    const status = $("#docker-volumes-status");
    if (!host) return;
    CWUI.skeletonList(host, 6);
    try {
      const data = await dockerApi("/api/docker/volumes");
      dockerState.volumes = data.volumes || [];
      if (status) {
        status.textContent = `${data.count ?? dockerState.volumes.length} volumes · ${new Date(data.updatedAt || Date.now()).toLocaleTimeString()}`;
      }
      renderVolumes();
    } catch (e) {
      host.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
    }
  }

  async function removeDockerVolume(name) {
    if (!(await CWConfirm(`Remover volume «${name}»?`, { danger: true }))) return;
    try {
      await dockerApi("/api/docker/volumes/remove", {
        method: "POST",
        body: JSON.stringify({ name, force: false }),
      });
      CWUI.toast("Volume removido", "success");
      loadDockerVolumes();
    } catch (e) {
      if (/in use|being used/i.test(e.message) && (await CWConfirm("Volume em uso. Remover à força?", { danger: true }))) {
        try {
          await dockerApi("/api/docker/volumes/remove", {
            method: "POST",
            body: JSON.stringify({ name, force: true }),
          });
          CWUI.toast("Volume removido", "success");
          loadDockerVolumes();
        } catch (e2) {
          CWUI.toast(e2.message, "error");
        }
      } else {
        CWUI.toast(e.message, "error");
      }
    }
  }

  function startAutoPoll() {
    stopAutoPoll();
    dockerState.pollTimer = setInterval(() => {
      if (state.screen === "docker" && dockerState.autoRefresh && state.ssh.connected) {
        if (dockerState.moduleTab === "containers") loadDockerContainers();
        else if (dockerState.moduleTab === "images") loadDockerImages();
        else if (dockerState.moduleTab === "volumes") loadDockerVolumes();
      }
    }, dockerState.pollMs);
  }

  function stopAutoPoll() {
    if (dockerState.pollTimer) {
      clearInterval(dockerState.pollTimer);
      dockerState.pollTimer = null;
    }
  }

  function bindDockerUI() {
    loadViewMode();
    syncPrefsToUI();
    syncDockerModuleTabs();
    $$(".docker-module-tab").forEach((btn) => {
      btn.addEventListener("click", () => setDockerModuleTab(btn.dataset.dockerTab || "containers"));
    });
    $("#docker-images-refresh")?.addEventListener("click", loadDockerImages);
    $("#docker-images-filter")?.addEventListener("input", (e) => {
      dockerState.imagesFilter = e.target.value;
      renderImages();
    });
    $("#docker-images-dangling")?.addEventListener("change", (e) => {
      dockerState.imagesDanglingOnly = e.target.checked;
      renderImages();
    });
    $("#docker-volumes-refresh")?.addEventListener("click", loadDockerVolumes);
    $("#docker-volumes-filter")?.addEventListener("input", (e) => {
      dockerState.volumesFilter = e.target.value;
      renderVolumes();
    });
    applyViewClasses();
    $$(".docker-view-toggle .view-toggle-btn").forEach((btn) => {
      btn.addEventListener("click", () => setViewMode(btn.dataset.view));
    });
    $$(".docker-filter-chip").forEach((btn) => {
      btn.addEventListener("click", () => setQuickFilter(btn.dataset.quick || "all"));
    });
    $("#docker-back-list")?.addEventListener("click", clearContainerFocus);
    $("#docker-refresh")?.addEventListener("click", loadDocker);
    $("#docker-filter")?.addEventListener("input", (e) => {
      dockerState.filter = e.target.value;
      renderContainers();
    });
    $("#docker-sort")?.addEventListener("change", (e) => {
      dockerState.sort = e.target.value;
      savePref(DOCKER_SORT_KEY, dockerState.sort);
      renderContainers();
    });
    $("#docker-show-all")?.addEventListener("change", (e) => {
      dockerState.showAll = e.target.checked;
      savePref(DOCKER_SHOW_ALL_KEY, dockerState.showAll ? "1" : "0");
      loadDocker();
    });
    $("#docker-group-compose")?.addEventListener("change", (e) => {
      dockerState.groupCompose = e.target.checked;
      savePref(DOCKER_GROUP_KEY, dockerState.groupCompose ? "1" : "0");
      syncGroupControlsVisibility();
      renderContainers();
    });
    $("#docker-groups-expand")?.addEventListener("click", expandAllGroups);
    $("#docker-groups-collapse")?.addEventListener("click", collapseAllGroups);
    $("#docker-auto-refresh")?.addEventListener("change", (e) => {
      dockerState.autoRefresh = e.target.checked;
      if (dockerState.autoRefresh) startAutoPoll();
      else stopAutoPoll();
    });
    $("#docker-poll-interval")?.addEventListener("change", (e) => {
      const ms = parseInt(e.target.value, 10);
      if ([5000, 8000, 15000, 30000, 60000].includes(ms)) {
        dockerState.pollMs = ms;
        savePref(DOCKER_POLL_KEY, String(ms));
        if (dockerState.autoRefresh) startAutoPoll();
      }
    });
    $("#docker-export")?.addEventListener("click", () => {
      const q = dockerState.showAll ? "?all=1" : "";
      window.open(`/api/docker/export${q}`, "_blank");
    });
    $("#docker-select-all-master")?.addEventListener("change", (e) => {
      toggleSelectAllMaster(e.target.checked);
    });
    $("#docker-batch-start")?.addEventListener("click", () => batchAction("start"));
    $("#docker-batch-restart")?.addEventListener("click", () => batchAction("restart"));
    $("#docker-batch-stop")?.addEventListener("click", () => batchAction("stop"));
    $("#docker-batch-remove")?.addEventListener("click", () => batchAction("remove"));
    $("#docker-batch-clear")?.addEventListener("click", () => {
      dockerState.selected.clear();
      renderContainers();
    });
    document.addEventListener("keydown", (e) => {
      if (typeof state === "undefined" || state.screen !== "docker") return;
      const tag = (e.target?.tagName || "").toLowerCase();
      if (tag === "input" || tag === "textarea" || tag === "select" || e.target?.isContentEditable) return;
      if (e.key === "r" || e.key === "R") {
        e.preventDefault();
        loadDocker();
      } else if (e.key === "/") {
        e.preventDefault();
        $("#docker-filter")?.focus();
      } else if (e.key === "Escape" && dockerState.focusId) {
        clearContainerFocus();
      }
    });
    $("#docker-logs-refresh")?.addEventListener("click", () => loadLogsPanel(true));
    $("#docker-logs-since")?.addEventListener("change", () => loadLogsPanel(true));
    $("#docker-logs-wrap")?.addEventListener("change", applyLogsToBody);
    $("#docker-logs-search")?.addEventListener("input", applyLogsToBody);
    $("#docker-logs-tail")?.addEventListener("change", () => loadLogsPanel(true));
    $("#docker-logs-close")?.addEventListener("click", () => $("#docker-logs-dialog")?.close());
    $("#docker-logs-copy")?.addEventListener("click", async () => {
      const t = dockerState.logsRaw || $("#docker-logs-body")?.textContent || "";
      try {
        await navigator.clipboard.writeText(t);
        CWUI.toast("Copiado", "success");
      } catch {
        CWUI.toast("Não foi possível copiar", "error");
      }
    });
    $("#docker-stats-refresh")?.addEventListener("click", loadStatsPanel);
    $("#docker-stats-close")?.addEventListener("click", () => $("#docker-stats-dialog")?.close());
    $("#docker-batch-result-close")?.addEventListener("click", () => $("#docker-batch-result-dialog")?.close());
    $("#docker-inspect-close")?.addEventListener("click", () => $("#docker-inspect-dialog")?.close());
    $("#docker-exec-close")?.addEventListener("click", () => {
      $("#docker-exec-dialog")?.close();
      closeExec();
    });

  }

  window.loadDocker = loadDocker;
  window.dockerOnScreenEnter = function () {
    syncPrefsToUI();
    syncDockerModuleTabs();
    if (dockerState.moduleTab === "containers") applyDockerHash();
    else refreshDockerModule();
  };
  window.initDockerManager = function () {
    bindDockerUI();
    syncGroupControlsVisibility();
    startAutoPoll();
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => window.initDockerManager?.());
  } else {
    window.initDockerManager?.();
  }
})();
