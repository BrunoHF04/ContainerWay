/** Gerenciador de contêineres Docker (web). */
(function () {
  const $ = (sel) => document.querySelector(sel);
  const $$ = (sel) => document.querySelectorAll(sel);
  const escapeHtml = (s) => {
    const d = document.createElement("div");
    d.textContent = s ?? "";
    return d.innerHTML;
  };

  const tr = (k, v) => (window.CWI18n && window.CWI18n.t(k, v)) || k;

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
    networks: [],
    systemUsage: null,
    imagesFilter: "",
    imagesDanglingOnly: false,
    volumesFilter: "",
    networksFilter: "",
    containers: [],
    selected: new Set(),
    pinned: new Set(),
    collapsedGroups: new Set(),
    metricsHistory: new Map(),
    statsModalHistory: new Map(),
    logsRaw: "",
    focusId: null,
    pollMs: 30000,
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
    pendingOps: new Map(),
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
    const poll = parseInt(localStorage.getItem(DOCKER_POLL_KEY) || "30000", 10);
    if ([5000, 8000, 15000, 30000, 60000].includes(poll)) dockerState.pollMs = poll;
    const prefSec = Number(window.CWWebPrefs?.get?.("dockerMetricsSec"));
    if (prefSec >= 5 && prefSec <= 120) dockerState.pollMs = prefSec * 1000;
    try {
      const pins = JSON.parse(localStorage.getItem(DOCKER_PINNED_KEY) || "[]");
      if (Array.isArray(pins)) dockerState.pinned = new Set(pins);
    } catch {
      dockerState.pinned = new Set();
    }
    const tab = localStorage.getItem(DOCKER_TAB_KEY);
    if (tab === "images" || tab === "volumes" || tab === "networks" || tab === "containers") dockerState.moduleTab = tab;
  }

  function setDockerModuleTab(tab) {
    const next =
      tab === "images" || tab === "volumes" || tab === "networks" ? tab : "containers";
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
    $("#docker-section-networks")?.classList.toggle("hidden", tab !== "networks");
    if (!window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      const section = document.getElementById(`docker-section-${tab}`);
      if (section && !section.classList.contains("hidden")) {
        section.classList.remove("section-enter");
        void section.offsetWidth;
        section.classList.add("section-enter");
        section.addEventListener(
          "animationend",
          () => section.classList.remove("section-enter"),
          { once: true }
        );
      }
    }
  }

  function refreshDockerModule() {
    loadDockerSystem();
    if (dockerState.moduleTab === "images") loadDockerImages();
    else if (dockerState.moduleTab === "volumes") loadDockerVolumes();
    else if (dockerState.moduleTab === "networks") loadDockerNetworks();
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
      CWUI.toast(tr("docker.toast.copied", { label: label || "Texto" }), "success");
    } catch {
      CWUI.toast(tr("common.copyFail"), "error");
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
    btn.title = "Explorar arquivos";
    btn.setAttribute("aria-label", "Explorar arquivos");
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
    btn.title = isPinned(c) ? tr("docker.pin.remove") : tr("docker.pin.add");
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
    const quickSel = $("#docker-quick-filter");
    if (quickSel && quickSel.value !== key) quickSel.value = key;
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
    const quickSel = $("#docker-quick-filter");
    if (quickSel) quickSel.value = dockerState.quickFilter;
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

  function sleep(ms) {
    return new Promise((r) => setTimeout(r, ms));
  }

  function getPendingOp(c) {
    return dockerState.pendingOps.get(containerId(c));
  }

  function setPendingOp(id, op) {
    if (op) dockerState.pendingOps.set(id, op);
    else dockerState.pendingOps.delete(id);
  }

  function setDockerOpMessage(msg) {
    const el = $("#docker-op-message");
    if (el) el.textContent = msg;
  }

  function setDockerOpStep(text, current = 0, total = 0) {
    const step = $("#docker-op-step");
    if (step) step.textContent = text || "";
    const wrap = $("#docker-op-progress-wrap");
    const fill = $("#docker-op-progress-fill");
    if (wrap && fill) {
      if (total > 0) {
        wrap.classList.remove("hidden");
        fill.style.width = `${Math.min(100, Math.round((current / total) * 100))}%`;
      } else {
        wrap.classList.add("hidden");
        fill.style.width = "0%";
      }
    }
  }

  function showDockerOp(title, message, detail = "") {
    const dlg = $("#docker-op-dialog");
    if (!dlg) return;
    const t = $("#docker-op-title");
    if (t) t.textContent = title;
    setDockerOpMessage(message);
    const d = $("#docker-op-detail");
    if (d) {
      d.textContent = detail;
      d.classList.toggle("hidden", !detail);
    }
    if (!dlg.open) dlg.showModal();
  }

  function hideDockerOp() {
    setDockerOpStep("", 0, 0);
    $("#docker-op-dialog")?.close();
  }

  const RECREATE_PHASE = {
    wait: { class: "is-wait", icon: "○", label: "Na fila" },
    stop: { class: "is-stop", icon: "■", label: "A parar contêiner antigo…" },
    recreate: { class: "is-recreate", icon: "↻", label: "A recriar a partir do YAML…" },
    start: { class: "is-start", icon: "▶", label: "A iniciar serviço…" },
    done: { class: "is-done", icon: "✓", label: "Concluído" },
    error: { class: "is-error", icon: "!", label: "Erro" },
  };

  let recreateProgressState = null;

  function containerDisplayName(c) {
    return (c.displayName || c.name || c.id || "").replace(/^\//, "");
  }

  function renderRecreateProgressList() {
    const ul = $("#docker-recreate-progress-list");
    if (!ul || !recreateProgressState) return;
    ul.innerHTML = recreateProgressState.items
      .map((item) => {
        const ph = RECREATE_PHASE[item.phase] || RECREATE_PHASE.wait;
        const label = item.phase === "error" && item.error ? item.error : ph.label;
        const proj = item.project
          ? `<span class="docker-recreate-item-project">Projeto: ${escapeHtml(item.project)}</span>`
          : "";
        return `<li class="docker-recreate-item ${ph.class}" data-key="${escapeHtml(item.key)}">
        <span class="docker-recreate-item-icon" aria-hidden="true">${ph.icon}</span>
        <div class="docker-recreate-item-body">
          <strong class="docker-recreate-item-name" title="${escapeHtml(item.name)}">${escapeHtml(item.name)}</strong>
          <span class="docker-recreate-item-status">${escapeHtml(label)}</span>
          ${proj}
        </div>
      </li>`;
      })
      .join("");
    updateRecreateProgressBar();
  }

  function updateRecreateProgressBar() {
    if (!recreateProgressState) return;
    const fill = $("#docker-recreate-progress-fill");
    if (!fill) return;
    const total = recreateProgressState.items.length || 1;
    const done = recreateProgressState.items.filter((i) => i.phase === "done" || i.phase === "error").length;
    fill.style.width = `${Math.min(100, Math.round((done / total) * 100))}%`;
  }

  function setRecreateSummary(text) {
    const el = $("#docker-recreate-progress-summary");
    if (el) el.textContent = text;
  }

  function setItemsPhase(matchFn, phase, errorMsg = "") {
    if (!recreateProgressState) return;
    for (const item of recreateProgressState.items) {
      if (!matchFn(item)) continue;
      item.phase = phase;
      if (errorMsg) item.error = errorMsg;
      else if (phase !== "error") item.error = "";
    }
    renderRecreateProgressList();
  }

  function openRecreateProgressUI(title, items) {
    const dlg = $("#docker-recreate-progress-dialog");
    const titleEl = $("#docker-recreate-progress-title");
    const closeBtn = $("#docker-recreate-progress-close");
    if (titleEl) titleEl.textContent = title;
    recreateProgressState = {
      items: items.map((it) => ({ ...it, phase: "wait", error: "" })),
      running: true,
    };
    setRecreateSummary("A preparar operação no host remoto…");
    renderRecreateProgressList();
    if (closeBtn) closeBtn.disabled = true;
    if (dlg && !dlg.open) dlg.showModal();
  }

  function closeRecreateProgressUI() {
    recreateProgressState = null;
    $("#docker-recreate-progress-dialog")?.close();
  }

  function buildRecreateItemsFromContainers(containers) {
    return containers.map((c) => ({
      key: containerId(c),
      name: containerDisplayName(c),
      project: (c.composeProject || "").trim(),
    }));
  }

  function findContainerForRecreateItem(item) {
    const want = item.name.toLowerCase();
    const project = (item.project || "").toLowerCase();
    return dockerState.containers.find((x) => {
      const n = containerDisplayName(x).toLowerCase();
      if (n === want || n.startsWith(want) || want.startsWith(n)) return true;
      if (project && (x.composeProject || "").toLowerCase() === project) {
        const svc = (x.composeService || "").toLowerCase();
        if (svc && (want.includes(svc) || n.includes(svc))) return true;
      }
      return false;
    });
  }

  async function waitRecreateItemsRunning(items, maxSec = 45) {
    const pending = () => items.filter((it) => it.phase !== "done" && it.phase !== "error");
    const maxRounds = Math.max(1, Math.ceil(maxSec / 1.5));
    for (let round = 0; round < maxRounds; round++) {
      if (!pending().length) return true;
      const left = pending().length;
      setRecreateSummary(
        left === 1
          ? `A asalvar "${pending()[0].name}" no host…`
          : `A asalvar ${left} serviço(s) no host… (${round + 1}/${maxRounds})`
      );
      for (const item of pending()) {
        setItemsPhase((i) => i.key === item.key, "start");
      }
      await refreshContainersLight();
      for (const item of pending()) {
        const c = findContainerForRecreateItem(item);
        if (!c) continue;
        if (c.running) setItemsPhase((i) => i.key === item.key, "done");
        else if (c.restarting) setItemsPhase((i) => i.key === item.key, "start");
      }
      if (!pending().length) return true;
      await sleep(round < 4 ? 1000 : 2000);
    }
    for (const item of pending()) {
      setItemsPhase((i) => i.key === item.key, "error", "Tempo esgotado à espera do arranque");
    }
    return false;
  }

  async function runRecreateFlow(containers) {
    const targets = containers.filter(Boolean);
    if (!targets.length) return { ok: 0, errors: [] };

    const items = buildRecreateItemsFromContainers(targets);
    openRecreateProgressUI(
      targets.length === 1 ? `A recriar "${items[0].name}"` : `A recriar ${targets.length} contêineres`,
      items
    );

    for (const c of targets) setPendingOp(containerId(c), { title: "Recriar" });
    renderContainers();

    const byProject = new Map();
    const standalone = [];
    for (const c of targets) {
      const proj = (c.composeProject || "").trim();
      if (proj) {
        if (!byProject.has(proj)) byProject.set(proj, c);
      } else {
        standalone.push(c);
      }
    }

    const errors = [];
    let ok = 0;

    try {
      for (const [project, rep] of byProject) {
        setRecreateSummary(`Projeto "${project}": a parar contêineres antigos…`);
        setItemsPhase((i) => i.project === project, "stop");
        await sleep(120);
        setRecreateSummary(`Projeto "${project}": a recriar (docker compose up --force-recreate)…`);
        setItemsPhase((i) => i.project === project, "recreate");
        try {
          await dockerApi("/api/docker/compose/restart-project", {
            method: "POST",
            body: JSON.stringify({ id: containerId(rep) }),
          });
          const projectItems = recreateProgressState.items.filter((i) => i.project === project);
          setRecreateSummary(`Projeto "${project}": a iniciar serviços…`);
          setItemsPhase((i) => i.project === project, "start");
          await waitRecreateItemsRunning(projectItems);
          for (const item of projectItems) {
            if (item.phase === "done") ok++;
            else if (item.phase === "error") errors.push(`${item.name}: ${item.error || "falha"}`);
          }
        } catch (e) {
          setItemsPhase((i) => i.project === project, "error", e.message);
          errors.push(`Projeto ${project}: ${e.message}`);
        }
      }

      for (const c of standalone) {
        const key = containerId(c);
        const name = containerDisplayName(c);
        setRecreateSummary(`${name}: a parar contêiner antigo…`);
        setItemsPhase((i) => i.key === key, "stop");
        await sleep(120);
        setRecreateSummary(`${name}: a recriar…`);
        setItemsPhase((i) => i.key === key, "recreate");
        try {
          await dockerApi("/api/docker/restart", {
            method: "POST",
            body: JSON.stringify({ id: key, compose: true }),
          });
          const one = recreateProgressState.items.filter((i) => i.key === key);
          setItemsPhase((i) => i.key === key, "start");
          await waitRecreateItemsRunning(one);
          if (one[0]?.phase === "done") ok++;
          else errors.push(`${name}: ${one[0]?.error || "falha"}`);
        } catch (e) {
          setItemsPhase((i) => i.key === key, "error", e.message);
          errors.push(`${name}: ${e.message}`);
        }
      }

      const doneN = recreateProgressState.items.filter((i) => i.phase === "done").length;
      const errN = recreateProgressState.items.filter((i) => i.phase === "error").length;
      setRecreateSummary(
        errN
          ? `${doneN} concluído(s) · ${errN} com erro. Revise a lista e feche quando quiser.`
          : `${doneN} contêiner(es) recriado(s) com sucesso. Pode fechar.`
      );
    } finally {
      if (recreateProgressState) recreateProgressState.running = false;
      const closeBtn = $("#docker-recreate-progress-close");
      if (closeBtn) closeBtn.disabled = false;
      for (const c of targets) setPendingOp(containerId(c), null);
      loadDocker();
    }

    return { ok, errors };
  }

  async function refreshContainersLight() {
    const data = await fetchContainers();
    dockerState.containers = data.containers || [];
    updateStatusBar(data);
    renderContainers();
    if (dockerState.focusId) {
      const c = findContainer(dockerState.focusId);
      if (c) renderDetailPanel(c);
    }
    return dockerState.focusId ? findContainer(dockerState.focusId) : null;
  }

  async function pollContainerState(c, { wantRunning = false, wantStopped = false, maxSec = 90 } = {}) {
    const id = containerId(c);
    const steps = Math.max(1, Math.ceil(maxSec / 2));
    for (let i = 0; i < steps; i++) {
      setDockerOpMessage(`A acompanhar estado no host… (${i + 1}/${steps})`);
      await sleep(2000);
      try {
        const data = await fetchContainers();
        dockerState.containers = data.containers || [];
        const fresh = findContainer(id);
        if (!fresh) continue;
        renderContainers();
        if (dockerState.focusId === id) renderDetailPanel(fresh);
        const st = (fresh.state || "").toLowerCase();
        if (wantRunning) {
          if (fresh.restarting || st === "restarting") {
            setDockerOpMessage("Contêiner a reiniciar…");
            continue;
          }
          if (fresh.running) return fresh;
        }
        if (wantStopped && !fresh.running && !fresh.restarting) return fresh;
      } catch {
        /* continua a tentar */
      }
    }
    return null;
  }

  async function withContainerAction(c, { title, confirmMsg, confirmOpts, runApi, poll }) {
    const id = containerId(c);
    if (dockerState.pendingOps.has(id)) {
      CWUI.toast(tr("docker.toast.busy"), "warn");
      return;
    }
    if (confirmMsg && !(await CWConfirm(confirmMsg, confirmOpts || {}))) return;
    const detail = c.displayName || c.name || id;
    setPendingOp(id, { title });
    showDockerOp(title, "A enviar comando ao host remoto…", detail);
    renderContainers();
    try {
      await runApi();
      setDockerOpMessage("Comando enviado. A acompanhar o contêiner…");
      if (poll) await poll();
      else await refreshContainersLight();
      CWUI.toast(tr("docker.toast.done", { title }), "success");
    } catch (e) {
      CWUI.toast(e.message, "error");
    } finally {
      setPendingOp(id, null);
      hideDockerOp();
      loadDocker();
    }
  }

  function stateBadge(c) {
    if (getPendingOp(c)) {
      const op = getPendingOp(c);
      return `<span class="docker-state docker-state-pending" title="${escapeHtml(op?.title || "Operação em curso")}"></span>`;
    }
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
      { label: tr("docker.act.logs"), fn: () => openLogs(c) },
      { label: "Estatísticas", fn: () => openStats(c) },
      { label: "Explorar arquivos", fn: () => openInExplorer(c) },
      { label: "Consola interativa", fn: () => openExec(c), hidden: !(c.running || c.restarting) },
      { divider: true },
    ];
    if (c.composeProject || c.composeService) {
      items.push({ label: tr("docker.act.restartCompose"), fn: () => restartComposeProject(c) });
    }
    items.push({ label: tr("docker.act.autoRule"), fn: () => openAutomationRuleForContainer(c) });
    items.push({ divider: true });
    const st = (c.state || "").toLowerCase();
    if (st === "paused") {
      items.push({ label: tr("docker.act.unpause"), fn: () => lifecycle(c, "unpause") });
      items.push({ label: tr("docker.act.stop"), fn: () => lifecycle(c, "stop") });
    } else if (c.running || c.restarting) {
      items.push({ label: tr("docker.act.restart"), fn: () => restartOne(c, false) });
      if (c.composeProject || c.composeService) {
        items.push({
          label: "Recriar via Compose",
          fn: () => restartOne(c, true),
        });
      }
      items.push({ label: tr("docker.act.pause"), fn: () => lifecycle(c, "pause") });
      items.push({ label: tr("docker.act.stop"), fn: () => lifecycle(c, "stop") });
    } else {
      items.push({ label: tr("docker.act.start"), fn: () => lifecycle(c, "start") });
    }
    items.push({ divider: true });
    items.push({ label: tr("docker.act.removeContainer"), fn: () => removeOne(c), danger: true });
    return items.filter((it) => !it.hidden);
  }

  function createQuickActions(c) {
    const wrap = document.createElement("div");
    wrap.className = "docker-quick-actions";
    wrap.setAttribute("role", "group");
    wrap.setAttribute("aria-label", "Ações rápidas");
    const st = (c.state || "").toLowerCase();
    const running = c.running || c.restarting;
    const paused = st === "paused";
    const addBtn = (title, label, fn, extra = "") => {
      const b = document.createElement("button");
      b.type = "button";
      b.className = `btn btn-ghost btn-sm docker-quick-btn ${extra}`.trim();
      b.title = title;
      b.setAttribute("aria-label", title);
      b.textContent = label;
      b.addEventListener("click", (e) => {
        e.stopPropagation();
        fn();
      });
      wrap.appendChild(b);
    };
    if (paused) {
      addBtn("Retomar", "▶", () => lifecycle(c, "unpause"));
      addBtn("Parar", "■", () => lifecycle(c, "stop"));
    } else if (running) {
      addBtn("Reiniciar", "↻", () => restartOne(c, false));
      addBtn("Parar", "■", () => lifecycle(c, "stop"));
      addBtn("Pausar", "⏸", () => lifecycle(c, "pause"));
    } else {
      addBtn("Iniciar", "▶", () => lifecycle(c, "start"));
    }
    return wrap;
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
    if (lbl) lbl.textContent = tr("docker.batch.count", { n });
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
    if (getPendingOp(c)) el.classList.add("docker-item-pending");
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
      if (
        e.target.closest(
          ".docker-check, .docker-tools-menu, .docker-quick-actions, .panel-menu, .docker-pin-btn, .docker-explore-btn, button, summary, a, input, label"
        )
      )
        return;
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
    foot.appendChild(createQuickActions(c));
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
    foot.appendChild(createQuickActions(c));
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
    foot.appendChild(createQuickActions(c));
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
    const project = (c.composeProject || "").trim();
    const label = project || c.composeService || containerDisplayName(c);
    if (!(await CWConfirm(tr("docker.confirm.restartCompose", { label })))) return;
    const inProject = project
      ? dockerState.containers.filter((x) => (x.composeProject || "").trim() === project)
      : [];
    const targets = inProject.length ? inProject : [c];
    const { errors } = await runRecreateFlow(targets);
    if (errors.length) CWUI.toast(errors[0], "error");
  }

  function openAutomationRuleForContainer(c) {
    const target = (c.name || "").replace(/^\//, "") || c.displayName || c.id;
    sessionStorage.setItem(
      "cw-docker-auto-prefill",
      JSON.stringify({
        target,
        name: tr("auto.rule.dockerName", { target: c.displayName || target }),
      })
    );
    if (typeof showScreen === "function") showScreen("automations");
    else CWUI.toast(tr("docker.toast.openAutomations"), "info");
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
        restartBtn.textContent = tr("docker.act.restartComposeBtn");
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
      let desc = tr("docker.empty.noneRunning");
      if (hasText || hasQuick) desc = tr("docker.empty.adjustFilters");
      else if (dockerState.showAll) desc = tr("docker.empty.noneHost");
      CWUI.emptyState(box, {
        icon: "🐳",
        title: hasText || hasQuick ? tr("docker.empty.noResults") : tr("docker.empty.none"),
        desc,
        actionLabel: tr("common.refresh"),
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
    if (!m) return `<p class="muted">${escapeHtml(tr("docker.metrics.unavailable"))}</p>`;
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
      const quick = createQuickActions(c);
      quick.classList.add("docker-detail-lifecycle");
      headActions.appendChild(quick);
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
          tr("docker.toast.critical", {
            n: summary.critical - dockerState.lastCriticalCount,
          }),
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
    const labelKeys = {
      stop: "docker.act.stop",
      start: "docker.act.start",
      pause: "docker.act.pause",
      unpause: "docker.act.unpause",
      remove: "docker.act.remove",
    };
    const label = tr(labelKeys[action] || action);
    const danger = action === "remove";
    const poll =
      action === "stop"
        ? () => pollContainerState(c, { wantStopped: true })
        : action === "start" || action === "unpause"
          ? () => pollContainerState(c, { wantRunning: true })
          : null;
    await withContainerAction(c, {
      title: label,
      confirmMsg: `${label} ${c.displayName || c.name || id}?`,
      confirmOpts: { danger },
      runApi: () =>
        dockerApi(`/api/docker/${action}`, {
          method: "POST",
          body: JSON.stringify({ id, ...extra }),
        }),
      poll,
    });
    if (action === "remove") {
      dockerState.selected.delete(id);
      clearContainerFocus();
    } else {
      dockerState.selected.delete(id);
    }
  }

  async function restartOne(c, compose = false) {
    const name = containerDisplayName(c);
    const confirmMsg = compose
      ? `Recriar "${name}" via Compose?\n\nRemove e recria o contêiner para aplicar o YAML. Pode demorar vários minutos.`
      : `Recriar ${name}?\n\nRemove e recria o contêiner (compose --force-recreate quando disponível).`;
    if (!(await CWConfirm(confirmMsg))) return;
    const { errors } = await runRecreateFlow([c]);
    if (errors.length) CWUI.toast(errors[0], "error");
  }

  async function removeOne(c) {
    const id = containerId(c);
    const msg = (c.running || c.restarting)
      ? tr("docker.confirm.removeRunning")
      : tr("docker.confirm.removeContainer", { name: c.displayName || c.name || id });
    await withContainerAction(c, {
      title: tr("docker.confirm.removeTitle"),
      confirmMsg: msg,
      confirmOpts: { danger: true },
      runApi: () =>
        dockerApi("/api/docker/remove", {
          method: "POST",
          body: JSON.stringify({ id, force: true }),
        }),
      poll: null,
    });
    dockerState.selected.delete(id);
    clearContainerFocus();
  }

  function showBatchResult(action, okCount, errors) {
    const dlg = $("#docker-batch-result-dialog");
    const title = $("#docker-batch-result-title");
    const summary = $("#docker-batch-result-summary");
    const body = $("#docker-batch-result-body");
    if (!dlg) return;
    const labels = {
      restart: "Recriação",
      stop: "Paragem",
      remove: "Remoção",
      start: "Início",
      pause: "Pausa",
      unpause: "Retoma",
    };
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

  async function batchRecreateContainers(ids) {
    const targets = ids.map((id) => findContainer(id)).filter(Boolean);
    if (!targets.length) {
      CWUI.toast(tr("docker.toast.noSelection"), "warn");
      return;
    }
    const { ok, errors } = await runRecreateFlow(targets);
    dockerState.selected.clear();
    if (errors.length) showBatchResult("restart", ok, errors);
  }

  async function batchAction(action) {
    const ids = [...dockerState.selected];
    if (!ids.length) return;
    const labels = {
      restart: tr("docker.batch.restart"),
      stop: tr("docker.act.stop"),
      remove: tr("docker.act.remove"),
      start: tr("docker.act.start"),
      pause: tr("docker.act.pause"),
      unpause: tr("docker.act.unpause"),
    };
    const confirmMsg =
      action === "restart"
        ? `Recriar ${ids.length} contêiner(es)?\n\nRemove e recria os contêineres (docker compose up --force-recreate) para aplicar alterações no YAML. Projetos Compose são recriados uma vez por projeto. Pode demorar vários minutos.`
        : `${labels[action]} ${ids.length} contêiner(es)?`;
    if (!(await CWConfirm(confirmMsg, { danger: action === "remove" }))) return;

    if (action === "restart") {
      await batchRecreateContainers(ids);
      return;
    }

    const batchErrors = [];
    let okCount = 0;
    showDockerOp(`${labels[action]} em lote`, "A processar…", `${ids.length} contêiner(es)`);
    renderContainers();
    try {
      let step = 0;
      for (const id of ids) {
        const c = findContainer(id);
        if (action === "pause") {
          const st = (c?.state || "").toLowerCase();
          if (!c || st === "paused" || !(c.running || c.restarting)) continue;
        }
        if (action === "unpause" && (c?.state || "").toLowerCase() !== "paused") continue;
        if (action === "start" && c && (c.running || c.restarting)) continue;
        step += 1;
        const name = c?.displayName || c?.name || id;
        setDockerOpStep(`${labels[action]}: ${name} (${step}/${ids.length})`, step, ids.length);
        setDockerOpMessage("A enviar comando ao host…");
        try {
          await dockerApi(`/api/docker/${action}`, {
            method: "POST",
            body: JSON.stringify({ id, force: action === "remove" }),
          });
          okCount++;
        } catch (e) {
          batchErrors.push(`${name}: ${e.message}`);
        }
      }
      dockerState.selected.clear();
      loadDocker();
      if (batchErrors.length) showBatchResult(action, okCount, batchErrors);
      else CWUI.toast(tr("docker.toast.batchOk", { action: labels[action], n: okCount }), "success");
    } catch (e) {
      CWUI.toast(e.message, "error");
    } finally {
      hideDockerOp();
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

  function formatDockerLogTimestamp(text) {
    const m = text.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s+(.*)$/);
    if (!m) return null;
    const d = new Date(m[1]);
    if (Number.isNaN(d.getTime())) return null;
    const local = d.toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "medium", hour12: false });
    return { local, body: m[2] };
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
      const ts = formatDockerLogTimestamp(text);
      if (ts) {
        html.push(
          `<span class="docker-log-line ${cls}"><span class="docker-log-ts" title="UTC ${escapeHtml(text.slice(0, text.indexOf(" ")))}">${escapeHtml(ts.local)}</span> ${highlightSearch(ts.body, q)}</span>`
        );
      } else {
        html.push(`<span class="docker-log-line ${cls}">${highlightSearch(text, q)}</span>`);
      }
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

  function stopLogsPoll() {
    if (dockerState.logsPoll) {
      clearInterval(dockerState.logsPoll);
      dockerState.logsPoll = null;
    }
  }

  function openLogs(c) {
    const dlg = $("#docker-logs-dialog");
    const body = $("#docker-logs-body");
    const title = $("#docker-logs-title");
    if (!dlg || !body) return;
    const fresh = findContainer(containerId(c)) || c;
    dockerState.logsCtx = fresh;
    dockerState.logsRaw = "";
    fresh._startedAt = undefined;
    title.textContent = tr("docker.logs.title", {
      name: fresh.displayName || fresh.name || fresh.id,
    });
    body.textContent = "A carregar…";
    const search = $("#docker-logs-search");
    if (search) search.value = "";
    dlg.showModal();
    loadLogsPanel(true);
    stopLogsPoll();
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
      stopLogsPoll();
      dockerState.logsCtx = null;
    };
  }

  async function loadLogsPanel(forceScroll) {
    let c = dockerState.logsCtx;
    if (!c) return;
    const fresh = findContainer(containerId(c));
    if (!fresh) {
      stopLogsPoll();
      const body = $("#docker-logs-body");
      const status = $("#docker-logs-status");
      if (body) {
        body.innerHTML =
          '<p class="error">Contêiner não encontrado no host (foi removido ou recriado). Feche e atualize a lista Docker.</p>';
      }
      if (status) status.textContent = "Contêiner inexistente — live pausado";
      return;
    }
    c = fresh;
    dockerState.logsCtx = fresh;
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
      const msg = e.message || String(e);
      if (status) status.textContent = msg;
      if (/não encontrado|no such container|404/i.test(msg)) {
        stopLogsPoll();
        const autoBtn = $("#docker-logs-auto");
        if (autoBtn) autoBtn.textContent = "Live pausado";
        if (body) {
          body.innerHTML = `<p class="error">${escapeHtml(msg)}</p><p class="muted">O contêiner pode ter sido recriado com outro ID. Atualize a lista e abra os logs de novo.</p>`;
        }
      }
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
    ws.onopen = () => dockerState.execTerm.writeln("\r\n\x1b[32mConectado ao contêiner.\x1b[0m\r\n");
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
    CWUI.toast(tr("docker.toast.explorerHint"), "info");
  }

  function volumeRefToContainer(ref) {
    if (!ref) return null;
    return {
      id: ref.id,
      idFull: ref.idFull || ref.id,
      displayName: ref.displayName,
      name: ref.displayName || ref.name,
    };
  }

  function volumeExploreControlHtml(vol) {
    const cs = vol.containers || [];
    if (!cs.length) {
      return `<span class="muted docker-vol-explore-empty" title="Nenhum contêiner monta este volume">—</span>`;
    }
    if (cs.length === 1) {
      const c = cs[0];
      const title = escapeHtml(c.displayName || c.id);
      return `<button type="button" class="btn btn-ghost btn-sm docker-vol-explore-btn" data-vol-explore="${escapeHtml(vol.name)}" data-container-id="${escapeHtml(c.idFull || c.id)}" title="Explorar arquivos — ${title}">📁 Arquivos</button>`;
    }
    const items = cs
      .map((c) => {
        const label = escapeHtml(c.displayName || c.id) + (c.running ? "" : " (parado)");
        return `<button type="button" class="btn btn-ghost btn-sm docker-vol-explore-pick" data-vol-explore="${escapeHtml(vol.name)}" data-container-id="${escapeHtml(c.idFull || c.id)}">${label}</button>`;
      })
      .join("");
    return `<details class="panel-menu docker-vol-explore-menu"><summary class="btn btn-ghost btn-sm" title="Escolher contêiner para explorar arquivos">📁 Arquivos</summary><div class="panel-menu-list">${items}</div></details>`;
  }

  function bindVolumeExploreActions(host) {
    host.querySelectorAll("[data-vol-explore]").forEach((btn) => {
      btn.addEventListener("click", (e) => {
        e.stopPropagation();
        const vol = dockerState.volumes.find((v) => v.name === btn.dataset.volExplore);
        if (!vol) return;
        const id = btn.dataset.containerId;
        let ref = vol.containers?.find((c) => (c.idFull || c.id) === id);
        if (!ref) ref = vol.containers?.find((c) => c.running) || vol.containers?.[0];
        const c = volumeRefToContainer(ref);
        if (c) openInExplorer(c);
      });
    });
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
        desc: "Ajuste o filtro ou atualize a lista.",
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
    host.innerHTML = `<div class="docker-resource-table-wrap glass-card"><table class="docker-resource-table docker-resource-table--images">
      <colgroup>
        <col class="col-tag" />
        <col class="col-id" />
        <col class="col-size" />
        <col class="col-created" />
        <col class="col-use" />
        <col class="col-actions" />
      </colgroup>
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
    if (!(await CWConfirm(tr("docker.confirm.removeImage", { label: label || id }), { danger: true }))) return;
    const img = dockerState.images.find((i) => (i.idFull || i.id) === id);
    let useForce = false;
    if (img && img.containers > 0) {
      if (!(await CWConfirm(tr("docker.confirm.imageInUse"), { danger: true }))) return;
      useForce = true;
    }
    try {
      await dockerApi("/api/docker/images/remove", {
        method: "POST",
        body: JSON.stringify({ id, force: useForce }),
      });
      CWUI.toast(tr("docker.toast.imageRemoved"), "success");
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
          <td class="mono muted docker-resource-mount" title="${escapeHtml(v.mountpoint || "—")}">${escapeHtml(v.mountpoint || "—")}</td>
          <td class="muted">${escapeHtml(v.createdLabel || "—")}</td>
          <td class="docker-resource-actions">
            ${volumeExploreControlHtml(v)}
            <button type="button" class="btn btn-ghost btn-sm" data-copy-vol="${escapeHtml(v.name)}">Copiar nome</button>
            <button type="button" class="btn btn-ghost btn-sm btn-danger" data-remove-vol="${escapeHtml(v.name)}">Remover</button>
          </td>
        </tr>`
      )
      .join("");
    host.innerHTML = `<div class="docker-resource-table-wrap glass-card"><table class="docker-resource-table docker-resource-table--volumes">
      <colgroup>
        <col class="col-name" />
        <col class="col-driver" />
        <col class="col-mount" />
        <col class="col-created" />
        <col class="col-actions" />
      </colgroup>
      <thead><tr><th>Nome</th><th>Driver</th><th>Mountpoint</th><th>Criado</th><th></th></tr></thead>
      <tbody>${rows}</tbody></table></div>`;
    host.querySelectorAll("[data-copy-vol]").forEach((btn) => {
      btn.addEventListener("click", () => copyText(btn.dataset.copyVol, "Nome"));
    });
    host.querySelectorAll("[data-remove-vol]").forEach((btn) => {
      btn.addEventListener("click", () => removeDockerVolume(btn.dataset.removeVol));
    });
    bindVolumeExploreActions(host);
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
    if (!(await CWConfirm(tr("docker.confirm.removeVolume", { name }), { danger: true }))) return;
    try {
      await dockerApi("/api/docker/volumes/remove", {
        method: "POST",
        body: JSON.stringify({ name, force: false }),
      });
      CWUI.toast(tr("docker.toast.volumeRemoved"), "success");
      loadDockerVolumes();
    } catch (e) {
      if (/in use|being used/i.test(e.message) && (await CWConfirm(tr("docker.confirm.volumeInUse"), { danger: true }))) {
        try {
          await dockerApi("/api/docker/volumes/remove", {
            method: "POST",
            body: JSON.stringify({ name, force: true }),
          });
          CWUI.toast(tr("docker.toast.volumeRemoved"), "success");
          loadDockerVolumes();
        } catch (e2) {
          CWUI.toast(e2.message, "error");
        }
      } else {
        CWUI.toast(e.message, "error");
      }
    }
  }

  function renderSystemBar() {
    const bar = $("#docker-system-bar");
    if (!bar) return;
    const u = dockerState.systemUsage;
    if (!u) {
      bar.classList.add("hidden");
      bar.innerHTML = "";
      return;
    }
    bar.classList.remove("hidden");
    const chips = [
      `<span class="docker-system-chip" title="Imagens no host">📦 ${u.imagesCount} imagens · ${escapeHtml(u.imagesSizeHuman || "—")}</span>`,
      u.imagesDangling
        ? `<span class="docker-system-chip docker-system-chip-warn" title="Imagens dangling recuperáveis">⚠ ${u.imagesDangling} dangling · ${escapeHtml(u.imagesDanglingHuman || "—")}</span>`
        : "",
      `<span class="docker-system-chip" title="Contêineres">🧱 ${u.containersCount} contêineres · ${escapeHtml(u.containersSizeHuman || "—")}</span>`,
      `<span class="docker-system-chip" title="Volumes">💾 ${u.volumesCount} volumes · ${escapeHtml(u.volumesSizeHuman || "—")}</span>`,
      u.buildCacheSize > 0
        ? `<span class="docker-system-chip muted" title="Cache de build">🔧 cache ${escapeHtml(u.buildCacheSizeHuman || "—")}</span>`
        : "",
    ].filter(Boolean);
    const cacheBtn =
      u.buildCacheSize > 0
        ? `<button type="button" class="btn btn-ghost btn-sm docker-system-prune-btn" id="docker-prune-cache" title="Limpar cache de build">Limpar cache</button>`
        : "";
    bar.innerHTML = chips.join("") + cacheBtn;
    $("#docker-prune-cache")?.addEventListener("click", pruneBuildCache);
  }

  async function loadDockerSystem() {
    const bar = $("#docker-system-bar");
    if (!bar || !state.ssh?.connected) {
      dockerState.systemUsage = null;
      renderSystemBar();
      return;
    }
    try {
      const data = await dockerApi("/api/docker/system");
      dockerState.systemUsage = data.usage || null;
      renderSystemBar();
    } catch {
      dockerState.systemUsage = null;
      renderSystemBar();
    }
  }

  async function pruneDockerImages() {
    if (!(await CWConfirm(tr("docker.confirm.pruneDangling"), { danger: true }))) return;
    try {
      const res = await dockerApi("/api/docker/images/prune", {
        method: "POST",
        body: JSON.stringify({ danglingOnly: true }),
      });
      CWUI.toast(tr("docker.toast.freed", { space: res.spaceReclaimedHuman || "—" }), "success");
      loadDockerSystem();
      loadDockerImages();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  async function pruneDockerVolumes() {
    if (!(await CWConfirm(tr("docker.confirm.pruneVolumes"), { danger: true }))) return;
    try {
      const res = await dockerApi("/api/docker/volumes/prune", { method: "POST", body: "{}" });
      CWUI.toast(tr("docker.toast.freed", { space: res.spaceReclaimedHuman || "—" }), "success");
      loadDockerSystem();
      loadDockerVolumes();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  function matchNetworksFilter(net, q) {
    q = (q || "").trim().toLowerCase();
    if (!q) return true;
    const hay = [net.name, net.driver, net.scope, net.id].join(" ").toLowerCase();
    return hay.includes(q);
  }

  function renderNetworks() {
    const host = $("#docker-networks-list");
    if (!host) return;
    const filtered = dockerState.networks.filter((n) => matchNetworksFilter(n, dockerState.networksFilter));
    if (!filtered.length) {
      CWUI.emptyState(host, {
        icon: "🌐",
        title: dockerState.networksFilter ? "Nenhum resultado" : "Nenhuma rede",
        desc: "Não há redes neste host.",
        actionLabel: "Atualizar",
        onAction: loadDockerNetworks,
      });
      return;
    }
    const rows = filtered
      .map(
        (n) => `<tr>
          <td class="docker-resource-name" title="${escapeHtml(n.name)}">${escapeHtml(n.name)}</td>
          <td class="mono muted">${escapeHtml(n.id)}</td>
          <td>${escapeHtml(n.driver || "—")}</td>
          <td class="muted">${escapeHtml(n.scope || "—")}</td>
          <td>${n.containers}</td>
          <td class="muted">${n.internal ? "sim" : "—"}</td>
          <td class="docker-resource-actions">
            <button type="button" class="btn btn-ghost btn-sm" data-copy-net="${escapeHtml(n.name)}">Copiar nome</button>
            ${
              n.protected
                ? ""
                : `<button type="button" class="btn btn-ghost btn-sm btn-danger" data-remove-net="${escapeHtml(n.name)}" data-net-label="${escapeHtml(n.name)}">Remover</button>`
            }
          </td>
        </tr>`
      )
      .join("");
    host.innerHTML = `<div class="docker-resource-table-wrap glass-card"><table class="docker-resource-table docker-resource-table--networks">
      <colgroup>
        <col class="col-name" />
        <col class="col-id" />
        <col class="col-driver" />
        <col class="col-scope" />
        <col class="col-use" />
        <col class="col-internal" />
        <col class="col-actions" />
      </colgroup>
      <thead><tr><th>Nome</th><th>ID</th><th>Driver</th><th>Scope</th><th>Contêineres</th><th>Interna</th><th></th></tr></thead>
      <tbody>${rows}</tbody></table></div>`;
    host.querySelectorAll("[data-copy-net]").forEach((btn) => {
      btn.addEventListener("click", () => copyText(btn.dataset.copyNet, "Nome da rede"));
    });
    host.querySelectorAll("[data-remove-net]").forEach((btn) => {
      btn.addEventListener("click", () => removeDockerNetwork(btn.dataset.removeNet, btn.dataset.netLabel));
    });
  }

  async function loadDockerNetworks() {
    const host = $("#docker-networks-list");
    const status = $("#docker-networks-status");
    if (!host) return;
    CWUI.skeletonList(host, 6);
    try {
      const data = await dockerApi("/api/docker/networks");
      dockerState.networks = data.networks || [];
      if (status) {
        status.textContent = `${data.count ?? dockerState.networks.length} redes · ${new Date(data.updatedAt || Date.now()).toLocaleTimeString()}`;
      }
      renderNetworks();
    } catch (e) {
      host.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
    }
  }

  async function pruneBuildCache() {
    if (!(await CWConfirm(tr("docker.confirm.pruneBuildCache"), { danger: true }))) return;
    try {
      const res = await dockerApi("/api/docker/buildcache/prune", { method: "POST", body: "{}" });
      CWUI.toast(`Cache libertado: ${res.spaceReclaimedHuman || "—"}`, "success");
      loadDockerSystem();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  function populateCreateDatalists() {
    const imgList = $("#docker-create-image-list");
    if (imgList) {
      imgList.innerHTML = (dockerState.images || [])
        .filter((i) => !i.dangling && i.display)
        .slice(0, 80)
        .map((i) => `<option value="${escapeHtml(i.tags?.[0] || i.display)}"></option>`)
        .join("");
    }
    const netList = $("#docker-create-network-list");
    if (netList) {
      netList.innerHTML = (dockerState.networks || [])
        .map((n) => `<option value="${escapeHtml(n.name)}"></option>`)
        .join("");
    }
  }

  async function openCreateContainerDialog() {
    const dlg = $("#docker-create-dialog");
    const err = $("#docker-create-error");
    if (!dlg) return;
    if (!dockerState.images.length) {
      try {
        const data = await dockerApi("/api/docker/images");
        dockerState.images = data.images || [];
      } catch {
        /* ignore */
      }
    }
    if (!dockerState.networks.length) {
      try {
        const data = await dockerApi("/api/docker/networks");
        dockerState.networks = data.networks || [];
      } catch {
        /* ignore */
      }
    }
    populateCreateDatalists();
    $("#docker-create-image").value = "";
    $("#docker-create-name").value = "";
    $("#docker-create-cmd").value = "";
    $("#docker-create-env").value = "";
    $("#docker-create-ports").value = "";
    $("#docker-create-restart").value = "unless-stopped";
    $("#docker-create-network").value = "";
    if (err) {
      err.textContent = "";
      err.classList.add("hidden");
    }
    dlg.showModal();
    CWUI.enhanceSelects?.(dlg);
    setTimeout(() => $("#docker-create-image")?.focus(), 50);
  }

  async function submitCreateContainer(e) {
    e?.preventDefault();
    const err = $("#docker-create-error");
    const image = $("#docker-create-image")?.value?.trim();
    if (!image) {
      if (err) {
        err.textContent = "Indique a imagem.";
        err.classList.remove("hidden");
      }
      return;
    }
    const body = {
      image,
      name: $("#docker-create-name")?.value?.trim() || "",
      cmd: $("#docker-create-cmd")?.value?.trim() || "",
      env: $("#docker-create-env")?.value || "",
      ports: $("#docker-create-ports")?.value?.trim() || "",
      restart: $("#docker-create-restart")?.value || "no",
      network: $("#docker-create-network")?.value?.trim() || "",
    };
    const submit = $("#docker-create-submit");
    if (submit) submit.disabled = true;
    try {
      await dockerApi("/api/docker/containers/create", { method: "POST", body: JSON.stringify(body) });
      CWUI.toast(tr("docker.toast.containerCreated"), "success");
      $("#docker-create-dialog")?.close();
      loadDocker();
    } catch (ex) {
      if (err) {
        err.textContent = ex.message;
        err.classList.remove("hidden");
      } else {
        CWUI.toast(ex.message, "error");
      }
    } finally {
      if (submit) submit.disabled = false;
    }
  }

  function openCreateNetworkDialog() {
    const dlg = $("#docker-network-create-dialog");
    if (!dlg) return;
    $("#docker-network-create-name").value = "";
    $("#docker-network-create-driver").value = "bridge";
    $("#docker-network-create-internal").checked = false;
    const err = $("#docker-network-create-error");
    if (err) {
      err.textContent = "";
      err.classList.add("hidden");
    }
    dlg.showModal();
    setTimeout(() => $("#docker-network-create-name")?.focus(), 50);
  }

  async function submitCreateNetwork(e) {
    e?.preventDefault();
    const err = $("#docker-network-create-error");
    const name = $("#docker-network-create-name")?.value?.trim();
    if (!name) {
      if (err) {
        err.textContent = "Indique o nome.";
        err.classList.remove("hidden");
      }
      return;
    }
    try {
      await dockerApi("/api/docker/networks/create", {
        method: "POST",
        body: JSON.stringify({
          name,
          driver: $("#docker-network-create-driver")?.value || "bridge",
          internal: $("#docker-network-create-internal")?.checked || false,
        }),
      });
      CWUI.toast(tr("docker.toast.networkCreated"), "success");
      $("#docker-network-create-dialog")?.close();
      loadDockerNetworks();
      loadDockerSystem();
    } catch (ex) {
      if (err) {
        err.textContent = ex.message;
        err.classList.remove("hidden");
      } else {
        CWUI.toast(ex.message, "error");
      }
    }
  }

  function openCreateVolumeDialog() {
    const dlg = $("#docker-volume-create-dialog");
    if (!dlg) return;
    $("#docker-volume-create-name").value = "";
    $("#docker-volume-create-driver").value = "local";
    const err = $("#docker-volume-create-error");
    if (err) {
      err.textContent = "";
      err.classList.add("hidden");
    }
    dlg.showModal();
    setTimeout(() => $("#docker-volume-create-name")?.focus(), 50);
  }

  async function submitCreateVolume(e) {
    e?.preventDefault();
    const err = $("#docker-volume-create-error");
    const name = $("#docker-volume-create-name")?.value?.trim();
    if (!name) {
      if (err) {
        err.textContent = "Indique o nome.";
        err.classList.remove("hidden");
      }
      return;
    }
    try {
      await dockerApi("/api/docker/volumes/create", {
        method: "POST",
        body: JSON.stringify({
          name,
          driver: $("#docker-volume-create-driver")?.value?.trim() || "local",
        }),
      });
      CWUI.toast(tr("docker.toast.volumeCreated"), "success");
      $("#docker-volume-create-dialog")?.close();
      loadDockerVolumes();
      loadDockerSystem();
    } catch (ex) {
      if (err) {
        err.textContent = ex.message;
        err.classList.remove("hidden");
      } else {
        CWUI.toast(ex.message, "error");
      }
    }
  }

  async function removeDockerNetwork(name, label) {
    if (!(await CWConfirm(tr("docker.confirm.removeNetwork", { label: label || name }), { danger: true }))) return;
    try {
      await dockerApi("/api/docker/networks/remove", {
        method: "POST",
        body: JSON.stringify({ name }),
      });
      CWUI.toast(tr("docker.toast.networkRemoved"), "success");
      loadDockerNetworks();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  function startAutoPoll() {
    stopAutoPoll();
    dockerState.pollTimer = setInterval(() => {
      if (dockerState.pendingOps.size > 0) return;
      if (state.screen === "docker" && dockerState.autoRefresh && state.ssh.connected) {
        loadDockerSystem();
        if (dockerState.moduleTab === "containers") loadDockerContainers();
        else if (dockerState.moduleTab === "images") loadDockerImages();
        else if (dockerState.moduleTab === "volumes") loadDockerVolumes();
        else if (dockerState.moduleTab === "networks") loadDockerNetworks();
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
    $("#docker-volumes-prune")?.addEventListener("click", pruneDockerVolumes);
    $("#docker-images-prune")?.addEventListener("click", pruneDockerImages);
    $("#docker-networks-refresh")?.addEventListener("click", loadDockerNetworks);
    $("#docker-networks-filter")?.addEventListener("input", (e) => {
      dockerState.networksFilter = e.target.value;
      renderNetworks();
    });
    applyViewClasses();
    $$(".docker-view-toggle .view-toggle-btn").forEach((btn) => {
      btn.addEventListener("click", () => setViewMode(btn.dataset.view));
    });
    $("#docker-quick-filter")?.addEventListener("change", (e) => {
      setQuickFilter(e.target.value || "all");
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
    $("#docker-batch-pause")?.addEventListener("click", () => batchAction("pause"));
    $("#docker-batch-unpause")?.addEventListener("click", () => batchAction("unpause"));
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
        CWUI.toast(tr("common.copyFail"), "error");
      }
    });
    $("#docker-stats-refresh")?.addEventListener("click", loadStatsPanel);
    $("#docker-stats-close")?.addEventListener("click", () => $("#docker-stats-dialog")?.close());
    $("#docker-batch-result-close")?.addEventListener("click", () => $("#docker-batch-result-dialog")?.close());
    $("#docker-recreate-progress-close")?.addEventListener("click", () => closeRecreateProgressUI());
    $("#docker-recreate-progress-dialog")?.addEventListener("cancel", (e) => {
      if (recreateProgressState?.running) e.preventDefault();
    });
    $("#docker-inspect-close")?.addEventListener("click", () => $("#docker-inspect-dialog")?.close());
    $("#docker-exec-close")?.addEventListener("click", () => {
      $("#docker-exec-dialog")?.close();
      closeExec();
    });
    $("#docker-op-dialog")?.addEventListener("cancel", (e) => {
      if (dockerState.pendingOps.size > 0) e.preventDefault();
    });
    $("#docker-create-container")?.addEventListener("click", openCreateContainerDialog);
    $("#docker-create-form")?.addEventListener("submit", submitCreateContainer);
    $("#docker-create-cancel")?.addEventListener("click", () => $("#docker-create-dialog")?.close());
    $("#docker-open-network-create")?.addEventListener("click", openCreateNetworkDialog);
    $("#docker-network-create-form")?.addEventListener("submit", submitCreateNetwork);
    $("#docker-network-create-cancel")?.addEventListener("click", () => $("#docker-network-create-dialog")?.close());
    $("#docker-create-volume")?.addEventListener("click", openCreateVolumeDialog);
    $("#docker-volume-create-form")?.addEventListener("submit", submitCreateVolume);
    $("#docker-volume-create-cancel")?.addEventListener("click", () => $("#docker-volume-create-dialog")?.close());

  }

  window.dockerRestartPoll = function dockerRestartPoll() {
    if (dockerState.autoRefresh) startAutoPoll();
  };

  window.loadDocker = loadDocker;
  window.dockerOnScreenEnter = function () {
    syncPrefsToUI();
    syncDockerModuleTabs();
    loadDockerSystem();
    if (dockerState.moduleTab === "containers") applyDockerHash();
    else refreshDockerModule();
  };
  window.initDockerManager = function () {
    bindDockerUI();
    syncGroupControlsVisibility();
    startAutoPoll();
  };

  document.addEventListener("cw-lang-change", () => {
    if (!document.querySelector("#view-docker.active")) return;
    window.CWI18n?.applyLabels?.();
    renderContainers();
    if (dockerState.moduleTab === "images") renderImages();
    else if (dockerState.moduleTab === "volumes") renderVolumes();
    else if (dockerState.moduleTab === "networks") renderNetworks();
  });

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => window.initDockerManager?.());
  } else {
    window.initDockerManager?.();
  }
})();
