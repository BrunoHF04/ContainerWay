/** Inicialização das melhorias visuais (comandos, latência, versão). */
(function () {
  const U = window.CWUI;
  if (!U) return;

  function buildCommands() {
    const can = (id) => (typeof window.canScreen === "function" ? window.canScreen(id) : true);
    const all = [
      { id: "hub", title: "Início (hub)", sub: "Menu principal", kw: "hub", run: () => { showScreen("hub"); renderHub(); } },
      { id: "desktop", title: "Ambiente Linux", sub: "Interface simplificada", screen: "desktop", run: () => showScreen("desktop") },
      {
        id: "connect",
        title: "Conexão SSH",
        sub: "Perfis",
        run: () => {
          setAppTopbar(true);
          showView("#view-login");
          showLoginPhase("connect");
        },
      },
      { id: "explorer", title: "Explorador", sub: "Arquivos SFTP", screen: "explorer", run: () => showScreen("explorer") },
      { id: "docker", title: "Docker", sub: "Contêineres", screen: "docker", run: () => showScreen("docker") },
      { id: "disks", title: "Discos", sub: "Armazenamento", screen: "disks", run: () => showScreen("disks") },
      { id: "terminal", title: "Terminal", sub: "SSH interativo", screen: "terminal", run: () => showScreen("terminal") },
      { id: "auto", title: "Automações", sub: "Regras", screen: "automations", run: () => showScreen("automations") },
      { id: "settings", title: "Configurações", sub: "Conta / usuários / SMTP", screen: "settings", run: () => showScreen("settings") },
      { id: "theme", title: "Alternar tema", sub: "Claro ou escuro", run: () => toggleTheme() },
      { id: "disconnect", title: "Desconectar SSH", sub: "", run: () => $("#btn-disconnect")?.click() },
    ];
    return all.filter((c) => !c.screen || can(c.screen));
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
    let remotePrefix = "SFTP";
    if (state.remoteTarget === "container") {
      const opt = $("#remote-container")?.selectedOptions?.[0];
      const label = opt?.textContent?.trim() || "contêiner";
      remotePrefix = `Docker · ${label}`;
    }
    U.renderBreadcrumbs(
      $("#remote-bc"),
      U.pathToBreadcrumbs(state.remotePath, false),
      (p) => loadRemote(p),
      remotePrefix
    );
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

  $("#btn-shortcuts")?.addEventListener("click", () => {
    if (typeof window.openScreenHelp === "function") window.openScreenHelp();
  });
  $("#btn-toggle-xfer-panel")?.addEventListener("click", () => {
    const dock = $("#explorer-xfer-dock");
    if (dock) {
      dock.classList.toggle("collapsed");
      if (typeof window.syncXferDockToggle === "function") window.syncXferDockToggle();
    } else $("#transfer-progress-panel")?.classList.toggle("hidden");
    refreshTransferStatus();
  });

  setInterval(() => {
    if (state.ssh.connected) window.measureSSHLatency();
  }, 20000);

  window.CWConfirm = (msg, opts) => U.confirmDialog(msg, opts);
  window.CWToast = (msg, type) => U.toast(msg, type);
})();
