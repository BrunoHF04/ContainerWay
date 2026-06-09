/**
 * Módulo de Backup & Restauração de Volumes Docker
 */
(function () {
  const $ = (sel) => document.querySelector(sel);
  const $$ = (sel) => document.querySelectorAll(sel);
  
  const state = {
    volumes: [],
    filter: "",
    selectedVolume: null,
    activeTab: "backup", // "backup" ou "restore"
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

  // Inicializa o módulo quando a tela é acessada
  window.loadVolBackup = async function () {
    state.selectedVolume = null;
    state.running = false;
    
    // Atualizar UI estática com i18n
    if (window.CWI18n?.applyLabels) {
      window.CWI18n.applyLabels($("#view-volbackup"));
    }
    
    resetPanelState();
    await loadVolumes();
  };

  // Reseta o formulário e logs
  function resetPanelState() {
    $("#volbackup-select-hint").classList.remove("hidden");
    $("#volbackup-panel-card").classList.add("hidden");
    $("#volbackup-logs").textContent = tr("volbackup.logs.placeholder");
    
    // Reset inputs
    $("#volbackup-backup-path").value = "";
    $("#volbackup-restore-path").value = "";
    $("#volbackup-restore-file").value = "";
    
    // Reset radios
    $$('input[name="backupDestType"]').forEach((r, idx) => r.checked = idx === 0);
    $$('input[name="restoreSourceType"]').forEach((r, idx) => r.checked = idx === 0);
    
    syncDestVisibility("download");
    syncSourceVisibility("upload");
  }

  // Carrega volumes via API
  async function loadVolumes() {
    const listBody = $("#volbackup-list");
    listBody.innerHTML = `<tr><td colspan="4" class="muted text-center">${tr("common.loading")}</td></tr>`;
    
    try {
      // Usa endpoint existente do docker manager para obter a lista detalhada
      const res = await api("/api/docker/volumes");
      state.volumes = res.volumes || [];
      renderVolumeTable();
      $("#volbackup-updated-at").textContent = new Date().toLocaleTimeString();
    } catch (e) {
      listBody.innerHTML = `<tr><td colspan="4" class="error text-center">${escapeHtml(e.message)}</td></tr>`;
      CWUI.toast(e.message, "error");
    }
  }

  // Renderiza a tabela de volumes
  function renderVolumeTable() {
    const listBody = $("#volbackup-list");
    listBody.innerHTML = "";
    
    const query = state.filter.trim().toLowerCase();
    const filtered = state.volumes.filter(v => {
      const name = (v.name || "").toLowerCase();
      const driver = (v.driver || "").toLowerCase();
      return !query || name.includes(query) || driver.includes(query);
    });

    if (filtered.length === 0) {
      listBody.innerHTML = `<tr><td colspan="4" class="muted text-center">${tr("common.empty")}</td></tr>`;
      return;
    }

    filtered.forEach(v => {
      const row = document.createElement("tr");
      if (state.selectedVolume && state.selectedVolume.name === v.name) {
        row.classList.add("selected");
      }
      
      // Monta badges dos contêineres associados
      let containerBadges = "";
      if (v.containers && v.containers.length > 0) {
        containerBadges = v.containers.map(c => {
          const stateCls = c.running ? "running" : "stopped";
          const namePart = c.displayName || "Contêiner";
          const idPart = c.id ? ` <small style="opacity: 0.7; font-size: 0.7rem; font-family: monospace;">(${c.id})</small>` : "";
          const display = `${escapeHtml(namePart)}${idPart}`;
          const stateLabel = c.running ? "running" : "stopped";
          return `<span class="vol-container-badge ${stateCls}" title="Estado: ${stateLabel}">${display}</span>`;
        }).join(" ");
      } else {
        containerBadges = `<span class="muted">—</span>`;
      }

      row.innerHTML = `
        <td style="font-weight: 500; word-break: break-all;">${escapeHtml(v.name)}</td>
        <td><span class="badge badge-outline">${escapeHtml(v.driver)}</span></td>
        <td>${containerBadges}</td>
        <td>
          <button type="button" class="btn btn-ghost btn-sm btn-select-vol" data-vol="${escapeHtml(v.name)}">
            ${tr("volbackup.action.backup")} / ${tr("volbackup.action.restore")}
          </button>
        </td>
      `;

      // Seleção da linha completa
      row.addEventListener("click", (e) => {
        if (state.running) return;
        selectVolume(v);
        $$("#volbackup-list tr").forEach(r => r.classList.remove("selected"));
        row.classList.add("selected");
      });

      listBody.appendChild(row);
    });
  }

  // Trata seleção de volume
  function selectVolume(v) {
    state.selectedVolume = v;
    
    // Atualizar UI
    $("#volbackup-select-hint").classList.add("hidden");
    $("#volbackup-panel-card").classList.remove("hidden");
    $("#volbackup-selected-title").textContent = tr("volbackup.panel.selected", { name: v.name });
    
    // Reset tabs
    setTab("backup");
  }

  // Alterna entre abas
  function setTab(tab) {
    if (state.running) return;
    state.activeTab = tab;
    $$(".volbackup-tab").forEach(btn => {
      const active = btn.dataset.tab === tab;
      btn.classList.toggle("active", active);
    });
    $$(".volbackup-tab-content").forEach(content => {
      const active = content.id === `volbackup-tab-${tab}`;
      content.classList.toggle("active", active);
    });
  }

  // Controla campos dinâmicos de Backup e Restauração
  function syncDestVisibility(val) {
    const pathField = $("#volbackup-backup-path-container");
    if (val === "download") {
      pathField.classList.add("hidden");
    } else {
      pathField.classList.remove("hidden");
      $("#volbackup-backup-path").placeholder = "/tmp/backup.tar";
    }
  }

  function syncSourceVisibility(val) {
    const pathField = $("#volbackup-restore-path-container");
    const fileField = $("#volbackup-restore-file-container");
    if (val === "upload") {
      pathField.classList.add("hidden");
      fileField.classList.remove("hidden");
    } else {
      pathField.classList.remove("hidden");
      fileField.classList.add("hidden");
      $("#volbackup-restore-path").placeholder = "/tmp/backup.tar";
    }
  }

  // Executa o streaming de logs da API e preenche o console da UI
  async function streamOperationLogs(url, method = "GET", body = null, isUpload = false) {
    const consoleLogs = $("#volbackup-logs");
    consoleLogs.textContent = "";
    state.running = true;
    toggleActionButtons(true);
    
    try {
      const fetchOpts = {
        method,
        credentials: "same-origin",
      };
      if (body) {
        fetchOpts.body = body;
        // Não definir Content-Type se for multipart/form-data (upload),
        // o navegador irá configurar automaticamente com os boundaries corretos.
        if (!isUpload) {
          fetchOpts.headers = { "Content-Type": "application/json" };
        }
      }

      const res = await fetch(url, fetchOpts);
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || res.statusText || "Erro desconhecido");
      }

      const reader = res.body?.getReader();
      if (!reader) {
        throw new Error("Stream de logs indisponível.");
      }

      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        let nlIndex;
        while ((nlIndex = buffer.indexOf("\n")) >= 0) {
          const line = buffer.slice(0, nlIndex).trim();
          buffer = buffer.slice(nlIndex + 1);
          if (!line) continue;

          try {
            const data = JSON.parse(line);
            if (data.type === "log" && data.message) {
              appendLog(data.message);
            } else if (data.type === "progress") {
              appendLog(`[Progresso] ${data.percent}%`);
            } else if (data.type === "done") {
              appendLog(data.message || "\n✓ Operação concluída com sucesso!");
              CWUI.toast(
                state.activeTab === "backup"
                  ? tr("volbackup.toast.backupSuccess", { name: state.selectedVolume.name })
                  : tr("volbackup.toast.restoreSuccess", { name: state.selectedVolume.name }),
                "success"
              );
            } else if (data.type === "error") {
              appendLog(`[ERRO] ${data.message}`);
              CWUI.toast(data.message, "error");
            }
          } catch (err) {
            appendLog(line); // Fallback para texto plano
          }
        }
      }
    } catch (e) {
      appendLog(`[ERRO CRÍTICO] ${e.message}`);
      CWUI.toast(e.message, "error");
    } finally {
      state.running = false;
      toggleActionButtons(false);
    }
  }

  function appendLog(msg) {
    const consoleLogs = $("#volbackup-logs");
    consoleLogs.textContent += msg + "\n";
    consoleLogs.scrollTop = consoleLogs.scrollHeight;
  }

  function toggleActionButtons(disabled) {
    $("#btn-run-backup").disabled = disabled;
    $("#btn-run-restore").disabled = disabled;
    $("#volbackup-refresh").disabled = disabled;
    $$('input[name="backupDestType"]').forEach(r => r.disabled = disabled);
    $$('input[name="restoreSourceType"]').forEach(r => r.disabled = disabled);
    $("#volbackup-backup-path").disabled = disabled;
    $("#volbackup-restore-path").disabled = disabled;
    $("#volbackup-restore-file").disabled = disabled;
  }

  // Configura os Listeners na inicialização do script
  function setupListeners() {
    // Filtro de pesquisa
    $("#volbackup-filter")?.addEventListener("input", (e) => {
      state.filter = e.target.value;
      renderVolumeTable();
    });

    // Atualizar
    $("#volbackup-refresh")?.addEventListener("click", () => {
      if (state.running) return;
      loadVolumes();
    });

    // Alternar abas
    $$(".volbackup-tab").forEach(btn => {
      btn.addEventListener("click", () => setTab(btn.dataset.tab));
    });

    // Radios backup
    $$('input[name="backupDestType"]').forEach(r => {
      r.addEventListener("change", (e) => syncDestVisibility(e.target.value));
    });

    // Radios restore
    $$('input[name="restoreSourceType"]').forEach(r => {
      r.addEventListener("change", (e) => syncSourceVisibility(e.target.value));
    });

    // Submit Backup Form
    $("#volbackup-form-backup")?.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (state.running || !state.selectedVolume) return;

      const destType = $('input[name="backupDestType"]:checked').value;
      const path = $("#volbackup-backup-path").value.trim();

      if (destType !== "download" && !path) {
        CWUI.toast(tr("Por favor, informe o caminho de destino."), "error");
        return;
      }

      CWUI.toast(tr("volbackup.toast.backupStart", { name: state.selectedVolume.name }), "info");

      if (destType === "download") {
        // Fluxo de download direto via browser
        const url = `/api/docker/volumes/backup?volumeName=${encodeURIComponent(state.selectedVolume.name)}&destType=download`;
        const a = document.createElement("a");
        a.href = url;
        a.download = `volume-${state.selectedVolume.name}.tar`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        appendLog(`[Log] Download direto solicitado via navegador para o volume: ${state.selectedVolume.name}`);
        CWUI.toast(tr("volbackup.toast.backupSuccess", { name: state.selectedVolume.name }), "success");
      } else {
        // Fluxo de streaming local/SSH
        const params = new URLSearchParams({
          volumeName: state.selectedVolume.name,
          destType: destType,
          destPath: path,
        });
        await streamOperationLogs(`/api/docker/volumes/backup?${params.toString()}`);
      }
    });

    // Submit Restore Form
    $("#volbackup-form-restore")?.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (state.running || !state.selectedVolume) return;

      const srcType = $('input[name="restoreSourceType"]:checked').value;
      const path = $("#volbackup-restore-path").value.trim();
      const fileInput = $("#volbackup-restore-file");

      if (srcType === "upload" && (!fileInput.files || fileInput.files.length === 0)) {
        CWUI.toast(tr("Por favor, selecione o arquivo de backup."), "error");
        return;
      }

      if (srcType !== "upload" && !path) {
        CWUI.toast(tr("Por favor, informe o caminho de origem do backup."), "error");
        return;
      }

      // Confirmação
      const confirmed = await CWConfirm(
        tr("volbackup.confirm.restore", { name: state.selectedVolume.name }),
        { type: "warning" }
      );
      if (!confirmed) return;

      CWUI.toast(tr("volbackup.toast.restoreStart", { name: state.selectedVolume.name }), "info");

      if (srcType === "upload") {
        const formData = new FormData();
        formData.append("volumeName", state.selectedVolume.name);
        formData.append("srcType", "upload");
        formData.append("file", fileInput.files[0]);
        
        await streamOperationLogs("/api/docker/volumes/restore", "POST", formData, true);
      } else {
        const body = JSON.stringify({
          volumeName: state.selectedVolume.name,
          srcType: srcType,
          srcPath: path,
        });
        await streamOperationLogs("/api/docker/volumes/restore", "POST", body, false);
      }
    });
  }

  // Esperar o DOM carregar para vincular os listeners
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", setupListeners);
  } else {
    setupListeners();
  }
})();
