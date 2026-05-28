/** Inicialização das melhorias visuais (comandos, latência, versão). */
(function () {
  const U = window.CWUI;
  if (!U) return;

  function buildCommands() {
    return [
      { id: "hub", title: "Início (hub)", sub: "Menu principal", kw: "hub", run: () => { showScreen("hub"); renderHub(); } },
      { id: "connect", title: "Ligação SSH", sub: "Perfis", run: () => showScreen("connect") },
      { id: "explorer", title: "Explorador", sub: "Ficheiros SFTP", run: () => showScreen("explorer") },
      { id: "docker", title: "Docker", sub: "Contêineres", run: () => showScreen("docker") },
      { id: "disks", title: "Discos", sub: "Armazenamento", run: () => showScreen("disks") },
      { id: "terminal", title: "Terminal", sub: "SSH interativo", run: () => showScreen("terminal") },
      { id: "auto", title: "Automações", sub: "Regras", run: () => showScreen("automations") },
      { id: "settings", title: "Configurações", sub: "Conta / SMTP", run: () => showScreen("settings") },
      { id: "theme", title: "Alternar tema", sub: "Claro ou escuro", run: () => toggleTheme() },
      { id: "disconnect", title: "Desconectar SSH", sub: "", run: () => $("#btn-disconnect")?.click() },
    ];
  }

  window.measureSSHLatency = async function () {
    const badge = $("#ssh-latency");
    if (!badge || !state.ssh.connected) {
      badge?.classList.add("hidden");
      return;
    }
    const t0 = performance.now();
    try {
      await api(`/api/remote/list?path=${encodeURIComponent(state.remotePath || "/")}`);
      const ms = Math.round(performance.now() - t0);
      badge.textContent = `${ms} ms`;
      badge.classList.remove("hidden", "ok", "warn", "bad");
      badge.classList.add(ms < 120 ? "ok" : ms < 350 ? "warn" : "bad");
    } catch {
      badge.classList.add("hidden");
    }
  };

  window.updateExplorerBreadcrumbs = function () {
    const isWin = state.localPath && /^[A-Za-z]:/.test(state.localPath);
    U.renderBreadcrumbs($("#local-bc"), U.pathToBreadcrumbs(state.localPath, isWin), (p) => loadLocal(p));
    U.renderBreadcrumbs($("#remote-bc"), U.pathToBreadcrumbs(state.remotePath, false), (p) => loadRemote(p));
  };

  async function loadAppVersion() {
    try {
      const h = await api("/api/health");
      const el = $("#settings-version");
      if (el) el.textContent = h.version || "—";
    } catch { /* ignore */ }
  }

  U.registerCommands(buildCommands());
  U.bindCommandPalette();
  U.bindRipple();
  loadAppVersion();

  $("#btn-shortcuts")?.addEventListener("click", () => $("#shortcuts-dialog")?.showModal());
  $("#btn-toggle-xfer-panel")?.addEventListener("click", () => {
    $("#transfer-progress-panel")?.classList.toggle("hidden");
    refreshTransferStatus();
  });

  setInterval(() => {
    if (state.ssh.connected) window.measureSSHLatency();
  }, 20000);

  window.CWConfirm = (msg, opts) => U.confirmDialog(msg, opts);
  window.CWToast = (msg, type) => U.toast(msg, type);
})();
