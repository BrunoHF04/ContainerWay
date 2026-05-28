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
};

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => document.querySelectorAll(sel);

const MODULES = [
  { id: "explorer", icon: "📁", title: "Gerenciador de arquivos", desc: "Painel duplo local/remoto, enviar e receber ficheiros.", kw: "arquivos sftp transferência" },
  { id: "docker", icon: "🐳", title: "Contêineres Docker", desc: "Lista e reinício de contêineres em execução.", kw: "docker container" },
  { id: "disks", icon: "💾", title: "Discos e armazenamento", desc: "lsblk e uso por df no host.", kw: "disco lsblk armazenamento" },
  { id: "terminal", icon: "⌨️", title: "Terminal SSH", desc: "Consola remota interativa.", kw: "terminal ssh shell" },
  { id: "automations", icon: "⚙️", title: "Central de automações", desc: "Regras e histórico por host.", kw: "automação regras" },
  { id: "settings", icon: "🔧", title: "Configurações", desc: "Informações da conta (admin).", kw: "configurações admin", adminOnly: true },
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
  $$(".subview").forEach((v) => v.classList.remove("active"));
  const el = $(`#view-${name}`);
  if (el) el.classList.add("active");
  $("#btn-hub").hidden = name === "connect" || name === "hub";
  if (name === "explorer") refreshTransferStatus();
  if (name === "docker") loadDocker();
  if (name === "disks") loadDisks();
  if (name === "automations") loadAutomations();
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

function renderHub() {
  const q = ($("#hub-search").value || "").toLowerCase();
  const grid = $("#hub-grid");
  grid.innerHTML = "";
  const host = state.ssh.host || "—";
  $("#hub-connected").textContent = `Conectado a: ${state.ssh.user}@${host}`;
  for (const m of MODULES) {
    if (m.adminOnly && !state.ssh.isAdmin) continue;
    const hay = `${m.title} ${m.desc} ${m.kw}`.toLowerCase();
    if (q && !hay.includes(q)) continue;
    const card = document.createElement("button");
    card.type = "button";
    card.className = "hub-card";
    card.innerHTML = `<span class="hub-icon">${m.icon}</span><strong>${escapeHtml(m.title)}</strong><span class="hub-desc">${escapeHtml(m.desc)}</span>`;
    card.addEventListener("click", () => showScreen(m.id));
    grid.appendChild(card);
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
  try {
    await api("/api/ssh/connect", {
      method: "POST",
      body: JSON.stringify({
        profileName: $("#ssh-profile").value,
        host: $("#ssh-host").value,
        user: $("#ssh-user").value,
        password: $("#ssh-pass").value,
      }),
    });
    await refreshSSH();
  } catch (e) {
    alert("Falha SSH: " + e.message);
  }
});

async function refreshConnections() {
  const data = await api("/api/connections");
  const sel = $("#ssh-profile");
  sel.innerHTML = '<option value="">— manual —</option>';
  for (const c of data.connections || []) {
    const opt = document.createElement("option");
    opt.value = c.name;
    opt.textContent = `${c.name} (${c.host})`;
    sel.appendChild(opt);
  }
}

$("#hub-search").addEventListener("input", renderHub);

// Explorer
function renderEntries(container, entries, side) {
  container.innerHTML = "";
  if (!entries?.length) {
    container.innerHTML = '<p class="placeholder">Pasta vazia.</p>';
    return;
  }
  for (const e of entries) {
    const row = document.createElement("div");
    row.className = "file-row" + (e.isDir ? " dir" : "");
    row.dataset.path = e.path;
    row.dataset.isDir = e.isDir ? "1" : "0";
    row.innerHTML = `<span class="icon">${e.isDir ? "📁" : "📄"}</span><span class="name">${escapeHtml(e.name)}</span><span class="meta">${e.isDir ? "" : formatSize(e.size)}</span>`;
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
    container.appendChild(row);
  }
}

async function loadLocal(path) {
  const q = path ? `?path=${encodeURIComponent(path)}` : "";
  const data = await api("/api/local/list" + q);
  state.localPath = data.path;
  state.selLocal = null;
  $("#local-path").textContent = data.path;
  renderEntries($("#local-list"), data.entries, "local");
}

async function loadRemote(path) {
  if (!state.ssh.connected) return;
  const data = await api(`/api/remote/list?path=${encodeURIComponent(path || "/")}`);
  state.remotePath = data.path;
  state.selRemote = null;
  $("#remote-path").textContent = data.path;
  renderEntries($("#remote-list"), data.entries, "remote");
}

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

$("#btn-send").addEventListener("click", async () => {
  if (!state.selLocal || state.selLocal.isDir || state.selLocal.name === "..") {
    alert("Selecione um ficheiro no painel local.");
    return;
  }
  const name = state.selLocal.path.split(/[/\\]/).pop();
  const remote = state.remotePath.endsWith("/") ? state.remotePath + name : state.remotePath + "/" + name;
  try {
    await api("/api/transfer/push", {
      method: "POST",
      body: JSON.stringify({ localPath: state.selLocal.path, remotePath: remote }),
    });
    refreshTransferStatus();
  } catch (e) { alert(e.message); }
});

$("#btn-receive").addEventListener("click", async () => {
  if (!state.selRemote || state.selRemote.isDir || state.selRemote.name === "..") {
    alert("Selecione um ficheiro no painel remoto.");
    return;
  }
  const name = state.selRemote.name;
  const local = state.localPath.replace(/[/\\]$/, "") + (state.localPath.includes("\\") ? "\\" : "/") + name;
  try {
    await api("/api/transfer/pull", {
      method: "POST",
      body: JSON.stringify({ localPath: local, remotePath: state.selRemote.path }),
    });
    refreshTransferStatus();
  } catch (e) { alert(e.message); }
});

async function refreshTransferStatus() {
  if (!state.ssh.connected) return;
  try {
    const st = await api("/api/transfer/status");
    $("#transfer-status").textContent = `[fila:${st.queued} exec:${st.running}]`;
  } catch { /* ignore */ }
}
setInterval(() => { if (state.screen === "explorer") refreshTransferStatus(); }, 3000);

// Docker
async function loadDocker() {
  const box = $("#docker-list");
  box.innerHTML = '<p class="placeholder">A carregar…</p>';
  try {
    const data = await api("/api/docker/containers");
    box.innerHTML = "";
    if (!data.containers?.length) {
      box.innerHTML = '<p class="placeholder">Nenhum contêiner em execução.</p>';
      return;
    }
    for (const c of data.containers) {
      const row = document.createElement("div");
      row.className = "data-row";
      row.innerHTML = `<div><strong>${escapeHtml(c.name || c.id)}</strong><span class="muted">${escapeHtml(c.image)} — ${escapeHtml(c.status)}</span></div>`;
      const btn = document.createElement("button");
      btn.className = "btn ghost";
      btn.textContent = "Reiniciar";
      btn.addEventListener("click", async () => {
        if (!confirm(`Reiniciar ${c.name || c.id}?`)) return;
        try {
          await api("/api/docker/restart", { method: "POST", body: JSON.stringify({ id: c.idFull || c.id }) });
          loadDocker();
        } catch (e) { alert(e.message); }
      });
      row.appendChild(btn);
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
  wrap.innerHTML = '<p class="placeholder">A sondar host…</p>';
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
async function loadAutomations() {
  const rulesBox = $("#auto-rules");
  const histBox = $("#auto-history");
  rulesBox.innerHTML = "…";
  histBox.textContent = "…";
  try {
    const rules = await api("/api/automations/rules");
    rulesBox.innerHTML = "";
    if (!rules.rules?.length) {
      rulesBox.innerHTML = '<p class="placeholder">Sem regras para este host.</p>';
    } else {
      for (const r of rules.rules) {
        const row = document.createElement("div");
        row.className = "data-row";
        row.innerHTML = `<div><strong>${escapeHtml(r.name || r.id)}</strong><span class="muted">${escapeHtml(r.trigger)} → ${escapeHtml(r.action)}</span><br/><span class="muted">${r.enabled ? "Ativa" : "Inativa"} · ${escapeHtml(r.target || "")}</span></div>`;
        rulesBox.appendChild(row);
      }
    }
    const hist = await api("/api/automations/history");
    histBox.textContent = (hist.lines || []).join("\n") || "(histórico vazio)";
  } catch (e) {
    rulesBox.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
  }
}

function loadSettings() {
  $("#settings-user").textContent = state.user?.displayName || state.user?.username || "—";
  $("#settings-role").textContent = state.ssh.isAdmin ? "Administrador" : "Utilizador";
}

checkSession();
