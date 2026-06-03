/** Melhorias do Ambiente Linux — transferências, sons, prompts, Docker, preview. */
(function () {
  function escapeHtml(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function hostWallKey() {
    const h = (state?.ssh?.host || "default").replace(/[^\w.-]+/g, "_");
    return `cw-desktop-wall-${h}`;
  }

  function applyWallpaperPerHost() {
    const global = window.CWWebPrefs?.get?.("desktopWallpaper") || "ubuntu";
    let wall = global;
    try {
      const per = localStorage.getItem(hostWallKey());
      if (per) wall = per;
    } catch (_) { /* */ }
    const desk = document.getElementById("linux-desktop");
    if (desk) desk.dataset.wallpaper = wall;
    document.querySelectorAll(".linux-wallpaper-btn").forEach((btn) => {
      btn.classList.toggle("is-active", btn.dataset.wall === wall);
    });
  }

  function saveWallpaperForHost(wall) {
    try {
      localStorage.setItem(hostWallKey(), wall);
    } catch (_) { /* */ }
    window.CWWebPrefs?.set?.("desktopWallpaper", wall);
    applyWallpaperPerHost();
  }

  function playSound(kind) {
    if (!window.CWWebPrefs?.get?.("desktopSounds")) return;
    try {
      const ctx = window.__cwDesktopAudioCtx || new AudioContext();
      window.__cwDesktopAudioCtx = ctx;
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.connect(gain);
      gain.connect(ctx.destination);
      const f = kind === "error" ? 220 : kind === "success" ? 880 : 660;
      osc.frequency.value = f;
      gain.gain.value = 0.04;
      osc.start();
      osc.stop(ctx.currentTime + 0.12);
    } catch (_) { /* */ }
  }

  function applyCompactMode() {
    const on = !!window.CWWebPrefs?.get?.("desktopCompact");
    document.getElementById("linux-desktop")?.classList.toggle("is-compact", on);
    document.getElementById("linux-compact-toggle")?.setAttribute("aria-pressed", on ? "true" : "false");
  }

  async function pollTransfers() {
    if (state.screen !== "desktop" || !state.ssh?.connected) return;
    const pill = document.getElementById("linux-panel-transfer");
    if (!pill) return;
    try {
      const st = await api("/api/transfer/status");
      const busy = (st.running || 0) > 0 || (st.queued || 0) > 0;
      if (busy) {
        pill.hidden = false;
        pill.textContent = `⇅ ${st.running || 0}/${st.queued || 0}`;
        pill.title = `Transferências: ${st.running} em curso, ${st.queued} na fila`;
      } else {
        pill.hidden = true;
        pill.textContent = "";
      }
    } catch {
      pill.hidden = true;
    }
  }

  function startTransferPoll() {
    stopTransferPoll();
    pollTransfers();
    window.__cwDesktopXferTimer = setInterval(pollTransfers, 2500);
  }

  function stopTransferPoll() {
    if (window.__cwDesktopXferTimer) {
      clearInterval(window.__cwDesktopXferTimer);
      window.__cwDesktopXferTimer = null;
    }
  }

  function linuxPrompt(opts) {
    const dlg = document.getElementById("linux-input-dialog");
    const form = document.getElementById("linux-input-form");
    if (!dlg?.showModal || !form) {
      const v = prompt(opts?.label || opts?.title || "Valor:");
      return Promise.resolve(v?.trim() || null);
    }
    return new Promise((resolve) => {
      document.getElementById("linux-input-title").textContent = opts.title || "Informe";
      const lab = form.querySelector("[data-input-label]");
      if (lab) lab.textContent = opts.label || "Nome";
      form.elements.value.value = opts.defaultValue || "";
      let settled = false;
      const done = (val) => {
        if (settled) return;
        settled = true;
        dlg.removeEventListener("close", onClose);
        resolve(val);
      };
      const onClose = () => done(null);
      const onSubmit = (ev) => {
        ev.preventDefault();
        const val = form.elements.value.value.trim();
        dlg.close();
        done(val || null);
      };
      form.onsubmit = onSubmit;
      document.getElementById("linux-input-cancel").onclick = () => dlg.close();
      dlg.addEventListener("close", onClose);
      dlg.showModal();
      form.elements.value.focus();
      form.elements.value.select();
    });
  }

  async function openFilePreview(path, containerId, name) {
    const dlg = document.getElementById("linux-preview-dialog");
    const body = document.getElementById("linux-preview-body");
    const title = document.getElementById("linux-preview-title");
    if (!dlg || !body) return;
    title.textContent = name || path;
    body.innerHTML = `<p class="muted">Carregando…</p>`;
    dlg.showModal();
    const cq = containerId ? `&containerId=${encodeURIComponent(containerId)}` : "";
    try {
      const data = await api(`/api/remote/file?path=${encodeURIComponent(path)}${cq}`);
      const mime = data.mimeType || "";
      if (data.encoding === "base64" && mime.startsWith("image/")) {
        body.innerHTML = `<img class="linux-preview-img" src="data:${mime};base64,${data.content}" alt="" />`;
        return;
      }
      if (data.encoding !== "base64" && typeof data.content === "string") {
        const text = data.content.length > 120000 ? data.content.slice(0, 120000) + "\n…" : data.content;
        body.innerHTML = `<pre class="linux-preview-text">${escapeHtml(text)}</pre>`;
        return;
      }
      body.innerHTML = `<p class="muted">Pré-visualização não disponível para este tipo de arquivo.</p>`;
    } catch (e) {
      body.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
    }
  }

  function createDockerAutoRule(containerName) {
    const target = (containerName || "").replace(/^\//, "");
    if (!target) return;
    sessionStorage.setItem(
      "cw-docker-auto-prefill",
      JSON.stringify({
        target,
        name: `Reinício automático: ${target}`,
      })
    );
    window.CWDesktop?.openApp?.("automations");
    CWUI.toast("Abra a regra, confira e clique em Salvar regras.", "info");
  }

  function applyDockerAutomationPrefill(rules, setDirty, renderRules, openRuleDialog) {
    const raw = sessionStorage.getItem("cw-docker-auto-prefill");
    if (!raw) return false;
    sessionStorage.removeItem("cw-docker-auto-prefill");
    let pre;
    try {
      pre = JSON.parse(raw);
    } catch {
      return false;
    }
    const target = String(pre.target || "").trim();
    if (!target) return false;
    const existing = rules.findIndex(
      (r) => r.kind === "docker_container_stopped_restart" && r.target === target
    );
    if (existing >= 0) {
      openRuleDialog(existing);
      CWUI.toast("Regra existente para este container — edite e salve.", "info");
      return true;
    }
    const rule = {
      id: `docker-${Date.now()}`,
      kind: "docker_container_stopped_restart",
      name: pre.name || `Reinício automático: ${target}`,
      description: "Criada a partir do Docker no Ambiente Linux",
      trigger: "Container Docker parou de rodar",
      action: "docker restart",
      target,
      cooldownSec: 60,
      enabled: true,
      webhookURL: "",
    };
    rules.push(rule);
    setDirty(true);
    renderRules();
    openRuleDialog(rules.length - 1);
    CWUI.toast("Nova regra — confirme e clique em Salvar regras.", "info");
    return true;
  }

  function renderDockerStats(el, samples) {
    if (!el || !samples?.length) {
      el.innerHTML = "";
      return;
    }
    const last = samples.slice(-24);
    const maxCpu = Math.max(1, ...last.map((s) => s.cpu || 0));
    const maxMem = Math.max(1, ...last.map((s) => s.mem || 0));
    const bar = (vals, max, cls) =>
      vals
        .map((v) => {
          const h = Math.max(4, Math.round((v / max) * 36));
          return `<span class="linux-spark ${cls}" style="height:${h}px" title="${v.toFixed(0)}%"></span>`;
        })
        .join("");
    el.innerHTML = `
      <div class="linux-spark-wrap">
        <span class="muted">CPU</span>
        <div class="linux-spark-row">${bar(last.map((s) => s.cpu || 0), maxCpu, "is-cpu")}</div>
        <span class="muted">RAM</span>
        <div class="linux-spark-row">${bar(last.map((s) => s.mem || 0), maxMem, "is-mem")}</div>
      </div>`;
  }

  function bindExtras() {
    document.getElementById("linux-wallpaper-picks")?.addEventListener("click", (ev) => {
      const btn = ev.target.closest(".linux-wallpaper-btn");
      if (!btn) return;
      saveWallpaperForHost(btn.dataset.wall || "ubuntu");
    });

    document.getElementById("linux-start-close-all")?.addEventListener("click", () => {
      window.CWDesktop?.closeAllWindows?.();
      window.CWDesktop?.closeStartMenu?.();
    });

    document.getElementById("linux-compact-toggle")?.addEventListener("click", () => {
      const on = !window.CWWebPrefs?.get?.("desktopCompact");
      window.CWWebPrefs?.set?.("desktopCompact", on);
      applyCompactMode();
    });

    document.getElementById("linux-sound-toggle")?.addEventListener("click", () => {
      const on = !window.CWWebPrefs?.get?.("desktopSounds");
      window.CWWebPrefs?.set?.("desktopSounds", on);
      document.getElementById("linux-sound-toggle")?.setAttribute("aria-pressed", on ? "true" : "false");
      if (on) playSound("info");
    });

    document.getElementById("linux-preview-close")?.addEventListener("click", () => {
      document.getElementById("linux-preview-dialog")?.close();
    });

    const origPush = window.CWDesktopCore?.pushNotif;
    if (origPush && !window.__cwDesktopSoundHooked) {
      window.__cwDesktopSoundHooked = true;
      window.CWDesktopCore.pushNotif = function (msg, level, meta) {
        if (level === "error" || level === "success") playSound(level);
        return origPush(msg, level, meta);
      };
    }
  }

  bindExtras();

  window.CWDesktopEnhancements = {
    linuxPrompt,
    openFilePreview,
    createDockerAutoRule,
    applyDockerAutomationPrefill,
    renderDockerStats,
    applyWallpaperPerHost,
    applyCompactMode,
    startTransferPoll,
    stopTransferPoll,
    playSound,
  };
})();
