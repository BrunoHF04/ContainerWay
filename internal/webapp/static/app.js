const state = {
  user: null,
  ssh: { connected: false, host: "", user: "", isAdmin: false },
  screen: "connect",
  localPath: "",
  remotePath: "/",
  selLocal: null,
  selRemote: null,
  term: null,
  termFit: null,
  termSocket: null,
  autoRules: [],
  autoRulesDirty: false,
  autoEditIndex: -1,
  autoPollTimer: null,
};

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => document.querySelectorAll(sel);

const THEME_KEY = "cw-web-theme";

function initTheme() {
  const saved = localStorage.getItem(THEME_KEY);
  const theme = saved === "light" || saved === "dark"
    ? saved
    : (window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark");
  document.documentElement.setAttribute("data-theme", theme);
}

function toggleTheme() {
  const root = document.documentElement;
  const next = root.getAttribute("data-theme") === "light" ? "dark" : "light";
  const veil = $("#theme-veil");
  const apply = () => {
    root.setAttribute("data-theme", next);
    localStorage.setItem(THEME_KEY, next);
  };
  const finish = () => {
    window.setTimeout(() => {
      root.classList.remove("theme-transition");
      veil?.classList.remove("theme-veil-active");
    }, 1250);
  };
  root.classList.add("theme-transition");
  veil?.classList.add("theme-veil-active");
  if (typeof document.startViewTransition === "function") {
    const vt = document.startViewTransition(apply);
    if (vt?.finished) vt.finished.then(finish).catch(finish);
    else finish();
    return;
  }
  apply();
  finish();
}

initTheme();

function bindThemeButtons() {
  for (const id of ["btn-theme", "btn-theme-login"]) {
    const el = $(`#${id}`);
    if (el) el.addEventListener("click", toggleTheme);
  }
}
bindThemeButtons();

const MODULES = [
  { id: "explorer", icon: "📁", accent: "cyan", title: "Gerenciador de arquivos", desc: "Painel duplo local/remoto, enviar e receber ficheiros.", kw: "arquivos sftp transferência" },
  { id: "docker", icon: "🐳", accent: "indigo", title: "Contêineres Docker", desc: "Lista e reinício de contêineres em execução.", kw: "docker container" },
  { id: "disks", icon: "💾", accent: "emerald", title: "Discos e armazenamento", desc: "lsblk e uso por df no host.", kw: "disco lsblk armazenamento" },
  { id: "terminal", icon: "⌨️", accent: "amber", title: "Terminal SSH", desc: "Consola remota interativa.", kw: "terminal ssh shell" },
  { id: "automations", icon: "⚙️", accent: "violet", title: "Central de automações", desc: "Regras e histórico por host.", kw: "automação regras" },
  { id: "settings", icon: "🔧", accent: "rose", title: "Configurações", desc: "Conta, utilizadores e SMTP.", kw: "configurações admin", adminOnly: true },
];

async function api(path, options = {}) {
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText || "Erro");
  return data;
}

function showView(id) {
  $$(".view").forEach((v) => v.classList.remove("active"));
  $(id).classList.add("active");
}

function showScreen(name) {
  state.screen = name;
  $$(".subview").forEach((v) => {
    v.classList.toggle("active", v.id === `view-${name}`);
  });
  const hubBtn = $("#btn-hub");
  if (hubBtn) hubBtn.hidden = name === "connect" || name === "hub";
  if (name === "explorer") refreshTransferStatus();
  if (name === "docker") loadDocker();
  if (name === "disks") loadDisks();
  if (name === "automations") {
    loadAutomations();
    startAutoPoll();
  } else {
    stopAutoPoll();
  }
  if (name === "terminal") openTerminal();
  if (name === "settings") loadSettings();
  if (name === "explorer") {
    loadLocal(state.localPath);
    if (state.ssh.connected) loadRemote(state.remotePath);
  }
}

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
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(i ? 1 : 0)} ${u[i]}`;
}

const motionReduced = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

/** bindHubCardTilt inclinação 3D — eventos no botão, transform no wrapper. */
function bindHubCardTilt(shell) {
  if (motionReduced()) return;
  const card = shell.querySelector(".hub-card");
  if (!card) return;
  const maxDeg = 14;

  const reset = () => {
    shell.classList.remove("is-tilt");
    shell.style.transform = "";
    shell.style.removeProperty("--mx");
    shell.style.removeProperty("--my");
  };

  const onMove = (e) => {
    const r = shell.getBoundingClientRect();
    const px = (e.clientX - r.left) / r.width - 0.5;
    const py = (e.clientY - r.top) / r.height - 0.5;
    const rotY = px * maxDeg * 2;
    const rotX = -py * maxDeg * 2;
    shell.classList.add("is-tilt");
    shell.style.setProperty("--mx", `${(px + 0.5) * 100}%`);
    shell.style.setProperty("--my", `${(py + 0.5) * 100}%`);
    shell.style.transform =
      `rotateX(${rotX.toFixed(2)}deg) rotateY(${rotY.toFixed(2)}deg) translateZ(12px)`;
  };

  card.addEventListener("mousemove", onMove);
  card.addEventListener("mouseleave", reset);
  card.addEventListener("blur", reset);
}

function renderHub() {
  const q = ($("#hub-search").value || "").toLowerCase();
  const grid = $("#hub-grid");
  grid.innerHTML = "";
  const host = state.ssh.host || "—";
  $("#hub-connected").textContent = `${state.ssh.user}@${host}`;
  let i = 0;
  const isAdmin = state.user?.isAdmin || state.ssh.isAdmin;
  for (const m of MODULES) {
    if (m.adminOnly && !isAdmin) continue;
    const hay = `${m.title} ${m.desc} ${m.kw}`.toLowerCase();
    if (q && !hay.includes(q)) continue;
    const shell = document.createElement("div");
    shell.className = "hub-card-3d";
    shell.dataset.accent = m.accent || "cyan";
    shell.style.setProperty("--delay", `${i * 70}ms`);
    i += 1;

    const card = document.createElement("button");
    card.type = "button";
    card.className = "hub-card";
    card.innerHTML = `<span class="hub-icon-wrap">${m.icon}</span><strong>${escapeHtml(m.title)}</strong><span class="hub-desc">${escapeHtml(m.desc)}</span>`;
    card.addEventListener("click", () => showScreen(m.id));

    shell.appendChild(card);
    shell.addEventListener("animationend", () => shell.classList.add("hub-card-entered"), { once: true });
    bindHubCardTilt(shell);
    grid.appendChild(shell);
  }
  if (!grid.children.length) {
    grid.innerHTML = '<p class="placeholder">Nenhum módulo encontrado.</p>';
  }
}

async function refreshSSH() {
  const st = await api("/api/ssh/status");
  state.ssh.connected = !!st.connected;
  state.ssh.host = st.host || "";
  state.ssh.user = st.user || "";
  state.ssh.isAdmin = !!st.isAdmin;
  const badge = $("#ssh-status");
  badge.textContent = st.connected ? "SSH: online" : "SSH: offline";
  badge.classList.toggle("online", st.connected);
  $("#btn-disconnect").hidden = !st.connected;
  $("#session-host").textContent = st.connected ? `${st.user}@${st.host}` : "";
  if (st.connected) {
    CWUI.hostAccent(st.host);
    if (typeof measureSSHLatency === "function") measureSSHLatency();
    if (typeof refreshSudoUI === "function") refreshSudoUI();
  } else {
    if (typeof refreshSudoUI === "function") refreshSudoUI();
    CWUI.hostAccent("");
    $("#ssh-latency")?.classList.add("hidden");
  }
  if (st.connected) {
    showView("#view-shell");
    showScreen("hub");
    renderHub();
  } else {
    showView("#view-shell");
    showScreen("connect");
  }
}

async function checkSession() {
  try {
    const me = await api("/api/auth/me");
    state.user = me;
    state.ssh.isAdmin = !!me.isAdmin;
    $("#user-label").textContent = me.displayName || me.username;
    showView("#view-shell");
    await refreshConnections();
    await refreshSSH();
  } catch {
    showView("#view-login");
  }
}

$("#login-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const err = $("#login-error");
  err.classList.add("hidden");
  try {
    const me = await api("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username: $("#login-user").value, password: $("#login-pass").value }),
    });
    state.user = me;
    state.ssh.isAdmin = !!me.isAdmin;
    $("#user-label").textContent = me.displayName || me.username;
    showView("#view-shell");
    await refreshConnections();
    await refreshSSH();
  } catch (e) {
    err.textContent = e.message;
    err.classList.remove("hidden");
  }
});

$("#btn-logout").addEventListener("click", async () => {
  closeTerminal();
  await api("/api/auth/logout", { method: "POST", body: "{}" });
  state.user = null;
  state.ssh.connected = false;
  showView("#view-login");
});

$("#btn-disconnect").addEventListener("click", async () => {
  closeTerminal();
  await api("/api/ssh/disconnect", { method: "POST", body: "{}" });
  await refreshSSH();
});

$("#btn-hub").addEventListener("click", () => {
  closeTerminal();
  showScreen("hub");
  renderHub();
});

$("#ssh-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  CWUI.showSplash("A estabelecer ligação SSH…");
  try {
    await api("/api/ssh/connect", {
      method: "POST",
      body: JSON.stringify({
        profileName: $("#ssh-profile").value || $("#ssh-name").value.trim(),
        host: $("#ssh-host").value,
        user: $("#ssh-user").value,
        password: $("#ssh-pass").value,
      }),
    });
    await refreshSSH();
    CWUI.toast("Ligação SSH estabelecida", "success");
  } catch (e) {
    CWUI.toast("Falha SSH: " + e.message, "error");
  } finally {
    CWUI.hideSplash();
  }
});

async function refreshConnections(selectName) {
  const data = await api("/api/connections");
  const sel = $("#ssh-profile");
  const prev = selectName || sel.value;
  sel.innerHTML = '<option value="">— novo / manual —</option>';
  for (const c of data.connections || []) {
    const opt = document.createElement("option");
    opt.value = c.name;
    opt.textContent = `${c.name} (${c.host})`;
    sel.appendChild(opt);
  }
  if (prev) sel.value = prev;
}

async function loadSelectedProfile() {
  const name = $("#ssh-profile").value;
  if (!name) {
    $("#ssh-name").value = "";
    $("#ssh-host").value = "";
    $("#ssh-user").value = "";
    $("#ssh-pass").value = "";
    $("#ssh-conn-status").textContent = "";
    return;
  }
  try {
    const data = await api("/api/connections?name=" + encodeURIComponent(name));
    const p = data.profile;
    if (!p) return;
    $("#ssh-name").value = p.name || name;
    $("#ssh-host").value = p.host || "";
    $("#ssh-user").value = p.user || "";
    $("#ssh-pass").value = p.password || "";
    $("#ssh-save-secrets").checked = !!(p.password || p.hasPassword);
    $("#ssh-conn-status").textContent = `Perfil «${p.name}» carregado.`;
  } catch (e) {
    $("#ssh-conn-status").textContent = e.message;
  }
}

$("#ssh-profile").addEventListener("change", loadSelectedProfile);

$("#ssh-save-profile").addEventListener("click", async () => {
  const name = $("#ssh-name").value.trim();
  const host = $("#ssh-host").value.trim();
  const user = $("#ssh-user").value.trim();
  if (!name || !host || !user) {
    CWUI.toast("Preencha nome do perfil, host e utilizador.", "error");
    return;
  }
  try {
    await api("/api/connections", {
      method: "POST",
      body: JSON.stringify({
        name,
        host,
        user,
        password: $("#ssh-pass").value,
        savePassword: $("#ssh-save-secrets").checked,
        insecureHostKey: true,
      }),
    });
    await refreshConnections(name);
    $("#ssh-profile").value = name;
    $("#ssh-conn-status").textContent = `Perfil «${name}» guardado.`;
    CWUI.toast(`Perfil «${name}» guardado`, "success");
  } catch (e) {
    CWUI.toast("Não foi possível guardar: " + e.message, "error");
  }
});

$("#ssh-delete-profile").addEventListener("click", async () => {
  const name = $("#ssh-profile").value || $("#ssh-name").value.trim();
  if (!name) {
    CWUI.toast("Selecione ou indique o nome do perfil a apagar.", "error");
    return;
  }
  if (!(await CWConfirm(`Apagar o perfil «${name}»?`, { danger: true, ok: "Apagar" }))) return;
  try {
    await api("/api/connections?name=" + encodeURIComponent(name), { method: "DELETE" });
    $("#ssh-profile").value = "";
    await refreshConnections();
    await loadSelectedProfile();
    $("#ssh-conn-status").textContent = `Perfil «${name}» apagado.`;
    CWUI.toast(`Perfil «${name}» apagado`, "success");
  } catch (e) {
    CWUI.toast(e.message, "error");
  }
});

$("#hub-search").addEventListener("input", renderHub);

// Explorer — ver explorer.js

async function goUp(side) {
  const path = side === "local" ? state.localPath : state.remotePath;
  const base = side === "local" ? "/api/local/list" : "/api/remote/list";
  const data = await api(`${base}?path=${encodeURIComponent(path)}`);
  const parent = (data.entries || []).find((e) => e.name === "..");
  if (parent) {
    if (side === "local") await loadLocal(parent.path);
    else await loadRemote(parent.path);
  }
}

$$(".icon-btn").forEach((btn) => {
  btn.addEventListener("click", () => {
    const side = btn.dataset.side;
    if (btn.dataset.action === "refresh") {
      if (side === "local") loadLocal(state.localPath);
      else loadRemote(state.remotePath);
    } else if (btn.dataset.action === "up") goUp(side);
  });
});

$("#btn-refresh-panels")?.addEventListener("click", () => {
  if (typeof loadLocal === "function") loadLocal(state.localPath);
  if (state.ssh.connected && typeof loadRemote === "function") loadRemote(state.remotePath);
});

function showTransferLog() {
  $("#transfer-log").classList.remove("hidden");
}

$("#btn-transfer-log-close").addEventListener("click", () => {
  $("#transfer-log").classList.add("hidden");
});

function renderTransferLog(recent) {
  const ul = $("#transfer-log-list");
  ul.innerHTML = "";
  const items = (recent || []).slice().reverse();
  if (!items.length) {
    ul.innerHTML = "<li class='muted'>Sem transferências recentes.</li>";
    return;
  }
  for (const it of items) {
    const li = document.createElement("li");
    li.className = it.status || "";
    const err = it.error ? ` — ${it.error}` : "";
    li.textContent = `${it.name} [${it.status}]${err}`;
    ul.appendChild(li);
  }
}

async function refreshTransferStatus() {
  if (!state.ssh.connected) return;
  try {
    const st = await api("/api/transfer/status");
    $("#transfer-status").textContent = `[fila:${st.queued} exec:${st.running}]`;
    if (st.queued > 0 || st.running > 0 || (st.recent && st.recent.length)) {
      showTransferLog();
    }
    if (st.active || st.running > 0) {
      $("#transfer-progress-panel")?.classList.remove("hidden");
    }
    CWUI.renderTransferProgress(st.active, st.recent);
    renderTransferLog(st.recent);
  } catch { /* ignore */ }
}
setInterval(() => { if (state.screen === "explorer") refreshTransferStatus(); }, 2000);

// Docker
async function loadDocker() {
  const box = $("#docker-list");
  CWUI.skeletonList(box, 5);
  try {
    const data = await api("/api/docker/containers");
    box.innerHTML = "";
    if (!data.containers?.length) {
      CWUI.emptyState(box, {
        icon: "🐳",
        title: "Nenhum contêiner",
        desc: "Não há contêineres em execução neste host.",
        actionLabel: "Atualizar",
        onAction: () => loadDocker(),
      });
      return;
    }
    for (const c of data.containers) {
      const row = document.createElement("div");
      row.className = "data-row";
      row.innerHTML = `<div><strong>${escapeHtml(c.name || c.id)}</strong><span class="muted">${escapeHtml(c.image)} — ${escapeHtml(c.status)}</span></div>`;
      const actions = document.createElement("div");
      actions.className = "data-row-actions";
      const id = c.idFull || c.id;
      for (const [label, fn] of [
        ["Logs", async () => {
          try {
            const data = await api(`/api/docker/logs?id=${encodeURIComponent(id)}&tail=300`);
            await CWConfirm(data.logs || "(vazio)", { ok: "Fechar", title: `Logs — ${c.name || id}` });
          } catch (e) { CWUI.toast(e.message, "error"); }
        }],
        ["Stats", async () => {
          try {
            const data = await api(`/api/docker/stats?id=${encodeURIComponent(id)}`);
            const txt = typeof data.stats === "string" ? data.stats : JSON.stringify(data.stats, null, 2);
            await CWConfirm(txt.slice(0, 4000), { ok: "Fechar", title: `Stats — ${c.name || id}` });
          } catch (e) { CWUI.toast(e.message, "error"); }
        }],
        ["Reiniciar", async () => {
          if (!(await CWConfirm(`Reiniciar ${c.name || c.id}?`))) return;
          await api("/api/docker/restart", { method: "POST", body: JSON.stringify({ id }) });
          CWUI.toast("Contêiner reiniciado", "success");
          loadDocker();
        }],
      ]) {
        const btn = document.createElement("button");
        btn.className = "btn btn-ghost btn-sm";
        btn.textContent = label;
        btn.addEventListener("click", fn);
        actions.appendChild(btn);
      }
      row.appendChild(actions);
      box.appendChild(row);
    }
  } catch (e) {
    box.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
  }
}
$("#docker-refresh").addEventListener("click", loadDocker);

// Disks
async function loadDisks() {
  const wrap = $("#disks-table-wrap");
  wrap.innerHTML = "";
  CWUI.skeletonList(wrap, 4);
  try {
    const data = await api("/api/disks/summary");
    let html = '<table class="data-table"><thead><tr><th>Dispositivo</th><th>Tipo</th><th>Montagem</th><th>Tamanho</th></tr></thead><tbody>';
    for (const d of data.devices || []) {
      const pad = "&nbsp;".repeat((d.depth || 0) * 2);
      html += `<tr><td>${pad}${escapeHtml(d.name)}</td><td>${escapeHtml(d.type)}</td><td>${escapeHtml(d.mount || "—")}</td><td>${escapeHtml(String(d.size))}</td></tr>`;
    }
    html += "</tbody></table>";
    wrap.innerHTML = html;
    $("#disks-df").textContent = data.dfRaw || "";
  } catch (e) {
    wrap.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
  }
}
$("#disks-refresh").addEventListener("click", loadDisks);

// Terminal
function terminalWSUrl() {
  const p = location.protocol === "https:" ? "wss:" : "ws:";
  return `${p}//${location.host}/api/ssh/terminal/ws`;
}

function closeTerminal() {
  if (state.termSocket) {
    state.termSocket.close();
    state.termSocket = null;
  }
  if (state.term) {
    state.term.dispose();
    state.term = null;
    state.termFit = null;
  }
}

function openTerminal() {
  if (!state.ssh.connected) return;
  const box = $("#terminal");
  if (!state.term) {
    state.term = new Terminal({ theme: { background: "#0f1419", foreground: "#e8eef7", cursor: "#3b82f6" }, fontSize: 14 });
    state.termFit = new (window.FitAddon?.FitAddon || FitAddon.FitAddon)();
    state.term.loadAddon(state.termFit);
    state.term.open(box);
  }
  state.termFit.fit();
  if (state.termSocket) state.termSocket.close();
  const ws = new WebSocket(terminalWSUrl());
  state.termSocket = ws;
  ws.onopen = () => state.term.writeln("\r\n\x1b[32mLigado ao host remoto.\x1b[0m\r\n");
  ws.onmessage = (ev) => state.term.write(ev.data);
  ws.onclose = () => state.term.writeln("\r\n\x1b[33mSessão terminada.\x1b[0m\r\n");
  state.term.onData((data) => { if (ws.readyState === WebSocket.OPEN) ws.send(data); });
}
$("#terminal-reconnect").addEventListener("click", openTerminal);

// Automations
function startAutoPoll() {
  stopAutoPoll();
  state.autoPollTimer = setInterval(() => {
    if (state.screen === "automations" && state.ssh.connected) refreshAutoHistory();
  }, 5000);
}

function stopAutoPoll() {
  if (state.autoPollTimer) {
    clearInterval(state.autoPollTimer);
    state.autoPollTimer = null;
  }
}

async function refreshAutoEngine() {
  const st = await api("/api/automations/engine");
  const running = !!st.running;
  const lbl = $("#auto-engine-status");
  const btn = $("#auto-engine-toggle");
  lbl.textContent = running ? "Motor ativo" : "Motor parado";
  lbl.className = running ? "badge on" : "badge muted";
  btn.textContent = running ? "Parar motor" : "Iniciar motor";
}

async function refreshAutoHistory() {
  const hist = await api("/api/automations/history");
  $("#auto-history").textContent = (hist.lines || []).join("\n") || "(histórico vazio)";
}

function renderAutoRules() {
  const rulesBox = $("#auto-rules");
  rulesBox.innerHTML = "";
  const saveBtn = $("#auto-save-rules");
  saveBtn.hidden = !state.autoRulesDirty;
  if (!state.autoRules.length) {
    CWUI.emptyState(rulesBox, {
      icon: "⚙️",
      title: "Sem regras",
      desc: "Configure regras de automação para este host.",
      actionLabel: "Recarregar",
      onAction: () => loadAutomations(),
    });
    return;
  }
  state.autoRules.forEach((r, idx) => {
    const row = document.createElement("div");
    row.className = "data-row";
    const editable = r.kind === "docker_container_stopped_restart";
    const targetHint = editable ? escapeHtml(r.target || "(defina o alvo)") : escapeHtml(r.target || "");
    row.innerHTML = `<div><strong>${escapeHtml(r.name || r.id)}</strong><span class="muted">${escapeHtml(r.trigger || "")} → ${escapeHtml(r.action || "")}</span><br/><span class="muted">${r.enabled ? "Ativa" : "Inativa"} · ${targetHint}</span></div>`;
    const actions = document.createElement("div");
    actions.className = "data-row-actions";
    if (editable) {
      const editBtn = document.createElement("button");
      editBtn.type = "button";
      editBtn.className = "btn btn-ghost btn-sm";
      editBtn.textContent = "Editar";
      editBtn.addEventListener("click", () => openAutoRuleDialog(idx));
      actions.appendChild(editBtn);
    } else {
      const tag = document.createElement("span");
      tag.className = "muted";
      tag.textContent = "Em breve";
      actions.appendChild(tag);
    }
    row.appendChild(actions);
    rulesBox.appendChild(row);
  });
}

function openAutoRuleDialog(idx) {
  const r = state.autoRules[idx];
  if (!r) return;
  state.autoEditIndex = idx;
  const form = $("#auto-rule-form");
  form.elements.name.value = r.name || "";
  form.elements.description.value = r.description || "";
  form.elements.kind.value = r.kind || "";
  form.elements.trigger.value = r.trigger || "";
  form.elements.action.value = r.action || "";
  form.elements.target.value = r.target || "";
  form.elements.cooldownSec.value = r.cooldownSec ?? 20;
  form.elements.webhookURL.value = r.webhookURL || "";
  form.elements.enabled.checked = !!r.enabled;
  $("#auto-dialog-title").textContent = `Editar: ${r.name || r.id}`;
  $("#auto-rule-dialog").showModal();
}

async function loadAutomations() {
  const rulesBox = $("#auto-rules");
  CWUI.skeletonList(rulesBox, 4);
  $("#auto-history").textContent = "…";
  try {
    await refreshAutoEngine();
    const rules = await api("/api/automations/rules");
    state.autoRules = rules.rules || [];
    state.autoRulesDirty = false;
    renderAutoRules();
    await refreshAutoHistory();
  } catch (e) {
    rulesBox.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
  }
}

$("#auto-refresh").addEventListener("click", loadAutomations);

$("#auto-engine-toggle").addEventListener("click", async () => {
  try {
    const st = await api("/api/automations/engine");
    const action = st.running ? "stop" : "start";
    await api("/api/automations/engine", { method: "POST", body: JSON.stringify({ action }) });
    await refreshAutoEngine();
    await refreshAutoHistory();
  } catch (e) {
    CWUI.toast(e.message, "error");
  }
});

$("#auto-save-rules").addEventListener("click", async () => {
  try {
    await api("/api/automations/rules", { method: "PUT", body: JSON.stringify({ rules: state.autoRules }) });
    state.autoRulesDirty = false;
    renderAutoRules();
    CWUI.toast("Regras guardadas", "success");
  } catch (e) {
    CWUI.toast(e.message, "error");
  }
});

$("#auto-clear-history").addEventListener("click", async () => {
  if (!(await CWConfirm("Limpar histórico deste host?"))) return;
  try {
    await api("/api/automations/history", { method: "DELETE" });
    await refreshAutoHistory();
    CWUI.toast("Histórico limpo", "success");
  } catch (e) {
    CWUI.toast(e.message, "error");
  }
});

$("#auto-dialog-cancel").addEventListener("click", () => $("#auto-rule-dialog").close());

$("#auto-rule-form").addEventListener("submit", (ev) => {
  ev.preventDefault();
  const idx = state.autoEditIndex;
  if (idx < 0) return;
  const form = ev.target;
  const r = { ...state.autoRules[idx] };
  r.name = form.elements.name.value.trim();
  r.description = form.elements.description.value.trim();
  r.trigger = form.elements.trigger.value.trim();
  r.action = form.elements.action.value.trim();
  r.target = form.elements.target.value.trim();
  r.cooldownSec = Math.max(10, parseInt(form.elements.cooldownSec.value, 10) || 20);
  r.webhookURL = form.elements.webhookURL.value.trim();
  r.enabled = form.elements.enabled.checked;
  state.autoRules[idx] = r;
  state.autoRulesDirty = true;
  $("#auto-rule-dialog").close();
  renderAutoRules();
});

const settingsState = { users: [], userEditIndex: -1, tab: "account" };

function showSettingsTab(tab) {
  settingsState.tab = tab;
  $$(".settings-tab").forEach((b) => b.classList.toggle("active", b.dataset.tab === tab));
  $$(".settings-panel").forEach((p) => p.classList.remove("active"));
  const panel = $(`#settings-panel-${tab}`);
  if (panel) panel.classList.add("active");
  if (tab === "users") loadSettingsUsers();
  if (tab === "mail") loadSettingsMail();
}

async function loadSettings() {
  $("#settings-user").textContent = state.user?.displayName || state.user?.username || "—";
  const isAdmin = !!state.user?.isAdmin || state.ssh.isAdmin;
  $("#settings-role").textContent = isAdmin ? "Administrador" : "Utilizador";
  const usersTab = $("#settings-tab-users");
  const mailTab = $("#settings-tab-mail");
  if (usersTab) usersTab.hidden = !isAdmin;
  if (mailTab) mailTab.hidden = !isAdmin;
  if (!isAdmin && (settingsState.tab === "users" || settingsState.tab === "mail")) {
    showSettingsTab("account");
  }
  if (isAdmin && settingsState.tab !== "account") {
    if (settingsState.tab === "users") await loadSettingsUsers();
    if (settingsState.tab === "mail") await loadSettingsMail();
  }
}

async function loadSettingsUsers() {
  const box = $("#settings-users-list");
  box.innerHTML = "…";
  try {
    const data = await api("/api/admin/users");
    settingsState.users = (data.users || []).map((u) => ({
      username: u.username,
      displayName: u.displayName || u.username,
      password: "",
      isAdmin: !!u.isAdmin,
    }));
    renderSettingsUsers();
  } catch (e) {
    box.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
  }
}

function renderSettingsUsers() {
  const box = $("#settings-users-list");
  box.innerHTML = "";
  for (const u of settingsState.users) {
    const row = document.createElement("div");
    row.className = "data-row";
    row.innerHTML = `<div><strong>${escapeHtml(u.displayName)}</strong><span class="muted">@${escapeHtml(u.username)}${u.isAdmin ? " · admin" : ""}</span></div>`;
    const actions = document.createElement("div");
    actions.className = "data-row-actions";
    const editBtn = document.createElement("button");
    editBtn.type = "button";
    editBtn.className = "btn btn-ghost btn-sm";
    editBtn.textContent = "Editar";
    editBtn.addEventListener("click", () => {
      settingsState.userEditIndex = settingsState.users.indexOf(u);
      const form = $("#settings-user-form");
      form.elements.username.value = u.username;
      form.elements.username.readOnly = !!u.isAdmin;
      form.elements.displayName.value = u.displayName;
      form.elements.password.value = "";
      form.elements.password.placeholder = u.isAdmin ? "admin: senha reposta ao padrão ao guardar" : "deixe vazio para manter";
      $("#settings-user-dialog").showModal();
    });
    actions.appendChild(editBtn);
    if (!u.isAdmin) {
      const delBtn = document.createElement("button");
      delBtn.type = "button";
      delBtn.className = "btn btn-ghost btn-sm";
      delBtn.textContent = "Remover";
      delBtn.addEventListener("click", () => {
        settingsState.users = settingsState.users.filter((x) => x !== u);
        renderSettingsUsers();
      });
      actions.appendChild(delBtn);
    }
    row.appendChild(actions);
    box.appendChild(row);
  }
}

async function loadSettingsMail() {
  const status = $("#settings-mail-status");
  status.textContent = "A carregar…";
  try {
    const m = await api("/api/admin/mail");
    const form = $("#settings-mail-form");
    form.elements.enabled.checked = !!m.enabled;
    form.elements.host.value = m.host || "";
    form.elements.port.value = m.port || 587;
    form.elements.user.value = m.user || "";
    form.elements.password.value = "";
    form.elements.password.placeholder = m.hasPassword ? "•••• (deixe vazio para manter)" : "senha SMTP";
    form.elements.from.value = m.from || "";
    form.elements.recipientsText.value = (m.recipients || []).join(", ");
    const bits = [];
    if (m.valid) bits.push("configuração completa");
    else if (m.validTransport) bits.push("SMTP OK (faltam destinatários)");
    else bits.push("configuração incompleta");
    status.textContent = bits.join(" · ");
  } catch (e) {
    status.textContent = e.message;
  }
}

$$(".settings-tab").forEach((btn) => {
  btn.addEventListener("click", () => showSettingsTab(btn.dataset.tab));
});

$("#settings-user-add").addEventListener("click", () => {
  settingsState.userEditIndex = -1;
  const form = $("#settings-user-form");
  form.elements.username.value = "";
  form.elements.username.readOnly = false;
  form.elements.displayName.value = "";
  form.elements.password.value = "";
  form.elements.password.placeholder = "obrigatória";
  $("#settings-user-dialog").showModal();
});

$("#settings-user-cancel").addEventListener("click", () => $("#settings-user-dialog").close());

$("#settings-user-form").addEventListener("submit", (ev) => {
  ev.preventDefault();
  const form = ev.target;
  const username = form.elements.username.value.trim().toLowerCase();
  const displayName = form.elements.displayName.value.trim() || username;
  const password = form.elements.password.value;
  if (!username) return;
  const entry = { username, displayName, password, isAdmin: username === "admin" };
  const idx = settingsState.userEditIndex;
  if (idx >= 0) {
    settingsState.users[idx] = { ...settingsState.users[idx], ...entry };
  } else {
    if (!password) {
      CWUI.toast("Senha obrigatória para novo utilizador.", "error");
      return;
    }
    if (settingsState.users.some((u) => u.username === username)) {
      CWUI.toast("Utilizador já existe.", "error");
      return;
    }
    settingsState.users.push(entry);
  }
  $("#settings-user-dialog").close();
  renderSettingsUsers();
});

$("#settings-users-save").addEventListener("click", async () => {
  try {
    const users = settingsState.users.map((u) => ({
      username: u.username,
      displayName: u.displayName,
      password: u.password || "",
    }));
    await api("/api/admin/users", { method: "PUT", body: JSON.stringify({ users }) });
    CWUI.toast("Utilizadores guardados", "success");
    await loadSettingsUsers();
  } catch (e) {
    CWUI.toast(e.message, "error");
  }
});

$("#settings-mail-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const f = ev.target;
  try {
    await api("/api/admin/mail", {
      method: "PUT",
      body: JSON.stringify({
        enabled: f.elements.enabled.checked,
        host: f.elements.host.value.trim(),
        port: parseInt(f.elements.port.value, 10) || 587,
        user: f.elements.user.value.trim(),
        password: f.elements.password.value,
        from: f.elements.from.value.trim(),
        recipientsText: f.elements.recipientsText.value,
      }),
    });
    CWUI.toast("Configuração SMTP guardada", "success");
    await loadSettingsMail();
  } catch (e) {
    CWUI.toast(e.message, "error");
  }
});

async function testSettingsMail(mode) {
  try {
    await api("/api/admin/mail", { method: "POST", body: JSON.stringify({ mode }) });
    CWUI.toast("E-mail de teste enviado", "success");
  } catch (e) {
    CWUI.toast(e.message, "error");
  }
}

$("#settings-mail-test-self").addEventListener("click", () => testSettingsMail("self"));
$("#settings-mail-test-all").addEventListener("click", () => testSettingsMail("recipients"));

checkSession();
