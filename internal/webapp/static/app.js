const state = {
  user: null,
  sshConnected: false,
  localPath: "",
  remotePath: "/",
};

const $ = (sel) => document.querySelector(sel);

async function api(path, options = {}) {
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const msg = data.error || res.statusText || "Erro";
    throw new Error(msg);
  }
  return data;
}

function showView(id) {
  document.querySelectorAll(".view").forEach((v) => v.classList.remove("active"));
  $(id).classList.add("active");
}

function formatSize(n) {
  if (n == null || n === 0) return "";
  const u = ["B", "KB", "MB", "GB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i ? 1 : 0)} ${u[i]}`;
}

function renderEntries(container, entries, onOpen) {
  container.innerHTML = "";
  if (!entries || entries.length === 0) {
    container.innerHTML = '<p class="placeholder">Pasta vazia.</p>';
    return;
  }
  for (const e of entries) {
    const row = document.createElement("div");
    row.className = "file-row" + (e.isDir ? " dir" : "");
    row.innerHTML = `
      <span class="icon">${e.isDir ? "📁" : "📄"}</span>
      <span class="name">${escapeHtml(e.name)}</span>
      <span class="meta">${e.isDir ? "" : formatSize(e.size)}</span>
    `;
    row.addEventListener("dblclick", () => onOpen(e));
    container.appendChild(row);
  }
}

function escapeHtml(s) {
  const d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

async function checkSession() {
  try {
    const me = await api("/api/auth/me");
    state.user = me;
    $("#user-label").textContent = me.displayName || me.username;
    showView("#view-app");
    await refreshConnections();
    await refreshSSHStatus();
    await loadLocal("");
  } catch {
    showView("#view-login");
  }
}

$("#login-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const err = $("#login-error");
  err.classList.add("hidden");
  try {
    const body = {
      username: $("#login-user").value,
      password: $("#login-pass").value,
    };
    const me = await api("/api/auth/login", { method: "POST", body: JSON.stringify(body) });
    state.user = me;
    $("#user-label").textContent = me.displayName || me.username;
    showView("#view-app");
    await refreshConnections();
    await refreshSSHStatus();
    await loadLocal("");
  } catch (e) {
    err.textContent = e.message;
    err.classList.remove("hidden");
  }
});

$("#btn-logout").addEventListener("click", async () => {
  await api("/api/auth/logout", { method: "POST", body: "{}" });
  state.user = null;
  state.sshConnected = false;
  showView("#view-login");
});

async function refreshConnections() {
  const data = await api("/api/connections");
  const sel = $("#ssh-profile");
  sel.innerHTML = '<option value="">— perfil —</option>';
  for (const c of data.connections || []) {
    const opt = document.createElement("option");
    opt.value = c.name;
    opt.textContent = `${c.name} (${c.host})`;
    sel.appendChild(opt);
  }
}

$("#ssh-profile").addEventListener("change", () => {
  const name = $("#ssh-profile").value;
  if (!name) return;
  const opt = $("#ssh-profile").selectedOptions[0];
  // host/user preenchidos manualmente ou via connect com profileName
});

async function refreshSSHStatus() {
  const st = await api("/api/ssh/status");
  state.sshConnected = st.connected;
  const badge = $("#ssh-status");
  badge.textContent = st.connected ? "SSH: online" : "SSH: offline";
  badge.classList.toggle("online", st.connected);
  $("#btn-ssh-connect").hidden = st.connected;
  $("#btn-ssh-disconnect").hidden = !st.connected;
  if (st.connected) {
    await loadRemote(state.remotePath || "/");
  }
}

$("#ssh-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  try {
    const body = {
      profileName: $("#ssh-profile").value,
      host: $("#ssh-host").value,
      user: $("#ssh-user").value,
      password: $("#ssh-pass").value,
    };
    await api("/api/ssh/connect", { method: "POST", body: JSON.stringify(body) });
    await refreshSSHStatus();
  } catch (e) {
    alert("Falha SSH: " + e.message);
  }
});

$("#btn-ssh-disconnect").addEventListener("click", async () => {
  await api("/api/ssh/disconnect", { method: "POST", body: "{}" });
  state.sshConnected = false;
  $("#remote-list").innerHTML =
    '<p class="placeholder">Ligue-se ao servidor SSH para listar ficheiros.</p>';
  await refreshSSHStatus();
});

async function loadLocal(path) {
  const q = path ? `?path=${encodeURIComponent(path)}` : "";
  const data = await api("/api/local/list" + q);
  state.localPath = data.path;
  $("#local-path").textContent = data.path;
  renderEntries($("#local-list"), data.entries, (e) => {
    if (e.isDir) loadLocal(e.path);
  });
}

async function loadRemote(path) {
  if (!state.sshConnected) return;
  const q = `?path=${encodeURIComponent(path || "/")}`;
  const data = await api("/api/remote/list" + q);
  state.remotePath = data.path;
  $("#remote-path").textContent = data.path;
  renderEntries($("#remote-list"), data.entries, (e) => {
    if (e.isDir) loadRemote(e.path);
  });
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

document.querySelectorAll(".panel").forEach((panel) => {
  const side = panel.dataset.side;
  panel.querySelector('[data-action="refresh"]').addEventListener("click", () => {
    if (side === "local") loadLocal(state.localPath);
    else loadRemote(state.remotePath);
  });
  panel.querySelector('[data-action="up"]').addEventListener("click", () => goUp(side));
});

checkSession();
