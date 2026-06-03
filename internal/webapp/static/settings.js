/** Configurações web — conta, interface, SSH, módulos, admin. */
(function () {
  const $ = (sel) => document.querySelector(sel);
  const $$ = (sel) => document.querySelectorAll(sel);

  const PERM_SCREENS = [
    { id: "desktop", label: "Ambiente Linux (simplificado)" },
    { id: "explorer", label: "Gerenciador de arquivos" },
    { id: "docker", label: "Contêineres Docker" },
    { id: "disks", label: "Discos e armazenamento" },
    { id: "services", label: "Serviços (systemd)" },
    { id: "terminal", label: "Terminal SSH" },
    { id: "automations", label: "Central de automações" },
    { id: "settings", label: "Configurações (conta)" },
  ];

  const PERM_ACTIONS = [
    { id: "files.read", label: "Arquivos: listar e visualizar" },
    { id: "files.write", label: "Arquivos: enviar, criar e renomear" },
    { id: "files.delete", label: "Arquivos: excluir" },
    { id: "files.transfer", label: "Arquivos: transferências em lote" },
    { id: "docker.view", label: "Docker: listar, logs e estatísticas" },
    { id: "docker.control", label: "Docker: iniciar, parar e reiniciar" },
    { id: "docker.manage", label: "Docker: criar, remover e limpar" },
    { id: "disks.view", label: "Discos: consultar uso" },
    { id: "disks.manage", label: "Discos: ampliar LV e excluir pastas" },
    { id: "services.view", label: "Serviços: listar e ver logs" },
    { id: "services.control", label: "Serviços: iniciar, parar e reiniciar" },
    { id: "terminal.use", label: "Terminal SSH interativo" },
    { id: "automations.manage", label: "Automações: regras e motor" },
    { id: "sudo.use", label: "Sudo no host remoto" },
    { id: "settings.manage", label: "Administração: usuários e SMTP" },
  ];

  const settingsState = {
    users: [],
    userEditIndex: -1,
    tab: "account",
    connections: [],
    connEdit: null,
    webPolicies: { forcePasswordChange: [] },
    diagnostics: null,
  };

  const ADMIN_TABS = ["security", "notifications", "users", "mail"];

  function escapeHtml(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function defaultPermissionsPayload() {
    return {
      screens: PERM_SCREENS.map((s) => s.id).filter((id) => id !== "settings"),
      actions: PERM_ACTIONS.map((a) => a.id).filter((id) => id !== "settings.manage"),
    };
  }

  function buildPermissionCheckboxGrid(container, items, groupName, selected) {
    if (!container) return;
    container.innerHTML = "";
    const set = new Set(selected || []);
    for (const item of items) {
      const label = document.createElement("label");
      label.className = "settings-perm-item checkbox-row";
      const input = document.createElement("input");
      input.type = "checkbox";
      input.name = groupName;
      input.value = item.id;
      input.checked = set.has(item.id);
      label.appendChild(input);
      label.appendChild(document.createTextNode(" " + item.label));
      container.appendChild(label);
    }
  }

  function readPermissionCheckboxes(container) {
    if (!container) return [];
    return [...container.querySelectorAll('input[type="checkbox"]:checked')].map((el) => el.value);
  }

  function normalizeUserPermissions(p) {
    if (!p || (!p.screens?.length && !p.actions?.length)) {
      return defaultPermissionsPayload();
    }
    return {
      screens: Array.isArray(p.screens) ? [...p.screens] : [],
      actions: Array.isArray(p.actions) ? [...p.actions] : [],
    };
  }

  function permLabel(id, list) {
    return list.find((x) => x.id === id)?.label || id;
  }

  function renderMyPermissions() {
    const box = $("#settings-my-perms");
    if (!box) return;
    const isAdmin = !!state.user?.isAdmin;
    if (isAdmin) {
      box.classList.add("hidden");
      return;
    }
    box.classList.remove("hidden");
    const screens = $("#settings-my-screens");
    const actions = $("#settings-my-actions");
    screens.innerHTML = "";
    actions.innerHTML = "";
    for (const id of state.permissions?.screens || []) {
      const li = document.createElement("li");
      li.textContent = permLabel(id, PERM_SCREENS);
      screens.appendChild(li);
    }
    for (const id of state.permissions?.actions || []) {
      const li = document.createElement("li");
      li.textContent = permLabel(id, PERM_ACTIONS);
      actions.appendChild(li);
    }
    if (!screens.children.length) screens.innerHTML = "<li class='muted'>Nenhuma (acesso total às telas listadas)</li>";
    if (!actions.children.length) actions.innerHTML = "<li class='muted'>Nenhuma</li>";
  }

  function fillUserPermissionForm(user) {
    const box = $("#settings-user-perms");
    if (!box) return;
    const isAdmin = !!user?.isAdmin || user?.username === "admin";
    let adminNote = box.querySelector(".settings-perms-admin-note");
    const screensEl = $("#settings-user-screens");
    const actionsEl = $("#settings-user-actions");
    const titles = box.querySelectorAll(".settings-perms-title");
    const hint = box.querySelector(".settings-perms-hint");
    const colTitle = box.querySelector(".settings-col-title");
    if (isAdmin) {
      if (colTitle) colTitle.hidden = false;
      titles.forEach((el) => { el.hidden = true; });
      if (hint) hint.hidden = true;
      if (screensEl) screensEl.hidden = true;
      if (actionsEl) actionsEl.hidden = true;
      if (!adminNote) {
        adminNote = document.createElement("p");
        adminNote.className = "muted settings-perms-admin-note";
        box.appendChild(adminNote);
      }
      adminNote.hidden = false;
      adminNote.textContent = "O administrador tem acesso total; permissões não se aplicam.";
      return;
    }
    if (colTitle) colTitle.hidden = false;
    if (adminNote) adminNote.hidden = true;
    titles.forEach((el) => { el.hidden = false; });
    if (hint) hint.hidden = false;
    if (screensEl) screensEl.hidden = false;
    if (actionsEl) actionsEl.hidden = false;
    const perms = normalizeUserPermissions(user?.permissions);
    buildPermissionCheckboxGrid(screensEl, PERM_SCREENS, "perm-screen", perms.screens);
    buildPermissionCheckboxGrid(actionsEl, PERM_ACTIONS, "perm-action", perms.actions);
  }

  function closeSettingsUserEditor() {
    $("#settings-users-layout")?.classList.remove("has-editor");
    const panel = $("#settings-user-editor");
    panel?.classList.remove("is-open");
    panel?.setAttribute("aria-hidden", "true");
    panel?.setAttribute("hidden", "");
    settingsState.userEditIndex = -1;
    renderSettingsUsers();
  }

  function openSettingsUserEditor(mode, user) {
    const isNew = mode === "new";
    const u = isNew
      ? { username: "", displayName: "", isAdmin: false, permissions: defaultPermissionsPayload() }
      : {
          username: user.username,
          displayName: user.displayName || user.username,
          isAdmin: !!user.isAdmin,
          permissions: normalizeUserPermissions(user.permissions),
        };
    $("#settings-user-editor-title").textContent = isNew ? "Novo usuário" : "Editar usuário";
    const form = $("#settings-user-form");
    if (form) {
      form.elements.username.value = u.username || "";
      form.elements.username.readOnly = !isNew && u.isAdmin;
      form.elements.displayName.value = u.displayName || "";
      form.elements.password.value = "";
      form.elements.password.placeholder = isNew
        ? "obrigatória"
        : u.isAdmin
          ? "admin: senha reposta ao padrão ao salvar"
          : "deixe vazio para manter";
    }
    fillUserPermissionForm(u);
    const panel = $("#settings-user-editor");
    panel?.querySelector(".settings-user-editor-body")?.scrollTo(0, 0);
    $("#settings-users-layout")?.classList.add("has-editor");
    panel?.removeAttribute("hidden");
    panel?.setAttribute("aria-hidden", "false");
    requestAnimationFrame(() => panel?.classList.add("is-open"));
    renderSettingsUsers();
  }

  function showSettingsTab(tab) {
    settingsState.tab = tab;
    $$(".settings-tab").forEach((b) => {
      const on = b.dataset.tab === tab;
      b.classList.toggle("active", on);
      b.setAttribute("aria-selected", on ? "true" : "false");
    });
    $$(".settings-panel").forEach((p) => {
      const on = p.id === `settings-panel-${tab}`;
      p.classList.toggle("active", on);
      p.hidden = !on;
    });
    const activePanel = document.getElementById(`settings-panel-${tab}`);
    window.CWMotion?.pulseEnter?.(activePanel, "panel-enter");
    if (tab === "users") loadSettingsUsers();
    else closeSettingsUserEditor();
    if (tab === "mail") loadSettingsMail();
    if (tab === "connections") loadSettingsConnections();
    if (tab === "session") refreshSettingsSSHStatus();
    if (tab === "interface") loadInterfaceForm();
    if (tab === "modules") loadModulesForm();
    if (tab === "security") loadSecurityForm();
    if (tab === "notifications") loadNotificationsForm();
    if (tab === "about") loadDiagnostics();
    CWUI.enhanceSelects?.($("#view-settings"));
  }

  function loadInterfaceForm() {
    const form = $("#settings-interface-form");
    if (!form || !window.CWWebPrefs) return;
    const P = CWWebPrefs;
    form.elements.theme.value = P.get("theme");
    form.elements.lang.value = P.get("lang");
    form.elements.homeScreen.value = P.get("homeScreen");
    form.elements.reduceMotion.checked = !!P.get("reduceMotion");
    form.elements.toastSeconds.value = P.get("toastSeconds");
    CWUI.enhanceSelects?.(form);
  }

  function loadModulesForm() {
    const form = $("#settings-modules-form");
    if (!form || !window.CWWebPrefs) return;
    CWWebPrefs.fillForm(form, CWWebPrefs.moduleFields);
    const disksChk = $("#disks-auto-refresh");
    if (disksChk) disksChk.checked = !!CWWebPrefs.get("disksAutoRefresh");
  }

  function loadSessionForm() {
    const form = $("#settings-session-form");
    if (!form || !window.CWWebPrefs) return;
    CWWebPrefs.fillForm(form, CWWebPrefs.sessionFields);
  }

  function refreshSettingsSSHStatus() {
    loadSessionForm();
    const badge = $("#settings-ssh-badge");
    const detail = $("#settings-ssh-detail");
    if (!badge) return;
    if (state.ssh.connected) {
      badge.textContent = "Online";
      badge.className = "settings-ssh-online";
      detail.textContent = `${state.ssh.user}@${state.ssh.host}`;
    } else {
      badge.textContent = "Offline";
      badge.className = "settings-ssh-offline";
      detail.textContent = "Ligue-se na tela de login SSH ou use Testar conexão com um perfil guardado.";
    }
  }

  async function loadSettings() {
    $("#settings-user").textContent = state.user?.displayName || state.user?.username || "—";
    const isAdmin = !!state.user?.isAdmin;
    $("#settings-role").textContent = isAdmin ? "Administrador" : "Usuário";
    const accForm = $("#settings-account-form");
    if (accForm) accForm.elements.displayName.value = state.user?.displayName || "";
    renderMyPermissions();
    for (const id of ["settings-tab-users", "settings-tab-mail", "settings-tab-security", "settings-tab-notifications"]) {
      const el = $(`#${id}`);
      if (el) el.hidden = !isAdmin;
    }
    if (!isAdmin && ADMIN_TABS.includes(settingsState.tab)) {
      showSettingsTab("account");
    }
    const tab = settingsState.tab;
    if (tab === "users" && isAdmin) await loadSettingsUsers();
    if (tab === "mail" && isAdmin) await loadSettingsMail();
    if (tab === "connections") await loadSettingsConnections();
    if (tab === "session") refreshSettingsSSHStatus();
    if (tab === "interface") loadInterfaceForm();
    if (tab === "modules") loadModulesForm();
    if (tab === "security" && isAdmin) await loadSecurityForm();
    if (tab === "notifications" && isAdmin) await loadNotificationsForm();
    if (tab === "about") await loadDiagnostics();
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
        permissions: normalizeUserPermissions(u.permissions),
      }));
      renderSettingsUsers();
      renderForcePasswordCheckboxes();
    } catch (e) {
      box.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
    }
  }

  function renderSettingsUsers() {
    const box = $("#settings-users-list");
    if (!box) return;
    box.innerHTML = "";
    for (let i = 0; i < settingsState.users.length; i++) {
      const u = settingsState.users[i];
      const row = document.createElement("div");
      row.className = "data-row";
      if (i === settingsState.userEditIndex) row.classList.add("is-selected");
      const permHint = u.isAdmin
        ? ""
        : ` · ${(u.permissions?.screens || []).length} telas, ${(u.permissions?.actions || []).length} ações`;
      row.innerHTML = `<div><strong>${escapeHtml(u.displayName)}</strong><span class="muted">@${escapeHtml(u.username)}${u.isAdmin ? " · admin" : permHint}</span></div>`;
      const actions = document.createElement("div");
      actions.className = "data-row-actions";
      const editBtn = document.createElement("button");
      editBtn.type = "button";
      editBtn.className = "btn btn-ghost btn-sm";
      editBtn.textContent = "Editar";
      editBtn.addEventListener("click", () => {
        settingsState.userEditIndex = settingsState.users.indexOf(u);
        openSettingsUserEditor("edit", u);
      });
      actions.appendChild(editBtn);
      if (!u.isAdmin) {
        const delBtn = document.createElement("button");
        delBtn.type = "button";
        delBtn.className = "btn btn-ghost btn-sm";
        delBtn.textContent = "Remover";
        delBtn.addEventListener("click", () => {
          const idx = settingsState.users.indexOf(u);
          settingsState.users = settingsState.users.filter((x) => x !== u);
          if (settingsState.userEditIndex === idx) closeSettingsUserEditor();
          else if (settingsState.userEditIndex > idx) settingsState.userEditIndex -= 1;
          renderSettingsUsers();
          renderForcePasswordCheckboxes();
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

  async function loadSettingsConnections() {
    const box = $("#settings-connections-list");
    box.innerHTML = "…";
    try {
      const data = await api("/api/connections");
      settingsState.connections = data.connections || [];
      box.innerHTML = "";
      if (!settingsState.connections.length) {
        box.innerHTML = "<p class='muted'>Nenhum perfil guardado.</p>";
        return;
      }
      for (const c of settingsState.connections) {
        const row = document.createElement("div");
        row.className = "data-row";
        row.innerHTML = `<div><strong>${escapeHtml(c.name)}</strong><span class="muted">${escapeHtml(c.user)}@${escapeHtml(c.host)}</span></div>`;
        const actions = document.createElement("div");
        actions.className = "data-row-actions";
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "btn btn-ghost btn-sm";
        btn.textContent = "Editar";
        btn.addEventListener("click", () => openConnectionForm(c.name));
        actions.appendChild(btn);
        row.appendChild(actions);
        box.appendChild(row);
      }
    } catch (e) {
      box.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
    }
  }

  async function openConnectionForm(name) {
    const form = $("#settings-conn-form");
    const title = $("#settings-conn-form-title");
    form.classList.remove("hidden");
    if (!name) {
      settingsState.connEdit = null;
      title.textContent = "Novo perfil SSH";
      form.reset();
      form.elements.name.readOnly = false;
      $("#settings-conn-delete").hidden = true;
      return;
    }
    try {
      const data = await api("/api/connections?name=" + encodeURIComponent(name));
      const p = data.profile;
      settingsState.connEdit = p.name;
      title.textContent = `Editar: ${p.name}`;
      form.elements.name.value = p.name;
      form.elements.name.readOnly = true;
      form.elements.host.value = p.host || "";
      form.elements.user.value = p.user || "";
      form.elements.password.value = p.password || "";
      form.elements.keyPath.value = p.keyPath || "";
      form.elements.insecureHostKey.checked = !!p.insecureHostKey;
      form.elements.dockerSocket.value = p.dockerSocket || "";
      $("#settings-conn-delete").hidden = false;
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  function renderForcePasswordCheckboxes() {
    const box = $("#settings-force-password-list");
    if (!box) return;
    box.innerHTML = "";
    const forced = new Set(settingsState.webPolicies.forcePasswordChange || []);
    for (const u of settingsState.users) {
      if (u.isAdmin) continue;
      const label = document.createElement("label");
      label.className = "settings-perm-item checkbox-row";
      const input = document.createElement("input");
      input.type = "checkbox";
      input.value = u.username;
      input.checked = forced.has(u.username);
      label.appendChild(input);
      label.appendChild(document.createTextNode(` ${u.displayName} (@${u.username})`));
      box.appendChild(label);
    }
  }

  async function loadSecurityForm() {
    try {
      const data = await api("/api/admin/web-settings");
      const p = data.policies || {};
      settingsState.webPolicies.forcePasswordChange = p.forcePasswordChange || [];
      const form = $("#settings-security-form");
      form.elements.sessionIdleMinutes.value = p.sessionIdleMinutes ?? 0;
      form.elements.maxLoginAttempts.value = p.maxLoginAttempts ?? 0;
      if (!settingsState.users.length) await loadSettingsUsers();
      else renderForcePasswordCheckboxes();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  async function loadNotificationsForm() {
    try {
      const data = await api("/api/admin/web-settings");
      const n = data.notifications || {};
      const form = $("#settings-notifications-form");
      form.elements.automationFail.checked = !!n.automationFail;
      form.elements.dockerStopped.checked = !!n.dockerStopped;
      form.elements.diskSpaceLow.checked = !!n.diskSpaceLow;
      form.elements.loginFailure.checked = !!n.loginFailure;
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  }

  function renderDiagnostics(data) {
    settingsState.diagnostics = data;
    const box = $("#settings-diagnostics");
    if (!box) return;
    const paths = data.paths || {};
    box.innerHTML = `
      <dl class="settings-dl">
        <dt>Produto</dt><dd>${escapeHtml(data.product || "—")}</dd>
        <dt>Versão</dt><dd>${escapeHtml(data.version || "—")}</dd>
        <dt>Sistema</dt><dd>${escapeHtml(data.goos || "")}/${escapeHtml(data.goarch || "")}</dd>
        <dt>Utilizador</dt><dd>${escapeHtml(data.displayName || data.user || "—")}</dd>
        <dt>SSH nesta sessão</dt><dd>${data.sshConnected ? "Conectado" : "Desconectado"}</dd>
        <dt>Configuração</dt><dd><code>${escapeHtml(paths.config || "—")}</code></dd>
        <dt>Preferências</dt><dd><code>${escapeHtml(paths.preferences || "—")}</code></dd>
        <dt>Web (web.json)</dt><dd><code>${escapeHtml(paths.webSettings || "—")}</code></dd>
        <dt>Log web</dt><dd><code>${escapeHtml(paths.webLog || "—")}</code></dd>
      </dl>`;
  }

  async function loadDiagnostics() {
    const box = $("#settings-diagnostics");
    if (box) box.innerHTML = "<p class='muted'>A carregar…</p>";
    try {
      const data = await api("/api/settings/diagnostics");
      renderDiagnostics(data);
    } catch (e) {
      if (box) box.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
    }
  }

  function applyModulePrefsGlobally() {
    const P = window.CWWebPrefs;
    if (!P) return;
    if (P.get("explorerCompactToolbar")) {
      $("#view-explorer")?.classList.add("explorer-compact");
    } else {
      $("#view-explorer")?.classList.remove("explorer-compact");
    }
    const disksChk = $("#disks-auto-refresh");
    if (disksChk) disksChk.checked = !!P.get("disksAutoRefresh");
    if (window.dockerState) {
      const sec = Number(P.get("dockerMetricsSec")) || 15;
      const ms = Math.min(120000, Math.max(5000, sec * 1000));
      window.dockerState.pollMs = ms;
      if (typeof window.dockerRestartPoll === "function") window.dockerRestartPoll();
    }
  }

  function applyThemeFromInterface(theme) {
    const P = window.CWWebPrefs;
    if (!P) return;
    P.set("theme", theme);
    localStorage.setItem("cw-web-theme", theme === "system" ? "" : theme);
    const resolved = P.applyThemeFromPrefs();
    if (typeof syncMascotIcons === "function") syncMascotIcons(resolved);
  }

  function promptMustChangePassword() {
    return new Promise((resolve) => {
      if (!state.user?.mustChangePassword) {
        resolve(true);
        return;
      }
      const dlg = $("#settings-password-dialog");
      const form = $("#settings-password-form");
      if (!dlg || !form) {
        resolve(true);
        return;
      }
      const onSubmit = async (ev) => {
        ev.preventDefault();
        try {
          await api("/api/auth/me", {
            method: "PATCH",
            body: JSON.stringify({
              currentPassword: form.elements.currentPassword.value,
              newPassword: form.elements.newPassword.value,
            }),
          });
          state.user.mustChangePassword = false;
          dlg.close();
          CWUI.toast("Senha alterada", "success");
          resolve(true);
        } catch (e) {
          CWUI.toast(e.message, "error");
        }
      };
      form.addEventListener("submit", onSubmit, { once: true });
      dlg.showModal();
    });
  }

  function navigateHomeScreen() {
    const home = CWWebPrefs?.get?.("homeScreen") || "hub";
    if (home && home !== "hub" && typeof canScreen === "function" && canScreen(home)) {
      showScreen(home);
      if (home === "docker") loadDocker();
      return;
    }
    showScreen("hub");
    renderHub();
  }

  $$(".settings-tab").forEach((btn) => {
    btn.addEventListener("click", () => showSettingsTab(btn.dataset.tab));
  });

  $("#settings-account-form")?.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const f = ev.target;
    try {
      const body = { displayName: f.elements.displayName.value.trim() };
      if (f.elements.newPassword.value) {
        body.currentPassword = f.elements.currentPassword.value;
        body.newPassword = f.elements.newPassword.value;
      }
      const res = await api("/api/auth/me", { method: "PATCH", body: JSON.stringify(body) });
      state.user.displayName = res.displayName || state.user.displayName;
      $("#user-label").textContent = state.user.displayName || state.user.username;
      f.elements.currentPassword.value = "";
      f.elements.newPassword.value = "";
      CWUI.toast("Conta atualizada", "success");
      await loadSettings();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  $("#settings-interface-form")?.addEventListener("submit", (ev) => {
    ev.preventDefault();
    const f = ev.target;
    const P = CWWebPrefs;
    P.set("theme", f.elements.theme.value);
    P.set("lang", f.elements.lang.value);
    P.set("homeScreen", f.elements.homeScreen.value);
    P.set("reduceMotion", f.elements.reduceMotion.checked);
    P.set("toastSeconds", Number(f.elements.toastSeconds.value) || 5);
    applyThemeFromInterface(f.elements.theme.value);
    P.applyLangFromPrefs();
    P.applyReduceMotion();
    CWUI.toast("Interface salva", "success");
  });

  $("#settings-session-form")?.addEventListener("submit", (ev) => {
    ev.preventDefault();
    CWWebPrefs.saveForm(ev.target, CWWebPrefs.sessionFields);
    CWUI.toast("Preferências de sessão salvas", "success");
  });

  $("#settings-modules-form")?.addEventListener("submit", (ev) => {
    ev.preventDefault();
    const f = ev.target;
    CWWebPrefs.saveForm(f, CWWebPrefs.moduleFields);
    const disksChk = $("#disks-auto-refresh");
    if (disksChk) CWWebPrefs.set("disksAutoRefresh", disksChk.checked);
    applyModulePrefsGlobally();
    CWUI.toast("Preferências dos módulos salvas", "success");
  });

  $("#settings-ssh-test")?.addEventListener("click", async () => {
    const out = $("#settings-ssh-test-result");
    if (state.ssh.connected) {
      out.textContent = `Já conectado a ${state.ssh.user}@${state.ssh.host}.`;
      return;
    }
    const prof = CWWebPrefs.get("lastSSHProfile");
    if (!prof) {
      CWUI.toast("Conecte-se na tela de login ou guarde um perfil em Conexões.", "warn");
      return;
    }
    out.textContent = "A testar…";
    try {
      const data = await api("/api/ssh/test", {
        method: "POST",
        body: JSON.stringify({ profileName: prof }),
      });
      out.textContent = data.ok ? "Teste OK" : (data.error || "Falhou");
    } catch (e) {
      out.textContent = e.message;
    }
  });

  $("#settings-ssh-disconnect")?.addEventListener("click", async () => {
    try {
      await api("/api/ssh/disconnect", { method: "POST", body: "{}" });
      await refreshSSH();
      refreshSettingsSSHStatus();
      CWUI.toast("SSH desligado", "success");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  $("#settings-conn-add")?.addEventListener("click", () => openConnectionForm(null));
  $("#settings-conn-cancel")?.addEventListener("click", () => {
    $("#settings-conn-form")?.classList.add("hidden");
    settingsState.connEdit = null;
  });

  $("#settings-conn-form")?.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const f = ev.target;
    try {
      await api("/api/connections", {
        method: "POST",
        body: JSON.stringify({
          name: f.elements.name.value.trim(),
          host: f.elements.host.value.trim(),
          user: f.elements.user.value.trim(),
          password: f.elements.password.value,
          keyPath: f.elements.keyPath.value.trim(),
          insecureHostKey: f.elements.insecureHostKey.checked,
          dockerSocket: f.elements.dockerSocket.value.trim(),
          savePassword: !!f.elements.password.value,
        }),
      });
      CWUI.toast("Perfil salvo", "success");
      f.classList.add("hidden");
      await loadSettingsConnections();
      if (typeof refreshConnections === "function") refreshConnections(f.elements.name.value);
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  $("#settings-conn-delete")?.addEventListener("click", async () => {
    const name = settingsState.connEdit || $("#settings-conn-form")?.elements.name?.value;
    if (!name || !(await CWConfirm(`Excluir perfil "${name}"?`, { danger: true, ok: "Excluir" }))) return;
    try {
      await api("/api/connections?name=" + encodeURIComponent(name), { method: "DELETE" });
      $("#settings-conn-form")?.classList.add("hidden");
      await loadSettingsConnections();
      CWUI.toast("Perfil excluído", "success");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  $("#settings-security-form")?.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const f = ev.target;
    const forced = [...f.querySelectorAll("#settings-force-password-list input:checked")].map((el) => el.value);
    try {
      await api("/api/admin/web-settings", {
        method: "PUT",
        body: JSON.stringify({
          policies: {
            sessionIdleMinutes: parseInt(f.elements.sessionIdleMinutes.value, 10) || 0,
            maxLoginAttempts: parseInt(f.elements.maxLoginAttempts.value, 10) || 0,
            forcePasswordChange: forced,
          },
        }),
      });
      settingsState.webPolicies.forcePasswordChange = forced;
      CWUI.toast("Segurança salva", "success");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  $("#settings-notifications-form")?.addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const f = ev.target;
    try {
      await api("/api/admin/web-settings", {
        method: "PUT",
        body: JSON.stringify({
          notifications: {
            automationFail: f.elements.automationFail.checked,
            dockerStopped: f.elements.dockerStopped.checked,
            diskSpaceLow: f.elements.diskSpaceLow.checked,
            loginFailure: f.elements.loginFailure.checked,
          },
        }),
      });
      CWUI.toast("Notificações salvas", "success");
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  $("#settings-user-add")?.addEventListener("click", () => {
    settingsState.userEditIndex = -1;
    openSettingsUserEditor("new");
  });
  $("#settings-user-cancel")?.addEventListener("click", () => closeSettingsUserEditor());
  $("#settings-user-close")?.addEventListener("click", () => closeSettingsUserEditor());

  $("#settings-user-form")?.addEventListener("submit", (ev) => {
    ev.preventDefault();
    const form = ev.target;
    const username = form.elements.username.value.trim().toLowerCase();
    const displayName = form.elements.displayName.value.trim() || username;
    const password = form.elements.password.value;
    if (!username) return;
    const isAdmin = username === "admin";
    const entry = {
      username,
      displayName,
      password,
      isAdmin,
      permissions: isAdmin
        ? null
        : {
            screens: readPermissionCheckboxes($("#settings-user-screens")),
            actions: readPermissionCheckboxes($("#settings-user-actions")),
          },
    };
    const idx = settingsState.userEditIndex;
    if (idx >= 0) {
      settingsState.users[idx] = { ...settingsState.users[idx], ...entry };
    } else {
      if (!password) {
        CWUI.toast("Senha obrigatória para novo usuário.", "error");
        return;
      }
      if (settingsState.users.some((u) => u.username === username)) {
        CWUI.toast("Usuário já existe.", "error");
        return;
      }
      settingsState.users.push(entry);
    }
    closeSettingsUserEditor();
    renderForcePasswordCheckboxes();
  });

  $("#settings-users-save")?.addEventListener("click", async () => {
    try {
      const users = settingsState.users.map((u) => ({
        username: u.username,
        displayName: u.displayName,
        password: u.password || "",
        permissions: u.isAdmin ? undefined : u.permissions,
      }));
      await api("/api/admin/users", { method: "PUT", body: JSON.stringify({ users }) });
      CWUI.toast("Usuários salvos", "success");
      await loadSettingsUsers();
    } catch (e) {
      CWUI.toast(e.message, "error");
    }
  });

  $("#settings-mail-form")?.addEventListener("submit", async (ev) => {
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
      CWUI.toast("Configuração SMTP salva", "success");
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
  $("#settings-mail-test-self")?.addEventListener("click", () => testSettingsMail("self"));
  $("#settings-mail-test-all")?.addEventListener("click", () => testSettingsMail("recipients"));

  $("#settings-copy-diagnostics")?.addEventListener("click", async () => {
    if (!settingsState.diagnostics) await loadDiagnostics();
    const text = JSON.stringify(settingsState.diagnostics, null, 2);
    try {
      await navigator.clipboard.writeText(text);
      CWUI.toast("Diagnóstico copiado", "success");
    } catch {
      CWUI.toast("Não foi possível copiar", "error");
    }
  });
  $("#settings-refresh-diagnostics")?.addEventListener("click", () => loadDiagnostics());

  document.addEventListener("DOMContentLoaded", () => {
    applyModulePrefsGlobally();
    const savedTheme = localStorage.getItem("cw-web-theme");
    if (savedTheme && window.CWWebPrefs) {
      CWWebPrefs.set("theme", savedTheme);
    }
  });

  window.CWSettings = {
    loadSettings,
    showSettingsTab,
    promptMustChangePassword,
    navigateHomeScreen,
    applyModulePrefsGlobally,
    PERM_SCREENS,
    PERM_ACTIONS,
  };
})();
