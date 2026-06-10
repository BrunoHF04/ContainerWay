/**
 * Módulo de Backup & Rotinas de Bancos de Dados
 */
(function () {
  const $ = (sel) => document.querySelector(sel);
  const $$ = (sel) => document.querySelectorAll(sel);
  
  const state = {
    databases: [],
    routines: [],
    filter: "",
    selectedDB: null,
    activeTab: "backup", // "backup", "routine" ou "routines"
    running: false,
  };

  function tr(key, vars) {
    if (window.CWI18n?.t) return window.CWI18n.t(key, vars);
    return key;
  }

  function escapeHtml(s) {
    return String(s ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  // Inicializa o módulo
  window.loadDBBackup = async function () {
    state.selectedDB = null;
    state.running = false;
    
    if (window.CWI18n?.applyLabels) {
      window.CWI18n.applyLabels($("#view-dbbackup"));
    }
    
    resetPanelState();
    await loadDatabases();
  };

  // Reseta estado
  function resetPanelState() {
    $(".dbbackup-layout")?.classList.add("no-selection");
    $(".dbbackup-layout")?.classList.remove("dbbackup-fullscreen");
    const fsBtn = $("#btn-dbbackup-fullscreen");
    if (fsBtn) {
      fsBtn.innerHTML = `<svg class="ico" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg>`;
      fsBtn.title = "Alternar visualização em tela cheia";
    }
    $("#dbbackup-select-hint").classList.remove("hidden");
    $("#dbbackup-panel-card").classList.add("hidden");
    $("#dbbackup-logs").textContent = tr("dbbackup.logs.placeholder");
    
    // Reset inputs
    $("#dbbackup-backup-dbName").value = "";
    $("#dbbackup-backup-user").value = "";
    $("#dbbackup-backup-pass").value = "";
    $("#dbbackup-backup-path").value = "";
    
    $("#dbbackup-restore-dbName").value = "";
    $("#dbbackup-restore-user").value = "";
    $("#dbbackup-restore-pass").value = "";
    $("#dbbackup-restore-path").value = "";
    $("#dbbackup-restore-file").value = "";
    
    $("#dbbackup-routine-dbName").value = "";
    $("#dbbackup-routine-user").value = "";
    $("#dbbackup-routine-pass").value = "";
    $("#dbbackup-routine-dest").value = "";
    $("#dbbackup-routine-retention").value = "7";
    $("#dbbackup-routine-cron").value = "";
    $("#dbbackup-routine-freq").value = "daily";
    $("#dbbackup-routine-type").value = "full";
    $("#dbbackup-routine-time").value = "02:00";
    $("#dbbackup-routine-dayofweek").value = "0";
    $("#dbbackup-routine-dayofmonth").value = "1";
    
    // Reset radios
    $$('input[name="dbbackupDestType"]').forEach((r, idx) => r.checked = idx === 0);
    $$('input[name="dbrestoreSourceType"]').forEach((r, idx) => r.checked = idx === 0);
    
    syncDestVisibility("download");
    syncRestoreSourceVisibility("upload");
    syncCronVisibility("daily");
  }

  // Carrega bancos de dados detectados
  async function loadDatabases() {
    const listBody = $("#dbbackup-list");
    listBody.innerHTML = `<tr><td colspan="5" class="muted text-center">${tr("common.loading")}</td></tr>`;
    
    try {
      const res = await api("/api/backup/db/discover");
      state.databases = res.databases || [];
      renderDatabaseTable();
      $("#dbbackup-updated-at").textContent = new Date().toLocaleTimeString();
    } catch (e) {
      listBody.innerHTML = `<tr><td colspan="5" class="error text-center">${escapeHtml(e.message)}</td></tr>`;
      CWUI.toast(e.message, "error");
    }
  }

  // Renderiza tabela de bancos
  function renderDatabaseTable() {
    const listBody = $("#dbbackup-list");
    listBody.innerHTML = "";
    
    const query = state.filter.trim().toLowerCase();
    const filtered = state.databases.filter(d => {
      const name = (d.name || "").toLowerCase();
      const engine = (d.engine || "").toLowerCase();
      const src = (d.sourceType || "").toLowerCase();
      const dbsString = (d.databases || []).join(" ").toLowerCase();
      return !query || name.includes(query) || engine.includes(query) || src.includes(query) || dbsString.includes(query);
    });

    if (filtered.length === 0) {
      listBody.innerHTML = `<tr><td colspan="5" class="muted text-center">${tr("common.empty")}</td></tr>`;
      return;
    }

    filtered.forEach(d => {
      const row = document.createElement("tr");
      if (state.selectedDB && state.selectedDB.targetId === d.targetId) {
        row.classList.add("selected");
      }
      
      const sourceBadge = d.sourceType === "docker" 
        ? `<span class="badge" style="background: rgba(59, 130, 246, 0.15); color: #3b82f6; border: 1px solid rgba(59, 130, 246, 0.3);">Docker</span>`
        : `<span class="badge" style="background: rgba(16, 185, 129, 0.15); color: #10b981; border: 1px solid rgba(16, 185, 129, 0.3);">Local (Systemd)</span>`;
      
      const statusCls = (d.status === "running" || d.status === "active" || d.status == "running (healthy)") ? "running" : "stopped";

      const dbsLabel = d.databases && d.databases.length > 0
        ? `<div class="muted" style="font-size: 0.75rem; margin-top: 3px;">Bancos: ${escapeHtml(d.databases.join(', '))}</div>`
        : `<div class="muted" style="font-size: 0.75rem; margin-top: 3px; font-style: italic;">Nenhum banco lógico detectado (ou requer senha)</div>`;

      row.innerHTML = `
        <td style="font-weight: 600;"><span style="font-size: 1.1rem; margin-right: 5px;">🗄️</span>${escapeHtml(d.engine.toUpperCase())}</td>
        <td>${sourceBadge}</td>
        <td>
          <div style="word-break: break-all; font-family: monospace; font-size: 0.85rem; font-weight: 600;">${escapeHtml(d.name)}</div>
          ${dbsLabel}
        </td>
        <td><span class="vol-container-badge ${statusCls}">${escapeHtml(d.status || "unknown")}</span></td>
        <td>
          <button type="button" class="btn btn-ghost btn-sm btn-select-db" data-id="${escapeHtml(d.targetId)}">
            ${tr("dbbackup.action.manage")}
          </button>
        </td>
      `;

      row.addEventListener("click", () => {
        if (state.running) return;
        selectDatabase(d);
        $$("#dbbackup-list tr").forEach(r => r.classList.remove("selected"));
        row.classList.add("selected");
      });

      listBody.appendChild(row);
    });
  }

  // Seleção do banco de dados
  function selectDatabase(d) {
    state.selectedDB = d;
    
    $(".dbbackup-layout")?.classList.remove("no-selection");
    $("#dbbackup-select-hint").classList.add("hidden");
    $("#dbbackup-panel-card").classList.remove("hidden");
    $("#dbbackup-selected-title").textContent = tr("dbbackup.panel.selected", { name: d.name, engine: d.engine.toUpperCase() });
    
    // Pré-preenche usuário padrão
    $("#dbbackup-backup-user").value = d.defaultUser || "";
    $("#dbbackup-restore-user").value = d.defaultUser || "";
    $("#dbbackup-routine-user").value = d.defaultUser || "";
    
    // Sugere caminho no servidor
    const defaultPath = `/tmp/backup_${d.engine}_${d.name.replace(/\//g, "_")}.sql.gz`;
    $("#dbbackup-backup-path").value = defaultPath;
    
    const defaultDest = `/var/backups/${d.engine}_${d.name.replace(/\//g, "_")}`;
    $("#dbbackup-routine-dest").value = defaultDest;

    setTab("backup");
  }

  // Alternar abas do painel
  function setTab(tab) {
    if (state.running) return;
    state.activeTab = tab;
    $$(".dbbackup-tab").forEach(btn => {
      btn.classList.toggle("active", btn.dataset.tab === tab);
    });
    $$(".dbbackup-tab-content").forEach(content => {
      content.classList.toggle("active", content.id === `dbbackup-tab-${tab}`);
    });
    
    if (tab === "routines") {
      loadRoutines();
    }
  }

  // Controla campos visíveis baseados no destino
  function syncDestVisibility(val) {
    const pathField = $("#dbbackup-backup-path-container");
    if (val === "download") {
      pathField.classList.add("hidden");
    } else {
      pathField.classList.remove("hidden");
    }
  }

  // Controla campos da origem do restore
  function syncRestoreSourceVisibility(val) {
    const pathField = $("#dbbackup-restore-path-container");
    const fileField = $("#dbbackup-restore-file-container");
    if (val === "upload") {
      pathField.classList.add("hidden");
      fileField.classList.remove("hidden");
    } else {
      pathField.classList.remove("hidden");
      fileField.classList.add("hidden");
    }
  }

  // Controla campo de cron customizado e seletores amigáveis
  function syncCronVisibility(val) {
    const cronField = $("#dbbackup-routine-cron-container");
    const timeField = $("#dbbackup-routine-time-container");
    const dayOfWeekField = $("#dbbackup-routine-dayofweek-container");
    const dayOfMonthField = $("#dbbackup-routine-dayofmonth-container");

    // Oculta tudo por padrão
    cronField?.classList.add("hidden");
    timeField?.classList.add("hidden");
    dayOfWeekField?.classList.add("hidden");
    dayOfMonthField?.classList.add("hidden");

    if (val === "custom") {
      cronField?.classList.remove("hidden");
    } else if (val === "daily") {
      timeField?.classList.remove("hidden");
    } else if (val === "weekly") {
      timeField?.classList.remove("hidden");
      dayOfWeekField?.classList.remove("hidden");
    } else if (val === "monthly") {
      timeField?.classList.remove("hidden");
      dayOfMonthField?.classList.remove("hidden");
    }
  }

  // Carrega as rotinas agendadas
  async function loadRoutines() {
    const listBody = $("#dbbackup-routines-list");
    listBody.innerHTML = `<tr><td colspan="6" class="muted text-center">${tr("common.loading")}</td></tr>`;
    
    try {
      const res = await api("/api/backup/db/routines");
      state.routines = res.routines || [];
      renderRoutinesTable();
    } catch (e) {
      listBody.innerHTML = `<tr><td colspan="6" class="error text-center">${escapeHtml(e.message)}</td></tr>`;
      CWUI.toast(e.message, "error");
    }
  }

  // Renderiza a tabela de rotinas
  function renderRoutinesTable() {
    const listBody = $("#dbbackup-routines-list");
    listBody.innerHTML = "";

    if (state.routines.length === 0) {
      listBody.innerHTML = `<tr><td colspan="6" class="muted text-center">${tr("dbbackup.routines.empty")}</td></tr>`;
      return;
    }

    state.routines.forEach(r => {
      const row = document.createElement("tr");
      
      let statusBadge = "";
      if (r.lastStatus === "success") {
        statusBadge = `<span class="badge" style="background: rgba(16, 185, 129, 0.15); color: #10b981; border: 1px solid rgba(16, 185, 129, 0.3);">Sucesso</span>`;
      } else if (r.lastStatus === "error") {
        statusBadge = `<span class="badge" style="background: rgba(239, 68, 68, 0.15); color: #ef4444; border: 1px solid rgba(239, 68, 68, 0.3);">Falha</span>`;
      } else if (r.lastStatus === "running") {
        statusBadge = `<span class="badge badge-pulse" style="background: rgba(245, 158, 11, 0.15); color: #f59e0b; border: 1px solid rgba(245, 158, 11, 0.3);">Rodando</span>`;
      } else {
        statusBadge = `<span class="badge" style="background: rgba(255, 255, 255, 0.1); color: rgba(255, 255, 255, 0.6); border: 1px solid rgba(255, 255, 255, 0.2);">Pendente</span>`;
      }

      const dbLabel = r.db ? r.db : "(Todos os bancos)";
      const retentionLabel = r.retention > 0 ? `${r.retention} dias` : "Sem limite";

      const modeLabel = r.backupMode === "incremental" ? "Incremental" : "Integral";

      row.innerHTML = `
        <td style="font-weight: 500; font-family: monospace; font-size: 0.8rem;">${escapeHtml(r.cron)}</td>
        <td style="font-weight: 600;"><span style="font-size: 1rem; margin-right: 5px;">🗄️</span>${escapeHtml(r.engine.toUpperCase())} / ${escapeHtml(r.type.toUpperCase())}<br/><small class="muted">${escapeHtml(modeLabel)}</small></td>
        <td style="word-break: break-all; font-family: monospace; font-size: 0.8rem;">${escapeHtml(dbLabel)}</td>
        <td style="word-break: break-all; font-family: monospace; font-size: 0.75rem;">${escapeHtml(r.dest)}<br/><small class="muted">Retenção: ${retentionLabel}</small></td>
        <td>
          <span style="font-family: monospace; font-size: 0.75rem;">${escapeHtml(r.lastRun)}</span><br/>
          ${statusBadge}
        </td>
        <td>
          <button type="button" class="btn btn-ghost btn-sm btn-danger btn-remove-routine" data-id="${escapeHtml(r.id)}">
            ${tr("common.remove")}
          </button>
        </td>
      `;

      row.querySelector(".btn-remove-routine").addEventListener("click", async (e) => {
        e.stopPropagation();
        const confirmed = await CWConfirm(
          `Tem certeza de que deseja remover esta rotina de backup agendada?\n\nTarefa Cron: ${r.cron}\nBanco: ${r.engine.toUpperCase()} (${dbLabel})`,
          { type: "danger" }
        );
        if (!confirmed) return;

        try {
          await api(`/api/backup/db/routines?id=${encodeURIComponent(r.id)}`, { method: "DELETE" });
          CWUI.toast(tr("dbbackup.toast.routineRemoveSuccess"), "success");
          await loadRoutines();
        } catch (err) {
          CWUI.toast(err.message, "error");
        }
      });

      listBody.appendChild(row);
    });
  }

  // Escreve logs na tela
  function appendLog(msg) {
    const consoleLogs = $("#dbbackup-logs");
    consoleLogs.textContent += msg + "\n";
    consoleLogs.scrollTop = consoleLogs.scrollHeight;
  }

  // Habilita/Desabilita inputs durantes operações
  function toggleInputs(disabled) {
    state.running = disabled;
    $("#dbbackup-refresh").disabled = disabled;
    $$(".dbbackup-tab").forEach(b => b.disabled = disabled);
    
    // Form backup
    $("#dbbackup-backup-dbName").disabled = disabled;
    $("#dbbackup-backup-user").disabled = disabled;
    $("#dbbackup-backup-pass").disabled = disabled;
    $("#dbbackup-backup-path").disabled = disabled;
    $$('input[name="dbbackupDestType"]').forEach(r => r.disabled = disabled);
    $("#btn-run-dbbackup").disabled = disabled;

    // Form restore
    $("#dbbackup-restore-dbName").disabled = disabled;
    $("#dbbackup-restore-user").disabled = disabled;
    $("#dbbackup-restore-pass").disabled = disabled;
    $("#dbbackup-restore-path").disabled = disabled;
    $("#dbbackup-restore-file").disabled = disabled;
    $$('input[name="dbrestoreSourceType"]').forEach(r => r.disabled = disabled);
    $("#btn-run-dbrestore").disabled = disabled;
    
    // Form rotina
    $("#dbbackup-routine-dbName").disabled = disabled;
    $("#dbbackup-routine-user").disabled = disabled;
    $("#dbbackup-routine-pass").disabled = disabled;
    $("#dbbackup-routine-dest").disabled = disabled;
    $("#dbbackup-routine-retention").disabled = disabled;
    $("#dbbackup-routine-cron").disabled = disabled;
    $("#dbbackup-routine-freq").disabled = disabled;
    $("#dbbackup-routine-time").disabled = disabled;
    $("#dbbackup-routine-dayofweek").disabled = disabled;
    $("#dbbackup-routine-dayofmonth").disabled = disabled;
    $("#btn-create-routine").disabled = disabled;
  }

  // Configura Listeners de evento
  function setupListeners() {
    // Popula dia do mês se estiver vazio
    const dayOfMonthSelect = $("#dbbackup-routine-dayofmonth");
    if (dayOfMonthSelect && dayOfMonthSelect.options.length === 0) {
      for (let i = 1; i <= 28; i++) {
        const opt = document.createElement("option");
        opt.value = i.toString();
        opt.textContent = i.toString();
        if (i === 1) opt.selected = true;
        dayOfMonthSelect.appendChild(opt);
      }
    }

    // Filtro de pesquisa
    $("#dbbackup-filter")?.addEventListener("input", (e) => {
      state.filter = e.target.value;
      renderDatabaseTable();
    });

    // Atualizar tabela
    $("#dbbackup-refresh")?.addEventListener("click", () => {
      if (state.running) return;
      loadDatabases();
    });

    // Abas
    $$(".dbbackup-tab").forEach(btn => {
      btn.addEventListener("click", () => setTab(btn.dataset.tab));
    });

    // Radio destType (backup)
    $$('input[name="dbbackupDestType"]').forEach(r => {
      r.addEventListener("change", (e) => syncDestVisibility(e.target.value));
    });

    // Radio destType (restore)
    $$('input[name="dbrestoreSourceType"]').forEach(r => {
      r.addEventListener("change", (e) => syncRestoreSourceVisibility(e.target.value));
    });

    // Frequência cron
    $("#dbbackup-routine-freq")?.addEventListener("change", (e) => syncCronVisibility(e.target.value));

    // Submit Backup Form
    $("#dbbackup-form-backup")?.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (state.running || !state.selectedDB) return;

      const destType = $('input[name="dbbackupDestType"]:checked').value;
      const dbName = $("#dbbackup-backup-dbName").value.trim();
      const user = $("#dbbackup-backup-user").value.trim();
      const pass = $("#dbbackup-backup-pass").value;
      const path = $("#dbbackup-backup-path").value.trim();

      if (!user) {
        CWUI.toast("Por favor, informe o usuário de acesso ao banco.", "error");
        return;
      }
      if (destType === "ssh" && !path) {
        CWUI.toast("Por favor, informe o caminho de destino no servidor.", "error");
        return;
      }

      const consoleLogs = $("#dbbackup-logs");
      consoleLogs.textContent = "";
      toggleInputs(true);
      appendLog("[Log] Iniciando processo de backup integral...");

      try {
        if (destType === "download") {
          // Download via navegador
          const query = new URLSearchParams({
            engine: state.selectedDB.engine,
            type: state.selectedDB.sourceType,
            target: state.selectedDB.targetId,
            db: dbName,
            user: user,
            pass: pass,
            destType: "download"
          });
          const url = `/api/backup/db/run?${query.toString()}`;
          
          const a = document.createElement("a");
          a.href = url;
          const ext = state.selectedDB.engine === "sqlserver" ? "bak.gz" : "sql.gz";
          const dbLabel = dbName ? dbName : "all";
          a.download = `backup_${state.selectedDB.engine}_${dbLabel}_${new Date().toISOString().replace(/[:.]/g, "-")}.${ext}`;
          document.body.appendChild(a);
          a.click();
          document.body.removeChild(a);
          
          appendLog(`[✓] Download do backup integral solicitado no navegador.`);
          CWUI.toast(tr("dbbackup.toast.backupSuccess", { name: state.selectedDB.name }), "success");
        } else {
          // Salvar no servidor (SSH)
          appendLog(`[Log] Executando comando de backup no servidor remoto...`);
          const res = await api("/api/backup/db/run", {
            method: "POST",
            body: JSON.stringify({
              engine: state.selectedDB.engine,
              type: state.selectedDB.sourceType,
              target: state.selectedDB.targetId,
              db: dbName,
              user: user,
              pass: pass,
              destType: "ssh",
              destPath: path
            })
          });
          appendLog(`[✓] Backup gravado com sucesso!`);
          appendLog(`[Info] Caminho: ${path}`);
          appendLog(`[Info] Tamanho: ${res.sizeHuman} (${res.bytesWritten} bytes)`);
          CWUI.toast(tr("dbbackup.toast.backupSuccess", { name: state.selectedDB.name }), "success");
        }
      } catch (err) {
        appendLog(`[ERRO] ${err.message}`);
        CWUI.toast(err.message, "error");
      } finally {
        toggleInputs(false);
      }
    });

    // Submit Restore Form
    $("#dbbackup-form-restore")?.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (state.running || !state.selectedDB) return;

      const srcType = $('input[name="dbrestoreSourceType"]:checked').value;
      const dbName = $("#dbbackup-restore-dbName").value.trim();
      const user = $("#dbbackup-restore-user").value.trim();
      const pass = $("#dbbackup-restore-pass").value;
      const path = $("#dbbackup-restore-path").value.trim();
      const fileInput = $("#dbbackup-restore-file");

      if (!dbName) {
        CWUI.toast("Por favor, informe o banco de dados de destino.", "error");
        return;
      }
      if (!user) {
        CWUI.toast("Por favor, informe o usuário de acesso ao banco.", "error");
        return;
      }
      if (srcType === "ssh" && !path) {
        CWUI.toast("Por favor, informe o caminho do arquivo de backup no servidor.", "error");
        return;
      }
      if (srcType === "upload" && (!fileInput.files || fileInput.files.length === 0)) {
        CWUI.toast("Por favor, selecione um arquivo de backup para upload.", "error");
        return;
      }

      const confirmed = await CWConfirm(
        tr("dbbackup.confirm.restore", { name: dbName }),
        { type: "danger" }
      );
      if (!confirmed) return;

      const consoleLogs = $("#dbbackup-logs");
      consoleLogs.textContent = "";
      toggleInputs(true);
      appendLog("[Log] Iniciando processo de restauração do banco...");
      CWUI.toast(tr("dbbackup.toast.restoreStart", { name: dbName }), "info");

      try {
        if (srcType === "upload") {
          appendLog("[Log] Enviando arquivo de backup para o servidor remoto...");
          const formData = new FormData();
          formData.append("engine", state.selectedDB.engine);
          formData.append("type", state.selectedDB.sourceType);
          formData.append("target", state.selectedDB.targetId);
          formData.append("db", dbName);
          formData.append("user", user);
          formData.append("pass", pass);
          formData.append("sourceType", "upload");
          formData.append("file", fileInput.files[0]);

          const res = await fetch("/api/backup/db/restore", {
            method: "POST",
            body: formData
          });
          const resJson = await res.json();
          if (!res.ok || resJson.error) {
            throw new Error(resJson.error || `Erro HTTP ${res.status}`);
          }
          appendLog("[✓] Arquivo transmitido e restauração executada com sucesso!");
          CWUI.toast(tr("dbbackup.toast.restoreSuccess", { name: dbName }), "success");
        } else {
          appendLog("[Log] Solicitando restauração a partir do arquivo remoto no servidor...");
          await api("/api/backup/db/restore", {
            method: "POST",
            body: JSON.stringify({
              engine: state.selectedDB.engine,
              type: state.selectedDB.sourceType,
              target: state.selectedDB.targetId,
              db: dbName,
              user: user,
              pass: pass,
              sourceType: "ssh",
              sourcePath: path
            })
          });
          appendLog("[✓] Restauração executada com sucesso!");
          CWUI.toast(tr("dbbackup.toast.restoreSuccess", { name: dbName }), "success");
        }
      } catch (err) {
        appendLog(`[ERRO] ${err.message}`);
        CWUI.toast(err.message, "error");
      } finally {
        toggleInputs(false);
      }
    });

    // Submit Routine Form
    $("#dbbackup-form-routine")?.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (state.running || !state.selectedDB) return;

      const dbName = $("#dbbackup-routine-dbName").value.trim();
      const user = $("#dbbackup-routine-user").value.trim();
      const pass = $("#dbbackup-routine-pass").value;
      const dest = $("#dbbackup-routine-dest").value.trim();
      const retentionVal = parseInt($("#dbbackup-routine-retention").value, 10);
      const freq = $("#dbbackup-routine-freq").value;
      const backupMode = $("#dbbackup-routine-type").value;
      let cronExpr = "";

      // Extrai hora e minuto do seletor amigável
      const timeVal = $("#dbbackup-routine-time")?.value || "02:00";
      const parts = timeVal.split(":");
      const hour = parts[0] ? parseInt(parts[0], 10) : 2;
      const min = parts[1] ? parseInt(parts[1], 10) : 0;
      const cronMin = isNaN(min) ? "0" : min.toString();
      const cronHour = isNaN(hour) ? "2" : hour.toString();

      switch (freq) {
        case "min15":
          cronExpr = "*/15 * * * *";
          break;
        case "min30":
          cronExpr = "*/30 * * * *";
          break;
        case "hourly":
          cronExpr = "0 * * * *";
          break;
        case "daily":
          cronExpr = `${cronMin} ${cronHour} * * *`;
          break;
        case "weekly":
          const dayOfWeek = $("#dbbackup-routine-dayofweek")?.value || "0";
          cronExpr = `${cronMin} ${cronHour} * * ${dayOfWeek}`;
          break;
        case "monthly":
          const dayOfMonth = $("#dbbackup-routine-dayofmonth")?.value || "1";
          cronExpr = `${cronMin} ${cronHour} ${dayOfMonth} * *`;
          break;
        case "custom":
          cronExpr = $("#dbbackup-routine-cron").value.trim();
          break;
      }

      if (!user) {
        CWUI.toast("Por favor, informe o usuário do banco.", "error");
        return;
      }
      if (!dest) {
        CWUI.toast("Por favor, informe a pasta de destino no servidor.", "error");
        return;
      }
      if (!cronExpr) {
        CWUI.toast("Por favor, informe uma expressão cron válida.", "error");
        return;
      }

      const consoleLogs = $("#dbbackup-logs");
      consoleLogs.textContent = "";
      toggleInputs(true);
      appendLog("[Log] Configurando rotina de backup no servidor remoto...");

      try {
        const res = await api("/api/backup/db/routines", {
          method: "POST",
          body: JSON.stringify({
            cron: cronExpr,
            engine: state.selectedDB.engine,
            type: state.selectedDB.sourceType,
            target: state.selectedDB.targetId,
            db: dbName,
            user: user,
            pass: pass,
            dest: dest,
            retention: isNaN(retentionVal) ? 0 : retentionVal,
            backupMode: backupMode
          })
        });

        appendLog(`[✓] Rotina de backup "${res.id}" configurada com sucesso no crontab!`);
        appendLog(`[Info] Expressão Cron: ${cronExpr}`);
        appendLog(`[Info] Modo: ${backupMode}`);
        appendLog(`[Info] Pasta Destino: ${dest}`);
        appendLog(`[Info] Retenção: ${retentionVal > 0 ? retentionVal + ' dias' : 'sem limite'}`);
        CWUI.toast(tr("dbbackup.toast.routineSuccess"), "success");
        
        // Vai para a aba de rotinas para ver
        setTab("routines");
      } catch (err) {
        appendLog(`[ERRO] ${err.message}`);
        CWUI.toast(err.message, "error");
      } finally {
        toggleInputs(false);
      }
    });

    $("#btn-dbbackup-fullscreen")?.addEventListener("click", () => {
      const layout = $(".dbbackup-layout");
      const btn = $("#btn-dbbackup-fullscreen");
      if (!layout || !btn) return;
      layout.classList.toggle("dbbackup-fullscreen");
      const isFS = layout.classList.contains("dbbackup-fullscreen");
      if (isFS) {
        btn.innerHTML = `<svg class="ico" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 14h6v6M20 10h-6V4M14 10l7-7M10 14l-7 7"/></svg>`;
        btn.title = "Recolher tela cheia";
      } else {
        btn.innerHTML = `<svg class="ico" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg>`;
        btn.title = "Alternar visualização em tela cheia";
      }
    });

    setupAutocomplete();
  }

  // Configura Autocomplete customizado para inputs de bancos
  function setupAutocomplete() {
    const inputs = [
      $("#dbbackup-backup-dbName"),
      $("#dbbackup-restore-dbName"),
      $("#dbbackup-routine-dbName")
    ];
    const listContainer = $("#db-autocomplete-list");
    let focusedIndex = -1;
    let activeInput = null;

    if (!listContainer) return;

    // Renderiza a lista no dropdown com base no filtro
    function renderDropdown(inputEl) {
      if (!state.selectedDB || !state.selectedDB.databases || state.selectedDB.databases.length === 0) {
        listContainer.classList.add("hidden");
        return;
      }

      const val = inputEl.value.trim().toLowerCase();
      const filtered = state.selectedDB.databases.filter(db => db.toLowerCase().includes(val));

      listContainer.innerHTML = "";
      focusedIndex = -1;

      if (filtered.length === 0) {
        const noRes = document.createElement("div");
        noRes.className = "db-autocomplete-no-results";
        noRes.textContent = tr("common.empty") || "Nenhum banco correspondente";
        listContainer.appendChild(noRes);
      } else {
        filtered.forEach((db, idx) => {
          const item = document.createElement("div");
          item.className = "db-autocomplete-item";
          item.textContent = db;
          item.dataset.index = idx;
          item.dataset.value = db;

          // Mousedown em vez de click para disparar antes do blur do input
          item.addEventListener("mousedown", (e) => {
            e.preventDefault(); // Impede o blur do input
            selectItem(db);
          });

          listContainer.appendChild(item);
        });
      }

      // Move o dropdown para o parent do input atual
      if (listContainer.parentElement !== inputEl.parentElement) {
        inputEl.parentElement.appendChild(listContainer);
      }
      listContainer.classList.remove("hidden");
    }

    // Seleciona o item e atualiza o input
    function selectItem(val) {
      if (activeInput) {
        activeInput.value = val;
        // Dispara eventos change e input
        activeInput.dispatchEvent(new Event("change"));
        activeInput.dispatchEvent(new Event("input"));
      }
      hideDropdown();
    }

    // Oculta o dropdown
    function hideDropdown() {
      listContainer.classList.add("hidden");
      focusedIndex = -1;
    }

    // Atualiza o destaque visual (foco) das opções
    function updateFocus() {
      const items = listContainer.querySelectorAll(".db-autocomplete-item");
      items.forEach((item, idx) => {
        if (idx === focusedIndex) {
          item.classList.add("is-focused");
          // Rolagem automática para manter visível
          item.scrollIntoView({ block: "nearest" });
        } else {
          item.classList.remove("is-focused");
        }
      });
    }

    // Adiciona listeners para cada input
    inputs.forEach(input => {
      if (!input) return;

      input.addEventListener("focus", () => {
        activeInput = input;
        renderDropdown(input);
      });

      input.addEventListener("input", () => {
        activeInput = input;
        renderDropdown(input);
      });

      input.addEventListener("keydown", (e) => {
        if (listContainer.classList.contains("hidden")) {
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            renderDropdown(input);
          }
          return;
        }

        const items = listContainer.querySelectorAll(".db-autocomplete-item");
        if (items.length === 0) return;

        if (e.key === "ArrowDown") {
          e.preventDefault();
          focusedIndex = (focusedIndex + 1) % items.length;
          updateFocus();
        } else if (e.key === "ArrowUp") {
          e.preventDefault();
          focusedIndex = (focusedIndex - 1 + items.length) % items.length;
          updateFocus();
        } else if (e.key === "Enter") {
          e.preventDefault();
          if (focusedIndex >= 0 && focusedIndex < items.length) {
            selectItem(items[focusedIndex].dataset.value);
          } else if (items.length === 1) {
            selectItem(items[0].dataset.value);
          } else {
            hideDropdown();
          }
        } else if (e.key === "Escape") {
          e.preventDefault();
          hideDropdown();
          input.blur();
        }
      });

      input.addEventListener("blur", () => {
        // Timeout curto para mousedown/click ser processado
        setTimeout(() => {
          if (activeInput === input) {
            hideDropdown();
          }
        }, 150);
      });
    });

    // Fecha o dropdown se clicar fora
    document.addEventListener("mousedown", (e) => {
      if (!e.target.closest(".db-autocomplete-wrapper")) {
        hideDropdown();
      }
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", setupListeners);
  } else {
    setupListeners();
  }
})();
