/** Terminal SSH — barra de ferramentas e lista de comandos (paridade com desktop). */
(function () {
  const $ = (sel) => document.querySelector(sel);
  const t = (k, vars) => window.CWI18n?.t(k, vars) || k;

  const CMD_HTOP =
    "command -v htop >/dev/null 2>&1 || { echo '[ContainerWay] htop não encontrado. Instalando...'; if command -v apt-get >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y htop; elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y htop; elif command -v yum >/dev/null 2>&1; then sudo yum install -y htop; elif command -v pacman >/dev/null 2>&1; then sudo pacman -Sy --noconfirm htop; else echo '[ContainerWay] Gerenciador de pacotes não suportado para instalação automática.'; fi; }; command -v htop >/dev/null 2>&1 && htop";

  const CMD_NCDU =
    "command -v ncdu >/dev/null 2>&1 || { echo '[ContainerWay] ncdu não encontrado. Instalando...'; if command -v apt-get >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y ncdu; elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y ncdu; elif command -v yum >/dev/null 2>&1; then sudo yum install -y ncdu; elif command -v pacman >/dev/null 2>&1; then sudo pacman -Sy --noconfirm ncdu; else echo '[ContainerWay] Gerenciador de pacotes não suportado para instalação automática.'; fi; }; command -v ncdu >/dev/null 2>&1 && cd / && ncdu";

  const COMMAND_SECTIONS = [
    {
      title: "Criar pasta",
      items: [
        { label: "Criar nova pasta", cmd: "mkdir nome_pasta", quick: true, profile: "Dev" },
        { label: "Criar com subpastas", cmd: "mkdir -p caminho/pasta/subpasta", profile: "Dev" },
      ],
    },
    {
      title: "Permissões",
      items: [
        { label: "Liberar tudo (recursivo)", cmd: "chmod -R 777 caminho_diretorio", quick: true, profile: "Infra" },
        { label: "Permissão recomendada (recursivo)", cmd: "chmod -R 755 caminho_diretorio", profile: "Infra" },
      ],
    },
    {
      title: "Navegação",
      items: [
        { label: "Mostrar caminho atual", cmd: "pwd", profile: "Dev" },
        { label: "Listar arquivos detalhado", cmd: "ls -lah", quick: true, profile: "Dev" },
        { label: "Entrar em diretório", cmd: "cd /caminho", quick: true, profile: "Dev" },
        { label: "Voltar um nível", cmd: "cd ..", quick: true, profile: "Dev" },
      ],
    },
    {
      title: "Arquivos e diretórios",
      items: [
        { label: "Criar arquivo vazio", cmd: "touch arquivo.txt", profile: "Dev" },
        { label: "Copiar arquivo", cmd: "cp arquivo.txt /destino/", profile: "Dev" },
        { label: "Renomear/mover", cmd: "mv arquivo.txt novo_nome.txt", profile: "Dev" },
        { label: "Remover arquivo", cmd: "rm arquivo.txt", profile: "Dev" },
        { label: "Remover pasta", cmd: "rm -rf pasta_antiga", quick: true, profile: "Infra" },
      ],
    },
    {
      title: "Disco e memória",
      items: [
        { label: "Uso de disco", cmd: "df -h", quick: true, profile: "Infra" },
        { label: "Tamanho de pasta", cmd: "du -sh /var/log", profile: "Infra" },
        { label: "Uso de memória", cmd: "free -h", quick: true, profile: "Infra" },
      ],
    },
    {
      title: "Processos e serviços",
      items: [
        { label: "Filtrar processo", cmd: "ps aux | grep nome", profile: "Infra" },
        { label: "Monitor de processos", cmd: "top", quick: true, profile: "Infra" },
        { label: "Monitor avançado", cmd: "htop", quick: true, profile: "Infra" },
        { label: "Status de serviço", cmd: "systemctl status nginx", profile: "Infra" },
      ],
    },
    {
      title: "Rede",
      items: [
        { label: "Interfaces de rede", cmd: "ip a", profile: "Infra" },
        { label: "Teste de conectividade", cmd: "ping 8.8.8.8", quick: true, profile: "Infra" },
        { label: "Portas abertas", cmd: "ss -tulpen", profile: "Infra" },
      ],
    },
    {
      title: "Dono e grupo",
      items: [
        { label: "Alterar dono/grupo", cmd: "chown usuario:grupo arquivo_ou_pasta", profile: "Infra" },
        { label: "Alterar dono/grupo recursivo", cmd: "chown -R usuario:grupo caminho_diretorio", profile: "Infra" },
      ],
    },
    {
      title: "Docker",
      items: [
        { label: "Listar contêineres", cmd: "docker ps -a", quick: true, profile: "Docker" },
        { label: "Ver logs do contêiner", cmd: "docker logs -f nome_container", profile: "Docker" },
        { label: "Entrar no contêiner", cmd: "docker exec -it nome_container bash", quick: true, profile: "Docker" },
        { label: "Reiniciar contêiner", cmd: "docker restart nome_container", profile: "Docker" },
      ],
    },
    {
      title: "Docker Compose",
      items: [
        {
          label: "Listar serviços do compose",
          cmd: "docker compose -f /opt/siplan/docker-compose-orion.yml -p orion ps",
          profile: "Docker",
        },
        {
          label: "Reiniciar tudo (recriar + pull)",
          cmd: "docker compose -f /opt/siplan/docker-compose-orion.yml -p orion up -d --force-recreate --pull always",
          quick: true,
          profile: "Docker",
        },
        {
          label: "Reiniciar serviço específico",
          cmd: "docker compose -f /opt/siplan/docker-compose-orion.yml -p orion up -d --force-recreate --pull always nome_servico",
          quick: true,
          profile: "Docker",
        },
      ],
    },
  ];

  const DANGER = ["rm -rf /", "rm -rf", "chmod -r 777", "chmod 777 -r", "mkfs", "dd if=", ":(){ :|:& };:"];

  let favoritesMap = {};
  let cmdApiOverride = null;
  let statusElOverride = null;

  function getTermApi() {
    return cmdApiOverride || window.CWTerminal;
  }

  function loadSectionPrefs() {
    try {
      const raw = window.CWWebPrefs?.get?.("terminalCmdSections") || "{}";
      return JSON.parse(raw) || {};
    } catch {
      return {};
    }
  }

  function saveSectionPrefs(prefs) {
    try {
      window.CWWebPrefs?.set?.("terminalCmdSections", JSON.stringify(prefs));
    } catch (_) { /* */ }
  }

  function sectionKey(title) {
    return String(title).replace(/\s+/g, "_").slice(0, 48);
  }

  function createTermApi(term, ws, opts = {}) {
    const logRef = opts.logRef || { value: "" };
    return {
      isConnected: () => ws?.readyState === WebSocket.OPEN,
      sendRaw(data) {
        if (!data || ws?.readyState !== WebSocket.OPEN) return false;
        ws.send(data);
        return true;
      },
      sendCmd(cmd, run) {
        const c = String(cmd ?? "");
        if (!c.trim() || ws?.readyState !== WebSocket.OPEN) return false;
        term?.write?.(c + (run ? "\r" : ""));
        ws.send(c + (run ? "\r" : ""));
        term?.focus?.();
        return true;
      },
      ctrlC: () => {
        if (ws?.readyState !== WebSocket.OPEN) return false;
        ws.send("\x03");
        return true;
      },
      clear: () => {
        term?.clear?.();
        logRef.value = "";
        if (ws?.readyState === WebSocket.OPEN) {
          term?.write?.("clear\r");
          ws.send("clear\r");
        }
        return true;
      },
      getLog: () => logRef.value || "",
      focus: () => term?.focus?.(),
      reconnect: opts.onReconnect,
    };
  }

  function escapeHtml(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function loadFavoritesMap() {
    favoritesMap = {};
    const raw = window.CWWebPrefs?.get?.("terminalFavorites") || "";
    raw
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean)
      .forEach((cmd) => {
        favoritesMap[cmd] = true;
      });
  }

  function saveFavoritesMap() {
    const flat = Object.keys(favoritesMap).filter((k) => favoritesMap[k]);
    window.CWWebPrefs?.set?.("terminalFavorites", flat.join("\n"));
  }

  function isDangerous(cmd) {
    const c = String(cmd).toLowerCase().trim();
    return DANGER.some((p) => c.includes(p));
  }

  function showTermStatus(msg, ms = 2200) {
    const el = statusElOverride || $("#terminal-status");
    if (!el) return;
    el.textContent = msg;
    el.classList.remove("hidden");
    clearTimeout(showTermStatus._t);
    showTermStatus._t = setTimeout(() => el.classList.add("hidden"), ms);
  }

  function ensureConnected() {
    if (getTermApi()?.isConnected?.()) return true;
    CWUI?.toast?.(t("term.err.offline"), "error");
    return false;
  }

  async function confirmAction(actionLabel, cmd, run) {
    const mode = run ? t("term.mode.exec") : t("term.mode.insert");
    const body = t("term.action.intro", { action: actionLabel, mode, cmd });
    return CWConfirm(`${t("term.action.title")}\n\n${body}`, { title: t("term.action.title") });
  }

  async function insertCommand(cmd) {
    if (!ensureConnected()) return;
    if (!(await confirmAction(t("term.action.insert"), cmd, false))) return;
    getTermApi().sendCmd(cmd, false);
    showTermStatus(t("term.status.inserted", { cmd }));
  }

  async function runCommand(cmd) {
    if (!ensureConnected()) return;
    if (isDangerous(cmd)) {
      if (!(await CWConfirm(t("term.danger.body", { cmd }), { title: t("term.danger.title") }))) return;
    } else if (!(await confirmAction(t("term.action.run"), cmd, true))) return;
    getTermApi().sendCmd(cmd, true);
    showTermStatus(t("term.status.ran", { cmd }));
  }

  function composePrefix() {
    const file = $("#term-compose-file")?.value?.trim() || "";
    const project = $("#term-compose-project")?.value?.trim() || "";
    let base = "docker compose";
    if (file) base += ` -f ${file}`;
    if (project) base += ` -p ${project}`;
    return base;
  }

  function composeService() {
    const s = $("#term-compose-service")?.value?.trim() || "";
    if (!s) {
      showTermStatus(t("term.compose.needService"));
      return null;
    }
    return s;
  }

  function buildComposeActions() {
    const host = $("#term-compose-actions");
    if (!host) return;
    const actions = [
      { label: "Inserir validação", cmd: () => `${composePrefix()} config -q`, run: false },
      { label: "Validar e executar", cmd: () => `${composePrefix()} config -q`, run: true },
      { label: "Inserir reiniciar tudo", cmd: () => `${composePrefix()} up -d --force-recreate --pull always`, run: false },
      { label: "Executar reiniciar tudo", cmd: () => `${composePrefix()} up -d --force-recreate --pull always`, run: true },
      {
        label: "Inserir reiniciar serviço",
        cmd: () => {
          const svc = composeService();
          return svc ? `${composePrefix()} up -d --force-recreate --pull always ${svc}` : null;
        },
        run: false,
      },
      {
        label: "Executar reiniciar serviço",
        cmd: () => {
          const svc = composeService();
          return svc ? `${composePrefix()} up -d --force-recreate --pull always ${svc}` : null;
        },
        run: true,
      },
      { label: "Diagnóstico: ps", cmd: () => `${composePrefix()} ps`, run: true },
      {
        label: "Diagnóstico: logs serviço",
        cmd: () => {
          const svc = composeService();
          return svc ? `${composePrefix()} logs -f ${svc}` : null;
        },
        run: true,
      },
      {
        label: "Diagnóstico: top serviço",
        cmd: () => {
          const svc = composeService();
          return svc ? `${composePrefix()} top ${svc}` : null;
        },
        run: true,
      },
      {
        label: "Buscar compose em /opt",
        cmd: () =>
          "find /opt -maxdepth 5 -type f \\( -name 'docker-compose*.yml' -o -name 'docker-compose*.yaml' -o -name 'compose*.yml' -o -name 'compose*.yaml' \\) 2>/dev/null | head -n 20",
        run: true,
      },
      {
        label: "Executar down && up",
        cmd: () => {
          const p = composePrefix();
          return `${p} down && ${p} up -d --pull always`;
        },
        run: true,
      },
      {
        label: "Saúde pós-deploy",
        cmd: () => {
          const p = composePrefix();
          return `${p} ps; ${p} ps | grep -Ei 'unhealthy|exit|restarting' || echo 'Sem serviços em estado crítico (unhealthy/exited/restarting).'`;
        },
        run: true,
      },
    ];
    host.innerHTML = actions
      .map(
        (a, i) =>
          `<button type="button" class="btn btn-ghost" data-compose-idx="${i}">${escapeHtml(a.label)}</button>`
      )
      .join("");
    host.querySelectorAll("[data-compose-idx]").forEach((btn) => {
      btn.addEventListener("click", async () => {
        const a = actions[Number(btn.dataset.composeIdx)];
        const cmd = a.cmd();
        if (!cmd) return;
        if (a.run) await runCommand(cmd);
        else await insertCommand(cmd);
      });
    });
  }

  function buildCommandRow(item) {
    const fav = !!favoritesMap[item.cmd];
    const row = document.createElement("div");
    row.className = "terminal-cmd-row";
    row.innerHTML = `
      <div class="terminal-cmd-row-text">
        <strong>${escapeHtml(item.label)}</strong>
        <code class="terminal-cmd-code">${escapeHtml(item.cmd)}</code>
      </div>
      <div class="terminal-cmd-row-actions">
        <button type="button" class="btn btn-ghost" data-act="copy" title="Copiar">⎘</button>
        <button type="button" class="btn btn-ghost" data-act="insert">${escapeHtml(t("term.cmd.insert"))}</button>
        <button type="button" class="btn btn-primary" data-act="run">${escapeHtml(t("term.cmd.run"))}</button>
        <button type="button" class="btn btn-ghost term-fav-btn${fav ? " is-fav" : ""}" data-act="fav" title="Favorito">${fav ? "★" : "☆"}</button>
      </div>`;
    row.querySelector('[data-act="copy"]').addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(item.cmd);
        showTermStatus(t("term.status.copied") || "Copiado");
      } catch {
        CWUI?.toast?.(t("term.err.copyFail"), "error");
      }
    });
    row.querySelector('[data-act="insert"]').addEventListener("click", () => insertCommand(item.cmd));
    row.querySelector('[data-act="run"]').addEventListener("click", () => runCommand(item.cmd));
    row.querySelector('[data-act="fav"]').addEventListener("click", (e) => {
      const btn = e.currentTarget;
      favoritesMap[item.cmd] = !favoritesMap[item.cmd];
      saveFavoritesMap();
      btn.classList.toggle("is-fav", favoritesMap[item.cmd]);
      btn.textContent = favoritesMap[item.cmd] ? "★" : "☆";
      showTermStatus(
        favoritesMap[item.cmd] ? t("term.fav.added", { cmd: item.cmd }) : t("term.fav.removed", { cmd: item.cmd })
      );
      rebuildCmdResults();
    });
    return row;
  }

  function buildCmdSection(title, items, expand) {
    const det = document.createElement("details");
    det.className = "terminal-cmd-section";
    const key = sectionKey(title);
    const prefs = loadSectionPrefs();
    if (expand) det.open = true;
    else if (prefs[key]) det.open = true;
    det.addEventListener("toggle", () => {
      const p = loadSectionPrefs();
      p[key] = det.open;
      saveSectionPrefs(p);
    });
    const sum = document.createElement("summary");
    sum.className = "terminal-cmd-section-summary";
    sum.textContent = `${title} (${items.length})`;
    const body = document.createElement("div");
    body.className = "terminal-cmd-section-body";
    items.forEach((item) => body.appendChild(buildCommandRow(item)));
    det.append(sum, body);
    return det;
  }

  function rebuildCmdResults() {
    const host = $("#term-cmd-results");
    if (!host) return;
    const filter = ($("#term-cmd-search")?.value || "").toLowerCase().trim();
    const profile = $("#term-cmd-profile")?.value || "Todos";
    const onlyQuick = $("#term-cmd-quick-only")?.checked;
    const onlyFav = $("#term-cmd-fav-only")?.checked;
    host.innerHTML = "";
    let matchCount = 0;
    const quickSeen = new Set();
    const quickItems = [];

    const matches = (section, item) => {
      if (onlyQuick && !item.quick) return false;
      if (onlyFav && !favoritesMap[item.cmd]) return false;
      if (profile !== "Todos" && item.profile !== profile) return false;
      const hay = `${section.title} ${item.label} ${item.cmd}`.toLowerCase();
      if (filter && !hay.includes(filter)) return false;
      return true;
    };

    const autoOpen = !!filter;
    const frag = document.createDocumentFragment();

    for (const section of COMMAND_SECTIONS) {
      const sectionItems = section.items.filter((item) => matches(section, item));
      if (!sectionItems.length) continue;
      sectionItems.forEach((item) => {
        matchCount++;
        if (item.quick && !quickSeen.has(item.cmd)) {
          quickSeen.add(item.cmd);
          quickItems.push(item);
        }
      });
      if (!onlyQuick) {
        frag.appendChild(buildCmdSection(section.title, sectionItems, autoOpen));
      }
    }

    if (quickItems.length > 0) {
      host.appendChild(buildCmdSection(t("term.cmd.mostUsed"), quickItems, autoOpen));
    }
    host.appendChild(frag);

    if (matchCount === 0) {
      const p = document.createElement("p");
      p.className = "muted";
      p.textContent = t("term.cmd.none");
      host.appendChild(p);
    }
  }

  function closeCmdPanel() {
    const ov = $("#terminal-cmd-overlay");
    if (!ov) return;
    cmdApiOverride = null;
    if (window.CWMotion?.closeOverlay) {
      window.CWMotion.closeOverlay(ov);
      return;
    }
    ov.classList.add("hidden");
    ov.setAttribute("aria-hidden", "true");
  }

  function mountToolbar(container, api, opts = {}) {
    if (!container || !api) return;
    cmdApiOverride = api;
    statusElOverride = opts.statusEl || null;
    const toolbar = document.createElement("div");
    toolbar.className = "terminal-toolbar glass-bar-inline linux-term-toolbar";
    toolbar.innerHTML = `
      <button type="button" class="btn btn-ghost btn-sm" data-term-act="htop">${escapeHtml(t("term.btn.htop"))}</button>
      <button type="button" class="btn btn-ghost btn-sm" data-term-act="ncdu">${escapeHtml(t("term.btn.ncdu"))}</button>
      <button type="button" class="btn btn-ghost btn-sm" data-term-act="cmdlist">${escapeHtml(t("term.btn.cmdlist"))}</button>
      <span class="terminal-toolbar-spacer"></span>
      <button type="button" class="btn btn-ghost btn-sm" data-term-act="ctrlc" title="Ctrl+C">Ctrl+C</button>
      <button type="button" class="btn btn-ghost btn-sm" data-term-act="copy">${escapeHtml(t("term.btn.copy"))}</button>
      <button type="button" class="btn btn-ghost btn-sm" data-term-act="clear">${escapeHtml(t("term.btn.clear"))}</button>`;
    container.prepend(toolbar);
    const bind = (sel, fn) => toolbar.querySelector(sel)?.addEventListener("click", fn);
    bind('[data-term-act="htop"]', async () => {
      if (!ensureConnected()) return;
      if (await confirmAction(t("term.btn.htop"), CMD_HTOP, true)) {
        api.sendCmd(CMD_HTOP, true);
        showTermStatus(t("term.status.htop"));
      }
    });
    bind('[data-term-act="ncdu"]', async () => {
      if (!ensureConnected()) return;
      if (await confirmAction(t("term.btn.ncdu"), CMD_NCDU, true)) {
        api.sendCmd(CMD_NCDU, true);
        showTermStatus(t("term.status.ncdu"));
      }
    });
    bind('[data-term-act="cmdlist"]', () => openCmdDialog(api));
    bind('[data-term-act="ctrlc"]', () => {
      if (!ensureConnected()) return;
      api.ctrlC();
      showTermStatus(t("term.status.ctrlc"));
    });
    bind('[data-term-act="copy"]', async () => {
      const log = api.getLog();
      if (!log.trim()) {
        CWUI?.toast?.(t("term.err.noCopy"), "warning");
        return;
      }
      try {
        await navigator.clipboard.writeText(log);
        CWUI?.toast?.(t("term.status.copied"), "success");
      } catch {
        CWUI?.toast?.(t("term.err.copyFail"), "error");
      }
    });
    bind('[data-term-act="clear"]', () => {
      if (!ensureConnected()) return;
      api.clear();
      showTermStatus(t("term.status.cleared"));
    });
    return toolbar;
  }

  function openCmdDialog(api) {
    const pageActive = document.querySelector("#view-terminal.subview.active");
    const desktopActive = document.getElementById("app")?.classList.contains("app-desktop-mode");
    if (!pageActive && !desktopActive && !api) {
      CWUI?.toast?.(t("term.err.offline"), "warning");
      return;
    }
    if (api) cmdApiOverride = api;
    loadFavoritesMap();
    rebuildCmdResults();
    buildComposeActions();
    const ov = $("#terminal-cmd-overlay");
    if (window.CWMotion?.openOverlay) {
      window.CWMotion.openOverlay(ov);
    } else {
      ov?.classList.remove("hidden");
      ov?.setAttribute("aria-hidden", "false");
    }
    $("#term-cmd-search")?.focus();
    showTermStatus(t("term.status.cmdList"));
  }

  function bindToolbar() {
    $("#term-btn-htop")?.addEventListener("click", async () => {
      if (!ensureConnected()) return;
      if (await confirmAction(t("term.btn.htop"), CMD_HTOP, true)) {
        window.CWTerminal.sendCmd(CMD_HTOP, true);
        showTermStatus(t("term.status.htop"));
      }
    });
    $("#term-btn-ncdu")?.addEventListener("click", async () => {
      if (!ensureConnected()) return;
      if (await confirmAction(t("term.btn.ncdu"), CMD_NCDU, true)) {
        window.CWTerminal.sendCmd(CMD_NCDU, true);
        showTermStatus(t("term.status.ncdu"));
      }
    });
    $("#term-btn-cmdlist")?.addEventListener("click", () => openCmdDialog());
    $("#term-btn-ctrlc")?.addEventListener("click", () => {
      if (!ensureConnected()) return;
      window.CWTerminal.ctrlC();
      showTermStatus(t("term.status.ctrlc"));
    });
    $("#term-btn-copy")?.addEventListener("click", async () => {
      const log = window.CWTerminal.getLog();
      if (!log.trim()) {
        CWUI?.toast?.(t("term.err.noCopy"), "warning");
        return;
      }
      try {
        await navigator.clipboard.writeText(log);
        CWUI?.toast?.(t("term.status.copied"), "success");
      } catch {
        CWUI?.toast?.(t("term.err.copyFail"), "error");
      }
    });
    $("#term-btn-clear")?.addEventListener("click", () => {
      if (!ensureConnected()) return;
      window.CWTerminal.clear();
      showTermStatus(t("term.status.cleared"));
    });
    $("#terminal-cmd-close")?.addEventListener("click", closeCmdPanel);
    $("#terminal-cmd-backdrop")?.addEventListener("click", closeCmdPanel);
    document.addEventListener("keydown", (e) => {
      const mod = e.ctrlKey || e.metaKey;
      if (mod && e.key.toLowerCase() === "k") {
        const ov = $("#terminal-cmd-overlay");
        if (ov && !ov.classList.contains("hidden")) {
          e.preventDefault();
          $("#term-cmd-search")?.focus();
          return;
        }
        if (document.querySelector("#view-terminal.subview.active") || document.getElementById("app")?.classList.contains("app-desktop-mode")) {
          e.preventDefault();
          openCmdDialog(cmdApiOverride);
        }
        return;
      }
      if (e.key !== "Escape") return;
      const ov = $("#terminal-cmd-overlay");
      if (ov && !ov.classList.contains("hidden")) closeCmdPanel();
    });
    $("#term-cmd-search")?.addEventListener("input", rebuildCmdResults);
    $("#term-cmd-profile")?.addEventListener("change", rebuildCmdResults);
    $("#term-cmd-quick-only")?.addEventListener("change", rebuildCmdResults);
    $("#term-cmd-fav-only")?.addEventListener("change", rebuildCmdResults);
  }

  function init() {
    loadFavoritesMap();
    bindToolbar();
    document.addEventListener("cw-lang-change", () => window.CWI18n?.applyLabels?.());
  }

  window.CWTerminalTools = { closeCmdPanel, openCmdDialog, mountToolbar, createTermApi };

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init);
  else init();
})();
