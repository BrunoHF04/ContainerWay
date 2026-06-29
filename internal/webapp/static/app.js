const state = {
  user: null,
  permissions: { screens: [], actions: [] },
  ssh: { connected: false, host: "", user: "", isAdmin: false },
  screen: "connect",
  localPath: "",
  remotePath: "/",
  selLocal: null,
  selRemote: null,
  term: null,
  termFit: null,
  termResizeBound: false,
  termSocket: null,
  termLog: "",
  autoRules: [],
  autoRulesDirty: false,
  autoEditIndex: -1,
  autoPollTimer: null,
  transferLogPinned: false,
};

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => document.querySelectorAll(sel);

const THEME_KEY = "cw-web-theme";

function mascotIconSrc(theme) {
  return theme === "light" ? "/icon-192-light.png" : "/icon-192.png";
}

function mascotHeroSrc(theme) {
  return theme === "light" ? "/mascot-hero-light.png" : "/mascot-hero.png";
}

function syncMascotIcons(theme) {
  const src = mascotIconSrc(theme);
  const hero = mascotHeroSrc(theme);
  for (const img of document.querySelectorAll(".logo, .splash-logo")) {
    img.src = src;
  }
  for (const img of document.querySelectorAll(".hub-mascot-img")) {
    img.src = hero;
  }
}

function preloadMascotAssets() {
  for (const theme of ["light", "dark"]) {
    const icon = new Image();
    icon.src = mascotIconSrc(theme);
    const hero = new Image();
    hero.src = mascotHeroSrc(theme);
  }
}

function initTheme() {
  if (window.CWWebPrefs) {
    const resolved = CWWebPrefs.applyThemeFromPrefs();
    preloadMascotAssets();
    syncMascotIcons(resolved);
    return;
  }
  const saved = localStorage.getItem(THEME_KEY);
  const theme = saved === "light" || saved === "dark"
    ? saved
    : (window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark");
  document.documentElement.setAttribute("data-theme", theme);
  preloadMascotAssets();
  syncMascotIcons(theme);
}

const THEME_FALLBACK_MS = 760;

function runThemeVeil(veil) {
  if (!veil) return;
  veil.classList.remove("theme-veil-run");
  void veil.offsetWidth;
  veil.classList.add("theme-veil-run");
}

function endThemeTransition(root) {
  if (!root.classList.contains("theme-transition")) {
    return;
  }
  root.classList.add("theme-transition-end");
  requestAnimationFrame(() => {
    root.classList.remove("theme-transition", "theme-transition-end");
  });
}

function finishThemeSwap(root, veil, next) {
  syncMascotIcons(next);
  veil?.classList.remove("theme-veil-run");
  endThemeTransition(root);
}

function toggleTheme() {
  const root = document.documentElement;
  const next = root.getAttribute("data-theme") === "light" ? "dark" : "light";
  const veil = $("#theme-veil");
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  const applyTheme = () => {
    root.setAttribute("data-theme", next);
    localStorage.setItem(THEME_KEY, next);
    window.CWWebPrefs?.set?.("theme", next);
  };

  const onThemeDone = () => {
    requestAnimationFrame(() => finishThemeSwap(root, veil, next));
  };

  if (reduced) {
    applyTheme();
    syncMascotIcons(next);
    return;
  }

  const useViewTransition = typeof document.startViewTransition === "function";
  runThemeVeil(veil);

  if (useViewTransition) {
    const vt = document.startViewTransition(() => applyTheme());
    if (vt?.finished) {
      vt.finished.then(onThemeDone).catch(onThemeDone);
    } else {
      onThemeDone();
    }
    return;
  }

  root.classList.add("theme-transition");
  applyTheme();
  window.setTimeout(onThemeDone, THEME_FALLBACK_MS);
}

initTheme();

function bindThemeButtons() {
  for (const id of ["btn-theme", "btn-theme-login"]) {
    const el = $(`#${id}`);
    if (el) el.addEventListener("click", toggleTheme);
  }
}
bindThemeButtons();

function initLoginMascotVideo() {
  const canvas = document.getElementById("login-mascot-canvas");
  if (!canvas) return;

  const video = document.createElement("video");
  video.src = "/formiga.mp4";
  video.loop = true;
  video.muted = true;
  video.autoplay = true;
  video.playsInline = true;

  const ctx = canvas.getContext("2d", { willReadFrequently: true });

  const setDimensions = () => {
    canvas.width = video.videoWidth;
    canvas.height = video.videoHeight;
  };

  if (video.readyState >= 1) {
    setDimensions();
  } else {
    video.addEventListener("loadedmetadata", setDimensions);
  }

  function step() {
    if (video.paused || video.ended) {
      requestAnimationFrame(step);
      return;
    }

    if (canvas.width !== video.videoWidth || canvas.height !== video.videoHeight) {
      setDimensions();
    }

    ctx.drawImage(video, 0, 0, canvas.width, canvas.height);

    try {
      const frame = ctx.getImageData(0, 0, canvas.width, canvas.height);
      const data = frame.data;
      const length = data.length;

      for (let i = 0; i < length; i += 4) {
        const r = data[i + 0];
        const g = data[i + 1];
        const b = data[i + 2];
        const avg = (r + g + b) / 3;
        const maxDiff = Math.max(r, g, b) - Math.min(r, g, b);

        // Blue container body or glowing eyes
        const isBlue = (b > r + 10 && b > g + 2);

        let alphaRatio = 1;

        if (isBlue) {
          alphaRatio = 1; // Solid blue container & eyes
        } else if (avg > 238 && b > 70) {
          alphaRatio = 1; // Protect crisp white DOCKER text & whale logo on container
        } else if (avg > 105 && maxDiff < 18) {
          // Universal studio background wall & floor keying (removes all patches behind neck & under belly)
          if (avg < 125) {
            alphaRatio = 1 - (avg - 105) / 20; // Smooth anti-aliased edge
          } else {
            alphaRatio = 0; // Completely transparent studio background & floor light
          }
        }

        data[i + 3] = Math.floor(data[i + 3] * alphaRatio);
      }
      ctx.putImageData(frame, 0, 0);
    } catch (e) {
      console.warn("Chroma key failed (likely CORS/taint):", e);
    }

    requestAnimationFrame(step);
  }

  video.addEventListener("play", () => {
    requestAnimationFrame(step);
  });

  video.play().catch(() => {
    const resumeVideo = () => {
      video.play();
      document.removeEventListener("pointerdown", resumeVideo);
      document.removeEventListener("keydown", resumeVideo);
    };
    document.addEventListener("pointerdown", resumeVideo);
    document.addEventListener("keydown", resumeVideo);
  });
}
initLoginMascotVideo();


const MODULES = [
  { id: "explorer", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>`, accent: "cyan", title: "Gerenciador de arquivos", desc: "Painel duplo local/remoto, enviar e receber arquivos.", kw: "arquivos sftp transferência" },
  { id: "docker", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>`, accent: "indigo", title: "Contêineres Docker", desc: "Lista, métricas, logs, consola e ciclo de vida.", kw: "docker container" },
  { id: "volbackup", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>`, accent: "indigo", title: "Backup de Volumes", desc: "Localize volumes Docker, faça backups (local/SSH) e restaure volumes.", kw: "backup restore volume docker salvar carregar sftp" },
  { id: "dbbackup", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/><path d="M3 12c0 1.66 4 3 9 3s9-1.34 9-3"/></svg>`, accent: "indigo", title: "Backup de Bancos", desc: "Identifique bancos de dados no servidor e faça backups integrais ou agende rotinas.", kw: "backup banco dados database postgres mysql mariadb sqlserver integral incremental rotina" },
  { id: "composeopt", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>`, accent: "indigo", title: "Otimizador YAML", desc: "Compose e Swarm: localiza YAML, compara hardware e sugere CPU, RAM e JVM.", kw: "compose swarm stack yaml docker otimizar memoria cpu java" },
  { id: "disks", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><rect x="2" y="2" width="20" height="20" rx="2" ry="2"/><path d="M12 18h.01"/><path d="M8 6h8v6H8z"/></svg>`, accent: "emerald", title: "Discos e armazenamento", desc: "lsblk, uso de pastas, LVM e ampliação.", kw: "disco lsblk armazenamento lvm ncdu treesize" },
  { id: "services", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><rect x="4" y="4" width="16" height="16" rx="2" /><rect x="9" y="9" width="6" height="6" /><line x1="9" y1="1" x2="9" y2="4" /><line x1="15" y1="1" x2="15" y2="4" /><line x1="9" y1="20" x2="9" y2="23" /><line x1="15" y1="20" x2="15" y2="23" /><line x1="20" y1="9" x2="23" y2="9" /><line x1="20" y1="15" x2="23" y2="15" /><line x1="1" y1="9" x2="4" y2="9" /><line x1="1" y1="15" x2="4" y2="15" /></svg>`, accent: "teal", title: "Serviços", desc: "systemd: estado agora, iniciar/parar e início automático ao ligar o servidor.", kw: "serviço systemd systemctl nginx apache boot enable" },
  { id: "terminal", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>`, accent: "amber", title: "Terminal SSH", desc: "Consola remota interativa.", kw: "terminal ssh shell" },
  { id: "automations", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 15V9a4 4 0 0 0-4-4H9"/><path d="M6 9v6"/></svg>`, accent: "violet", title: "Central de automações", desc: "Regras e histórico por host.", kw: "automação regras" },
  { id: "deploy", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="8.5" cy="7" r="4.5"/><line x1="20" y1="8" x2="20" y2="14"/><line x1="23" y1="11" x2="17" y2="11"/></svg>`, accent: "violet", title: "Cadastrar Cliente", desc: "Gere o comando de instalação para conectar o cliente à VPN.", kw: "vpn deploy cadastrar instalar cliente headscale", adminOnly: true },
  { id: "settings", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>`, accent: "rose", title: "Configurações", desc: "Conta, interface, SSH, módulos e administração.", kw: "configurações admin", adminOnly: true },
  { id: "desktop", icon: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="hub-icon"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>`, accent: "sky", title: "Ambiente Linux", desc: "Desktop com janelas: arquivos, sistema (rede/disco), Docker e terminal.", kw: "linux desktop ubuntu gui gráfico leigo iniciante ambiente janelas rede armazenamento" },
];

function closeAllAppDialogs() {
  document.querySelectorAll("dialog").forEach((dlg) => {
    if (dlg.open) dlg.close();
  });
}

function handleSessionExpired(message) {
  const msg = message || "Sessão expirada — inicie sessão novamente.";
  state.user = null;
  state.ssh.connected = false;
  state.ssh.host = "";
  state.ssh.user = "";
  closeAllAppDialogs();
  closeTerminalCmdPanel();
  closeTerminal();
  setAppTopbar(false);
  showView("#view-login");
  showLoginPhase("auth");
  const err = $("#login-error");
  if (err) {
    err.textContent = msg;
    err.classList.remove("hidden");
  }
  CWUI.toast(msg, "error", 7000);
}

async function api(path, options = {}) {
  const { noAuthRedirect, ...fetchOpts } = options;
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(fetchOpts.headers || {}) },
    ...fetchOpts,
  });
  const data = await res.json().catch(() => ({}));
  if (
    res.status === 401 &&
    !noAuthRedirect &&
    !String(path).includes("/api/auth/login") &&
    !String(path).includes("/api/auth/me")
  ) {
    handleSessionExpired(data.error || "Sessão expirada — inicie sessão novamente.");
    throw new Error(data.error || "Sessão expirada");
  }
  if (!res.ok) throw new Error(data.error || res.statusText || "Erro");
  return data;
}

function showView(id) {
  closeTerminalCmdPanel();
  if (id === "#view-login") {
    closeAllAppDialogs();
    window.hubAntColony?.stop();
  }
  $$(".view").forEach((v) => v.classList.remove("active"));
  $(id).classList.add("active");
}

function setAppTopbar(visible) {
  $("#app-topbar")?.classList.toggle("hidden", !visible);
  $("#app")?.classList.toggle("app-authed", !!visible);
  if (visible) requestAnimationFrame(syncAppTopbarHeight);
}

function showLoginPhase(phase) {
  const auth = $("#login-auth-panel");
  const conn = $("#login-connect-panel");
  const card = $("#login-card");
  const isConnect = phase === "connect";
  auth?.classList.toggle("hidden", isConnect);
  auth?.toggleAttribute("hidden", isConnect);
  conn?.classList.toggle("hidden", !isConnect);
  conn?.toggleAttribute("hidden", !isConnect);
  card?.classList.toggle("login-card--connect", isConnect);
  if (isConnect) state.screen = "connect";
  if (isConnect) {
    conn?.classList.remove("login-panel-enter");
    void conn?.offsetWidth;
    conn?.classList.add("login-panel-enter");
    requestAnimationFrame(() => CWUI.enhanceSelects?.(conn));
  }
}

function applyUserPermissions(perms) {
  state.permissions = {
    screens: Array.isArray(perms?.screens) ? [...perms.screens] : [],
    actions: Array.isArray(perms?.actions) ? [...perms.actions] : [],
  };
}

function canScreen(id) {
  if (state.user?.isAdmin || state.ssh.isAdmin) return true;
  const screens = state.permissions?.screens;
  if (!screens?.length) return true;
  return screens.includes(id);
}

function canAction(id) {
  if (state.user?.isAdmin || state.ssh.isAdmin) return true;
  const actions = state.permissions?.actions;
  if (!actions?.length) return true;
  return actions.includes(id);
}

window.canScreen = canScreen;
window.canAction = canAction;

async function afterAuthSuccess(me) {
  state.user = me;
  state.user.mustChangePassword = !!me.mustChangePassword;
  state.ssh.isAdmin = !!me.isAdmin;
  applyUserPermissions(me.permissions);
  $("#user-label").textContent = me.displayName || me.username;
  setAppTopbar(true);
  if (await window.CWSettings?.promptMustChangePassword?.()) {
    /* continua após troca de senha */
  }
  showView("#view-login");
  showLoginPhase("connect");
  await refreshConnections();
  await refreshSSH();
  tryAutoReconnectSSH();
}

function closeTerminalCmdPanel() {
  const ov = document.getElementById("terminal-cmd-overlay");
  if (!ov) return;
  if (window.CWMotion?.closeOverlay) {
    window.CWMotion.closeOverlay(ov);
    return;
  }
  ov.classList.add("hidden");
  ov.setAttribute("aria-hidden", "true");
}

function showScreen(name) {
  if (name !== "terminal") closeTerminalCmdPanel();
  if (name === "deploy") {
    showScreen("settings");
    window.CWSettings?.showSettingsTab?.("deploy");
    return;
  }
  if (!canScreen(name)) {
    CWUI.toast("Sem permissão para acessar este módulo.", "error");
    if (state.ssh.connected && name !== "hub") name = "hub";
    else return;
  }
  const prevScreen = state.screen;
  state.screen = name;
  if (name !== "hub") {
    window.hubAntColony?.stop();
  }
  $$(".subview").forEach((v) => {
    v.classList.toggle("active", v.id === `view-${name}`);
  });
  const hubBtn = $("#btn-hub");
  if (hubBtn) hubBtn.hidden = name === "hub" || !state.ssh.connected;
  if (name === "explorer") refreshTransferStatus();
  if (name === "docker") {
    loadDocker();
    window.dockerOnScreenEnter?.();
  }
  if (name === "volbackup") {
    loadVolBackup();
  }
  if (name === "dbbackup") {
    loadDBBackup();
  }
  if (name === "composeopt") window.composeOptOnScreenEnter?.();
  if (prevScreen === "disks" && name !== "disks") window.disksOnScreenLeave?.();
  if (name === "disks") loadDisks();
  if (prevScreen === "services" && name !== "services") window.servicesOnScreenLeave?.();
  if (name === "services") window.loadServices?.();
  if (name === "automations") {
    loadAutomations();
    startAutoPoll();
  } else {
    stopAutoPoll();
  }
  if (name === "terminal") openTerminal();
  else if (prevScreen === "terminal") closeTerminal();
  if (name === "desktop" && prevScreen !== "desktop") window.CWDesktop?.onEnter?.();
  if (prevScreen === "desktop" && name !== "desktop") window.CWDesktop?.onLeave?.();
  if (name === "settings") window.CWSettings?.loadSettings?.();
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

let hubMascotMagnetBound = false;

/** bindHubMascotMagnet — a formiga segue o cursor com suavização (efeito magnético). */
function bindHubMascotMagnet() {
  if (hubMascotMagnetBound || motionReduced()) return;
  const wrap = $("#hub-mascot");
  const scene = wrap?.querySelector(".hub-mascot-scene");
  if (!wrap || !scene) return;
  hubMascotMagnetBound = true;

  const leftPupil = scene.querySelector(".left-pupil");
  const rightPupil = scene.querySelector(".right-pupil");

  const strength = 32;
  let targetX = 0;
  let targetY = 0;
  let currentX = 0;
  let currentY = 0;
  let raf = null;

  const tick = () => {
    currentX += (targetX - currentX) * 0.14;
    currentY += (targetY - currentY) * 0.14;
    const rotY = currentX * 0.35;
    const rotX = -currentY * 0.28;
    scene.style.transform =
      `translate3d(${currentX.toFixed(2)}px, ${currentY.toFixed(2)}px, 0) rotateX(${rotX.toFixed(2)}deg) rotateY(${rotY.toFixed(2)}deg)`;
    
    if (leftPupil && rightPupil) {
      const pupilX = currentX * 0.12;
      const pupilY = currentY * 0.12;
      leftPupil.style.transform = `rotate(-15deg) translate(${pupilX.toFixed(2)}px, ${pupilY.toFixed(2)}px)`;
      rightPupil.style.transform = `rotate(15deg) translate(${pupilX.toFixed(2)}px, ${pupilY.toFixed(2)}px)`;
    }

    const done = Math.abs(targetX - currentX) < 0.15 && Math.abs(targetY - currentY) < 0.15;
    if (!done || targetX !== 0 || targetY !== 0) {
      raf = requestAnimationFrame(tick);
    } else {
      scene.style.transform = "";
      raf = null;
    }
  };

  const queue = () => {
    if (!raf) raf = requestAnimationFrame(tick);
  };

  wrap.addEventListener("mousemove", (e) => {
    const r = wrap.getBoundingClientRect();
    const cx = r.left + r.width / 2;
    const cy = r.top + r.height / 2;
    const nx = (e.clientX - cx) / (r.width / 2);
    const ny = (e.clientY - cy) / (r.height / 2);
    const dist = Math.min(1, Math.hypot(nx, ny));
    const pull = 0.55 + dist * 0.45;
    targetX = nx * strength * pull;
    targetY = ny * strength * pull;
    wrap.classList.add("is-active");
    queue();
  });

  wrap.addEventListener("mouseleave", () => {
    targetX = 0;
    targetY = 0;
    wrap.classList.remove("is-active");
    if (leftPupil && rightPupil) {
      leftPupil.style.transform = "";
      rightPupil.style.transform = "";
    }
    queue();
  });

  wrap.addEventListener("click", () => {
    if (wrap.classList.contains("is-clicked")) return;
    wrap.classList.add("is-clicked");
    setTimeout(() => {
      wrap.classList.remove("is-clicked");
    }, 800);
  });
}

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

function hubModuleText(m, field) {
  const key = `module.${m.id}.${field}`;
  const fallback = field === "title" ? m.title : m.desc;
  return window.CWI18n?.t?.(key) || fallback;
}

/** Ajusta offset do conteúdo quando a topbar quebra linha ao redimensionar. */
function syncAppTopbarHeight() {
  const bar = $("#app-topbar");
  const app = $("#app");
  if (!bar || !app || bar.classList.contains("hidden")) return;
  const h = Math.ceil(bar.getBoundingClientRect().height);
  if (h > 0) app.style.setProperty("--topbar-h", `${h}px`);
}

/** Limpa transforms 3D do hub após mudar de monitor / redimensionar. */
function resetHubVisualState() {
  document.querySelectorAll("#hub-grid .hub-card-3d").forEach((shell) => {
    shell.classList.remove("is-tilt");
    shell.style.transform = "";
    shell.style.removeProperty("--mx");
    shell.style.removeProperty("--my");
  });
  const wrap = $("#hub-mascot");
  const scene = wrap?.querySelector(".hub-mascot-scene");
  if (scene) scene.style.transform = "";
  wrap?.classList.remove("is-active");
}

let hubLayoutResizeTimer = null;
function scheduleHubLayoutSync() {
  clearTimeout(hubLayoutResizeTimer);
  hubLayoutResizeTimer = setTimeout(() => {
    syncAppTopbarHeight();
    resetHubVisualState();
  }, 80);
}

function bindAppLayoutSync() {
  if (bindAppLayoutSync.done) return;
  bindAppLayoutSync.done = true;
  window.addEventListener("resize", scheduleHubLayoutSync);
  if (typeof ResizeObserver !== "undefined") {
    const bar = $("#app-topbar");
    if (bar) new ResizeObserver(syncAppTopbarHeight).observe(bar);
    const hubBody = document.querySelector("#view-hub > .subview-body");
    if (hubBody) new ResizeObserver(scheduleHubLayoutSync).observe(hubBody);
  }
  syncAppTopbarHeight();
}

const CARGO_THEMES = [
  { top: "#38bdf8", left: "#0284c7", right: "#0369a1" }, // Docker Blue
  { top: "#34d399", left: "#059669", right: "#047857" }, // Emerald Green
  { top: "#fb7185", left: "#e11d48", right: "#be123c" }, // Rose Red
  { top: "#fbbf24", left: "#d97706", right: "#b45309" }, // Amber Yellow
  { top: "#c084fc", left: "#9333ea", right: "#7e22ce" }, // Purple
];

class HubAntColony {
  constructor() {
    this.container = null;
    this.ants = [];
    this.active = false;
    this.animationFrame = null;
    this.spawnTimer = null;
    this.lastTime = 0;
    this.mouseX = -1000;
    this.mouseY = -1000;
    
    this.onMouseMove = (e) => {
      if (!this.active || !this.container) return;
      const rect = this.container.getBoundingClientRect();
      this.mouseX = e.clientX - rect.left;
      this.mouseY = e.clientY - rect.top;
    };
    
    this.onMouseLeave = () => {
      this.mouseX = -1000;
      this.mouseY = -1000;
    };
  }

  init() {
    const hubView = document.getElementById("view-hub");
    if (!hubView) return;
    
    if (this.container && document.body.contains(this.container)) {
      return;
    }
    
    this.container = document.createElement("div");
    this.container.className = "hub-ant-highway";
    hubView.appendChild(this.container);
  }

  start() {
    this.init();
    if (this.active) return;
    this.active = true;
    this.lastTime = performance.now();
    this.loop();
    this.scheduleSpawning();
    
    window.addEventListener("mousemove", this.onMouseMove);
    this.container?.addEventListener("mouseleave", this.onMouseLeave);
  }

  stop() {
    this.active = false;
    window.removeEventListener("mousemove", this.onMouseMove);
    this.container?.removeEventListener("mouseleave", this.onMouseLeave);
    
    if (this.animationFrame) {
      cancelAnimationFrame(this.animationFrame);
      this.animationFrame = null;
    }
    if (this.spawnTimer) {
      clearTimeout(this.spawnTimer);
      this.spawnTimer = null;
    }
    if (this.container) {
      this.container.innerHTML = "";
    }
    this.ants = [];
    this.mouseX = -1000;
    this.mouseY = -1000;
  }

  scheduleSpawning() {
    if (!this.active) return;
    this.spawnAnt();
    const nextSpawnMs = 3000 + Math.random() * 5000;
    this.spawnTimer = setTimeout(() => this.scheduleSpawning(), nextSpawnMs);
  }

  spawnAnt() {
    if (!this.container) return;
    if (this.ants.length >= 8) return;

    const dir = Math.random() > 0.5 ? 1 : -1;
    const speed = 40 + Math.random() * 30;
    const y = 5 + Math.random() * 16;
    
    const containerWidth = this.container.offsetWidth || window.innerWidth || 800;
    const startX = dir === 1 ? -60 : containerWidth + 10;
    const targetX = dir === 1 ? containerWidth + 60 : -60;
    
    const antEl = document.createElement("div");
    antEl.className = "hub-ant is-walking";
    if (dir === -1) {
      antEl.style.transform = `scaleX(-1)`;
    }
    
    const theme = CARGO_THEMES[Math.floor(Math.random() * CARGO_THEMES.length)];
    
    antEl.innerHTML = `
      <div class="hub-ant-inner">
        <div class="hub-ant-cargo" style="--cargo-top: ${theme.top}; --cargo-left: ${theme.left}; --cargo-right: ${theme.right}">
          <svg viewBox="0 0 16 16" width="100%" height="100%">
            <path d="M 8 2 L 14 5 L 8 8 L 2 5 Z" fill="var(--cargo-top)" />
            <path d="M 2 5 L 8 8 L 8 14 L 2 11 Z" fill="var(--cargo-left)" />
            <path d="M 8 8 L 14 5 L 14 11 L 8 14 Z" fill="var(--cargo-right)" />
          </svg>
        </div>
        <svg class="hub-ant-svg" viewBox="0 0 40 22">
          <!-- Shadow under the ant -->
          <ellipse class="hub-ant-shadow" cx="19" cy="19.5" rx="14" ry="2.2" fill="url(#ant-shadow-grad)" />

          <!-- Antennae pointing forward/up -->
          <path class="hub-ant-antenna antenna-left" d="M 31 9.5 Q 34 5.5 36.5 6.5" />
          <path class="hub-ant-antenna antenna-right" d="M 31 10.5 Q 33 6.5 35 8" />
          
          <!-- Far legs (fl, ml, bl) -->
          <path class="hub-ant-leg leg-fl" d="M 23 11 Q 25 15 27 19" />
          <path class="hub-ant-leg leg-ml" d="M 19 11 Q 19 15 18 19" />
          <path class="hub-ant-leg leg-bl" d="M 15 11 Q 12 15 9 19" />
          
          <!-- Near legs (fr, mr, br) -->
          <path class="hub-ant-leg leg-fr" d="M 23 11 Q 25 15 27 19" />
          <path class="hub-ant-leg leg-mr" d="M 19 11 Q 19 15 18 19" />
          <path class="hub-ant-leg leg-br" d="M 15 11 Q 12 15 9 19" />
          
          <!-- Body parts in side profile (3D look via radial gradients) -->
          <ellipse class="hub-ant-abdomen" cx="10" cy="10" rx="6.5" ry="5.5" fill="url(#ant-body-grad)" />
          <ellipse class="hub-ant-thorax" cx="20" cy="11" rx="4.8" ry="3.3" fill="url(#ant-body-grad)" />
          <ellipse class="hub-ant-head" cx="28" cy="11" rx="3.8" ry="3.8" fill="url(#ant-body-grad)" />

          <!-- Tiny glowing eye -->
          <circle class="hub-ant-eye" cx="29.2" cy="9.8" r="1.1" fill="url(#ant-eye-grad)" />
        </svg>
      </div>
    `;
    
    antEl.style.left = `${startX}px`;
    antEl.style.top = `${y}px`;
    this.container.appendChild(antEl);
    
    const ant = {
      el: antEl,
      x: startX,
      y: y,
      baseY: y,
      dir: dir,
      speed: speed,
      targetX: targetX,
      isWalking: true,
      stopDuration: 0,
      stateTime: 0,
    };
    
    antEl.addEventListener("click", (e) => {
      e.stopPropagation();
      if (antEl.classList.contains("is-clicked")) return;
      antEl.classList.add("is-clicked");
      ant.isWalking = false;
      antEl.classList.remove("is-walking");
      
      setTimeout(() => {
        antEl.classList.remove("is-clicked");
        ant.isWalking = true;
        antEl.classList.add("is-walking");
        ant.stateTime = 0;
      }, 600);
    });
    
    this.ants.push(ant);
  }

  loop() {
    if (!this.active) return;
    
    const now = performance.now();
    const dt = (now - this.lastTime) / 1000;
    this.lastTime = now;
    
    const width = this.container ? (this.container.offsetWidth || window.innerWidth || 800) : (window.innerWidth || 800);
    
    for (let i = this.ants.length - 1; i >= 0; i--) {
      const ant = this.ants[i];
      ant.stateTime += dt;
      
      let targetY = ant.baseY;
      let minSameDirDist = Infinity;
      let speedScale = 1.0;
      
      const dxMouse = this.mouseX - ant.x;
      const dyMouse = this.mouseY - ant.y;
      const distMouse = Math.hypot(dxMouse, dyMouse);
      const isMouseClose = distMouse < 90;
      
      if (isMouseClose && !ant.el.classList.contains("is-clicked")) {
        ant.el.classList.add("is-alerted");
      } else {
        ant.el.classList.remove("is-alerted");
      }
      
      for (let j = 0; j < this.ants.length; j++) {
        if (i === j) continue;
        const other = this.ants[j];
        
        const dx = other.x - ant.x;
        const dy = other.y - ant.y;
        const dist = Math.abs(dx);
        
        // 1. Vertical lane steering/repulsion if close horizontally
        if (dist < 45) {
          if (Math.abs(dy) < 8) {
            // Push away vertically
            const pushDir = dy > 0 ? -1 : (dy < 0 ? 1 : (i > j ? 1 : -1));
            targetY += pushDir * 8;
          }
        }
        
        // 2. Queueing/Collision avoidance for ants in front in same direction
        const sameDir = (other.dir === ant.dir);
        const isAhead = ant.dir === 1 ? (dx > 0) : (dx < 0);
        if (sameDir && isAhead) {
          const aheadDist = Math.abs(dx);
          // Check if they are in overlapping/nearby lanes
          if (Math.abs(dy) < 6) {
            if (aheadDist < minSameDirDist) {
              minSameDirDist = aheadDist;
            }
          }
        }
      }
      
      // Calculate speed scale
      if (minSameDirDist < 25) {
        speedScale = 0; // Stop
      } else if (minSameDirDist < 50) {
        speedScale = (minSameDirDist - 25) / 25; // Gradual slow down
      }
      
      // Update vertical position smoothly
      targetY = Math.max(3, Math.min(23, targetY));
      ant.y += (targetY - ant.y) * 4 * dt;
      ant.el.style.top = `${ant.y}px`;
      
      if (ant.isWalking) {
        const alertMultiplier = isMouseClose ? 2.5 : 1.0;
        const currentSpeed = ant.speed * speedScale * alertMultiplier;
        ant.x += ant.dir * currentSpeed * dt;
        ant.el.style.left = `${ant.x}px`;
        
        if (currentSpeed < 5) {
          ant.el.classList.remove("is-walking");
        } else {
          ant.el.classList.add("is-walking");
        }
        
        // Random pause only if not stuck in traffic and not alerted
        if (!isMouseClose && speedScale > 0.8 && ant.x > 120 && ant.x < width - 120 && ant.stateTime > 3 && Math.random() < 0.006) {
          ant.isWalking = false;
          ant.stateTime = 0;
          ant.stopDuration = 1 + Math.random() * 1.5;
          ant.el.classList.remove("is-walking");
        }
        
        const out = ant.dir === 1 ? ant.x > ant.targetX : ant.x < ant.targetX;
        if (out) {
          ant.el.remove();
          this.ants.splice(i, 1);
        }
      } else {
        // Paused state
        ant.el.classList.remove("is-walking");
        if (ant.stateTime >= ant.stopDuration) {
          ant.isWalking = true;
          ant.stateTime = 0;
          ant.el.classList.add("is-walking");
        }
      }
    }
    
    this.animationFrame = requestAnimationFrame(() => this.loop());
  }
}

function renderHub() {
  bindHubMascotMagnet();
  
  if (!window.hubAntColony) {
    window.hubAntColony = new HubAntColony();
  }
  window.hubAntColony.start();
  const q = ($("#hub-search").value || "").toLowerCase();
  const grid = $("#hub-grid");
  grid.innerHTML = "";
  const host = state.ssh.host || "—";
  $("#hub-connected").textContent = `${state.ssh.user}@${host}`;
  let i = 0;
  const isAdmin = state.user?.isAdmin || state.ssh.isAdmin;
  for (const m of MODULES) {
    if (m.adminOnly && !isAdmin) continue;
    if (!canScreen(m.id)) continue;
    const title = hubModuleText(m, "title");
    const desc = hubModuleText(m, "desc");
    const hay = `${title} ${desc} ${m.kw}`.toLowerCase();
    if (q && !hay.includes(q)) continue;
    const shell = document.createElement("div");
    shell.className = "hub-card-3d";
    shell.dataset.accent = m.accent || "cyan";
    shell.style.setProperty("--delay", `${i * 70}ms`);
    i += 1;

    const card = document.createElement("button");
    card.type = "button";
    card.className = "hub-card";
    card.innerHTML = `<span class="hub-icon-wrap">${m.icon}</span><strong>${escapeHtml(title)}</strong><span class="hub-desc">${escapeHtml(desc)}</span>`;
    card.addEventListener("click", () => showScreen(m.id));

    shell.appendChild(card);
    shell.addEventListener("animationend", () => shell.classList.add("hub-card-entered"), { once: true });
    bindHubCardTilt(shell);
    grid.appendChild(shell);
  }
  if (!grid.children.length) {
    const empty = window.CWI18n?.t?.("module.hub.empty") || "Nenhum módulo encontrado.";
    grid.innerHTML = `<p class="placeholder">${escapeHtml(empty)}</p>`;
  }
  requestAnimationFrame(syncAppTopbarHeight);
}

document.addEventListener("cw-lang-change", () => {
  window.CWI18n?.applyLabels?.();
  if (state.screen === "hub") renderHub();
  const badge = $("#ssh-status");
  if (badge && state.ssh) {
    const sshKey = state.ssh.connected ? "nav.ssh.online" : "nav.ssh.offline";
    badge.textContent = window.CWI18n?.t?.(sshKey) || badge.textContent;
  }
});

async function refreshSSH() {
  const wasConnected = state.ssh.connected;
  const prevScreen = state.screen;
  const st = await api("/api/ssh/status");
  state.ssh.connected = !!st.connected;
  state.ssh.host = st.host || "";
  state.ssh.user = st.user || "";
  state.ssh.isAdmin = !!st.isAdmin;
  if (st.permissions) applyUserPermissions(st.permissions);
  const badge = $("#ssh-status");
  const sshKey = st.connected ? "nav.ssh.online" : "nav.ssh.offline";
  badge.textContent = window.CWI18n?.t?.(sshKey) || (st.connected ? "SSH: online" : "SSH: offline");
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
  if (wasConnected !== state.ssh.connected) {
    document.dispatchEvent(
      new CustomEvent("cw-ssh-changed", { detail: { connected: state.ssh.connected } })
    );
    window.CWDesktop?.onSSHChanged?.(state.ssh.connected);
  }
  if (st.connected) {
    setAppTopbar(true);
    showView("#view-shell");
    if (!wasConnected || prevScreen === "connect") {
      if (window.CWSettings?.navigateHomeScreen) {
        CWSettings.navigateHomeScreen();
      } else {
        showScreen("hub");
        renderHub();
      }
    }
  } else if (state.user) {
    setAppTopbar(true);
    showView("#view-login");
    showLoginPhase("connect");
  } else {
    setAppTopbar(false);
    showView("#view-login");
    showLoginPhase("auth");
  }
}

async function checkSession() {
  try {
    const me = await api("/api/auth/me");
    if (me.authenticated === false) {
      setAppTopbar(false);
      showView("#view-login");
      showLoginPhase("auth");
      return;
    }
    me.mustChangePassword = !!me.mustChangePassword;
    await afterAuthSuccess(me);
  } catch {
    setAppTopbar(false);
    showView("#view-login");
    showLoginPhase("auth");
  }
}

async function tryAutoReconnectSSH() {
  if (!window.CWWebPrefs?.get?.("sshAutoReconnect")) return;
  if (state.ssh.connected) return;
  const prof = CWWebPrefs.get("lastSSHProfile");
  if (!prof) return;
  try {
    await api("/api/ssh/connect", {
      method: "POST",
      body: JSON.stringify({ profileName: prof }),
    });
    await refreshSSH();
    CWUI.toast("SSH reconectado automaticamente", "success");
  } catch {
    /* perfil inválido ou host indisponível */
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
    me.mustChangePassword = !!me.mustChangePassword;
    await afterAuthSuccess(me);
  } catch (e) {
    err.textContent = e.message;
    err.classList.remove("hidden");
  }
});

$("#btn-logout").addEventListener("click", async () => {
  closeTerminal();
  if (CWWebPrefs?.get?.("sshDisconnectOnLogout") && state.ssh.connected) {
    try {
      await api("/api/ssh/disconnect", { method: "POST", body: "{}" });
    } catch { /* ignore */ }
  }
  await api("/api/auth/logout", { method: "POST", body: "{}" });
  state.user = null;
  state.ssh.connected = false;
  setAppTopbar(false);
  showView("#view-login");
  showLoginPhase("auth");
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

$("#btn-ssh-test")?.addEventListener("click", async () => {
  let host = $("#ssh-host")?.value?.trim();
  const port = $("#ssh-port")?.value?.trim() || "22";
  const user = $("#ssh-user")?.value?.trim();
  if (!host || !user) {
    CWUI.toast("Preencha host e usuário.", "error");
    return;
  }
  if (port && port !== "22") {
    if (host.includes(":") && !host.startsWith("[")) {
      host = `[${host}]:${port}`;
    } else {
      host = `${host}:${port}`;
    }
  }
  const out = $("#ssh-test-result");
  if (out) {
    out.classList.remove("hidden");
    out.textContent = window.CWI18n?.t?.("ssh.testing") || "A testar SSH…";
  }
  CWUI.showSplash("Testando conexão…");
  try {
    const data = await api("/api/ssh/test", {
      method: "POST",
      body: JSON.stringify({
        profileName: $("#ssh-profile")?.value || "",
        host,
        user,
        password: $("#ssh-pass")?.value || "",
        insecureHostKey: true,
      }),
    });
    const steps = (data.steps || []).join(" · ");
    if (out) {
      out.textContent = data.ok ? `OK — ${steps}` : `Falhou — ${data.error || steps}`;
      out.classList.toggle("error", !data.ok);
    }
    CWUI.toast(data.ok ? "Teste SSH concluído" : "Teste SSH falhou", data.ok ? "success" : "error");
  } catch (e) {
    if (out) {
      out.textContent = e.message;
      out.classList.add("error");
    }
    CWUI.toast(e.message, "error");
  } finally {
    CWUI.hideSplash();
  }
});

$("#ssh-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  CWUI.showSplash("Estabelecendo conexão SSH…");
  try {
    const profileName = $("#ssh-profile").value || "";
    let host = $("#ssh-host").value.trim();
    const port = $("#ssh-port")?.value?.trim() || "22";
    if (port && port !== "22") {
      if (host.includes(":") && !host.startsWith("[")) {
        host = `[${host}]:${port}`;
      } else {
        host = `${host}:${port}`;
      }
    }
    await api("/api/ssh/connect", {
      method: "POST",
      body: JSON.stringify({
        profileName,
        host,
        user: $("#ssh-user").value,
        password: $("#ssh-pass").value,
        insecureHostKey: true,
      }),
    });
    if (profileName) CWWebPrefs?.set?.("lastSSHProfile", profileName);
    await refreshSSH();
    CWUI.toast("Conexão SSH estabelecida", "success");
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
  CWUI.refreshSelect?.(sel);
}

async function loadSelectedProfile() {
  const name = $("#ssh-profile").value;
  if (!name) {
    $("#ssh-name").value = "";
    $("#ssh-host").value = "";
    if ($("#ssh-port")) $("#ssh-port").value = "22";
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
    
    let hostVal = p.host || "";
    let portVal = "22";
    const lastColon = hostVal.lastIndexOf(":");
    if (lastColon !== -1 && lastColon > hostVal.lastIndexOf("]")) {
      portVal = hostVal.slice(lastColon + 1);
      hostVal = hostVal.slice(0, lastColon);
      if (hostVal.startsWith("[") && hostVal.endsWith("]")) {
        hostVal = hostVal.slice(1, -1);
      }
    }
    $("#ssh-host").value = hostVal;
    if ($("#ssh-port")) $("#ssh-port").value = portVal;
    
    $("#ssh-user").value = p.user || "";
    $("#ssh-pass").value = p.password || "";
    $("#ssh-save-secrets").checked = !!(p.password || p.hasPassword);
    $("#ssh-conn-status").textContent = `Perfil "${p.name}" carregado.`;
  } catch (e) {
    $("#ssh-conn-status").textContent = e.message;
  }
}

$("#ssh-profile").addEventListener("change", loadSelectedProfile);

$("#ssh-save-profile").addEventListener("click", async () => {
  const name = $("#ssh-name").value.trim();
  let host = $("#ssh-host").value.trim();
  const port = $("#ssh-port")?.value?.trim() || "22";
  const user = $("#ssh-user").value.trim();
  if (!name || !host || !user) {
    CWUI.toast("Preencha nome do perfil, host e usuário.", "error");
    return;
  }
  if (port && port !== "22") {
    if (host.includes(":") && !host.startsWith("[")) {
      host = `[${host}]:${port}`;
    } else {
      host = `${host}:${port}`;
    }
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
    $("#ssh-conn-status").textContent = `Perfil "${name}" salvo.`;
    CWUI.toast(`Perfil "${name}" salvo`, "success");
  } catch (e) {
    CWUI.toast("Não foi possível salvar: " + e.message, "error");
  }
});

$("#ssh-delete-profile").addEventListener("click", async () => {
  const name = $("#ssh-profile").value || $("#ssh-name").value.trim();
  if (!name) {
    CWUI.toast("Selecione ou indique o nome do perfil a excluir.", "error");
    return;
  }
  if (!(await CWConfirm(`Excluir o perfil "${name}"?`, { danger: true, ok: "Excluir" }))) return;
  try {
    await api("/api/connections?name=" + encodeURIComponent(name), { method: "DELETE" });
    $("#ssh-profile").value = "";
    await refreshConnections();
    await loadSelectedProfile();
    $("#ssh-conn-status").textContent = `Perfil "${name}" excluído.`;
    CWUI.toast(`Perfil "${name}" excluído`, "success");
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

function expandXferDock() {
  const dock = $("#explorer-xfer-dock");
  if (!dock) return;
  dock.classList.remove("collapsed");
  if (typeof window.syncXferDockToggle === "function") window.syncXferDockToggle();
  else {
    const btn = $("#btn-xfer-dock-toggle");
    if (btn) {
      btn.textContent = "▾";
      btn.title = "Recolher";
      btn.setAttribute("aria-expanded", "true");
    }
  }
}

function showTransferLog() {
  expandXferDock();
  $("#transfer-log")?.classList.remove("hidden");
}

function hideTransferPanels() {
  const inner = $("#transfer-progress-inner");
  if (inner) inner.innerHTML = "";
}

$("#btn-transfer-log-close").addEventListener("click", () => {
  $("#transfer-log")?.classList.add("hidden");
  state.transferLogPinned = false;
});

function transferIsBusy(st) {
  if (!st) return false;
  if ((st.queued || 0) > 0 || (st.running || 0) > 0) return true;
  if (st.active && st.active.name) return true;
  return (st.recent || []).some((r) => r.status === "running");
}

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
    const busy = transferIsBusy(st);
    const pillText = busy ? `${st.queued || 0}/${st.running || 0}` : "";
    const pillTitle = busy ? `Fila: ${st.queued} · Em execução: ${st.running}` : "Sem transferências";
    for (const id of ["#transfer-status", "#transfer-status-dock"]) {
      const pill = $(id);
      if (pill) {
        pill.textContent = pillText;
        pill.classList.toggle("hidden", !busy);
        pill.title = pillTitle;
      }
    }
    if (busy) {
      expandXferDock();
      CWUI.renderTransferProgress(st.active, st.recent);
    } else {
      hideTransferPanels();
      CWUI.renderTransferProgress(null, []);
    }
    renderTransferLog(st.recent);
  } catch { /* ignore */ }
}
setInterval(() => { if (state.screen === "explorer") refreshTransferStatus(); }, 2000);

// Discos — disks-manager.js (loadDisks)

// Terminal
function terminalWSUrl() {
  const p = location.protocol === "https:" ? "wss:" : "ws:";
  const cols = state.term?.cols || 80;
  const rows = state.term?.rows || 24;
  const cwd = encodeURIComponent(state.remotePath || "/");
  const lang = encodeURIComponent(window.CWWebPrefs?.get?.("lang") || window.CWI18n?.lang?.() || "pt");
  return `${p}//${location.host}/api/ssh/terminal/ws?cols=${cols}&rows=${rows}&cwd=${cwd}&lang=${lang}`;
}

function sendTerminalResize(ws) {
  if (!state.term || !ws || ws.readyState !== WebSocket.OPEN) return;
  ws.send(JSON.stringify({ op: "resize", cols: state.term.cols, rows: state.term.rows }));
}

function setTerminalEndedBanner(show) {
  const banner = document.getElementById("terminal-ended-banner");
  if (!banner) return;
  banner.classList.toggle("hidden", !show);
}

function closeTerminal() {
  setTerminalEndedBanner(false);
  if (state.termSocket) {
    state.termSocket.close();
    state.termSocket = null;
  }
  if (state.term) {
    state.term.dispose();
    state.term = null;
    state.termFit = null;
  }
  state.termLog = "";
}

function sendTerminalRaw(data) {
  if (!data || state.termSocket?.readyState !== WebSocket.OPEN) return false;
  state.termSocket.send(data);
  return true;
}

function sendTerminalCmd(cmd, run) {
  const c = String(cmd ?? "");
  if (!c.trim() || state.termSocket?.readyState !== WebSocket.OPEN) return false;
  sendTerminalRaw(c + (run ? "\r" : ""));
  state.term?.focus?.();
  return true;
}

window.CWTerminal = {
  isConnected: () => state.termSocket?.readyState === WebSocket.OPEN,
  sendRaw: sendTerminalRaw,
  sendCmd: sendTerminalCmd,
  clear: () => {
    state.term?.clear?.();
    state.termLog = "";
    sendTerminalCmd("clear", true);
  },
  ctrlC: () => sendTerminalRaw("\x03"),
  getLog: () => state.termLog || "",
  focus: () => state.term?.focus?.(),
};

function fitTerminal() {
  if (state.screen !== "terminal" || !state.termFit) return;
  try {
    state.termFit.fit();
    sendTerminalResize(state.termSocket);
  } catch (_) { /* viewport ainda sem dimensões */ }
}

function openTerminal() {
  if (!state.ssh.connected) return;
  setTerminalEndedBanner(false);
  const box = $("#terminal");
  const fontSize = Number(window.CWWebPrefs?.get?.("terminalFontSize")) || 14;
  if (!state.term) {
    state.term = new Terminal({ theme: { background: "#0f1419", foreground: "#e8eef7", cursor: "#3b82f6" }, fontSize });
    state.termFit = new (window.FitAddon?.FitAddon || FitAddon.FitAddon)();
    state.term.loadAddon(state.termFit);
    state.term.open(box);
    state.term.onResize(() => sendTerminalResize(state.termSocket));
    if (!state.termResizeBound) {
      state.termResizeBound = true;
      window.addEventListener("resize", fitTerminal);
      if (typeof ResizeObserver !== "undefined") {
        const ro = new ResizeObserver(() => fitTerminal());
        ro.observe(box);
      }
    }
  }
  requestAnimationFrame(() => {
    fitTerminal();
    requestAnimationFrame(fitTerminal);
  });
  if (state.termSocket) state.termSocket.close();
  const ws = new WebSocket(terminalWSUrl());
  state.termSocket = ws;
  ws.onopen = () => {
    fitTerminal();
    sendTerminalResize(ws);
  };
  ws.onmessage = (ev) => {
    const chunk = typeof ev.data === "string" ? ev.data : "";
    if (chunk) {
      const max = 2 * 1024 * 1024;
      state.termLog += chunk;
      if (state.termLog.length > max) state.termLog = state.termLog.slice(-max);
    }
    state.term.write(ev.data);
  };
  ws.onclose = () => {
    state.term.writeln("\r\n\x1b[33mSessão terminada.\x1b[0m\r\n");
    setTerminalEndedBanner(true);
  };
  state.term.onData((data) => { if (ws.readyState === WebSocket.OPEN) ws.send(data); });
}
$("#terminal-reconnect").addEventListener("click", openTerminal);
$("#terminal-ended-reconnect")?.addEventListener("click", openTerminal);

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

function autoTr(key, vars) {
  return window.CWI18n?.t?.(key, vars) || key;
}

async function refreshAutoEngine() {
  const st = await api("/api/automations/engine");
  const running = !!st.running;
  const lbl = $("#auto-engine-status");
  const btn = $("#auto-engine-toggle");
  lbl.textContent = running ? autoTr("auto.engine.on") : autoTr("auto.engine.off");
  lbl.className = running ? "badge on" : "badge muted";
  btn.textContent = running ? autoTr("auto.engine.stop") : autoTr("auto.engine.start");
}

async function refreshAutoHistory() {
  const hist = await api("/api/automations/history");
  $("#auto-history").textContent = (hist.lines || []).join("\n") || autoTr("auto.history.empty");
}

function renderAutoRules() {
  const rulesBox = $("#auto-rules");
  rulesBox.innerHTML = "";
  const saveBtn = $("#auto-save-rules");
  saveBtn.hidden = !state.autoRulesDirty;
  if (!state.autoRules.length) {
    CWUI.emptyState(rulesBox, {
      icon: "⚙️",
      title: autoTr("auto.rule.emptyTitle"),
      desc: autoTr("auto.rule.emptyDesc"),
      actionLabel: autoTr("common.refresh"),
      onAction: () => loadAutomations(),
    });
    return;
  }
  state.autoRules.forEach((r, idx) => {
    const row = document.createElement("div");
    row.className = "data-row";
    const editable = r.kind === "docker_container_stopped_restart";
    const targetHint = editable
      ? escapeHtml(r.target || autoTr("auto.rule.targetHint"))
      : escapeHtml(r.target || "");
    row.innerHTML = `<div><strong>${escapeHtml(r.name || r.id)}</strong><span class="muted">${escapeHtml(r.trigger || "")} → ${escapeHtml(r.action || "")}</span><br/><span class="muted">${r.enabled ? autoTr("auto.rule.active") : autoTr("auto.rule.inactive")} · ${targetHint}</span></div>`;
    const actions = document.createElement("div");
    actions.className = "data-row-actions";
    if (editable) {
      const editBtn = document.createElement("button");
      editBtn.type = "button";
      editBtn.className = "btn btn-ghost btn-sm";
      editBtn.textContent = autoTr("auto.rule.edit");
      editBtn.addEventListener("click", () => openAutoRuleDialog(idx));
      actions.appendChild(editBtn);
    } else {
      const tag = document.createElement("span");
      tag.className = "muted";
      tag.textContent = autoTr("auto.rule.soon");
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
  $("#auto-dialog-title").textContent = autoTr("auto.dialog.edit", { name: r.name || r.id });
  $("#auto-rule-dialog").showModal();
}

function applyDockerAutomationPrefill() {
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
  const existing = state.autoRules.findIndex(
    (r) => r.kind === "docker_container_stopped_restart" && r.target === target
  );
  if (existing >= 0) {
    openAutoRuleDialog(existing);
    CWUI.toast(autoTr("auto.toast.existingRule"), "info");
    return true;
  }
  const rule = {
    id: `docker-${Date.now()}`,
    kind: "docker_container_stopped_restart",
    name: pre.name || autoTr("auto.rule.dockerName", { target }),
    description: autoTr("auto.rule.dockerDesc"),
    trigger: autoTr("auto.rule.dockerTrigger"),
    action: autoTr("auto.rule.dockerAction"),
    target,
    cooldownSec: 60,
    enabled: true,
    webhookURL: "",
  };
  state.autoRules.push(rule);
  state.autoRulesDirty = true;
  renderAutoRules();
  openAutoRuleDialog(state.autoRules.length - 1);
  CWUI.toast(autoTr("auto.toast.newRule"), "info");
  return true;
}

async function loadAutomations() {
  const rulesBox = $("#auto-rules");
  CWUI.skeletonList(rulesBox, 4);
  $("#auto-history").textContent = autoTr("common.loading");
  try {
    await refreshAutoEngine();
    const rules = await api("/api/automations/rules");
    state.autoRules = rules.rules || [];
    state.autoRulesDirty = false;
    renderAutoRules();
    await refreshAutoHistory();
    applyDockerAutomationPrefill();
  } catch (e) {
    rulesBox.innerHTML = `<p class="error">${escapeHtml(e.message)}</p>`;
  }
}

$("#auto-refresh").addEventListener("click", loadAutomations);

document.addEventListener("cw-lang-change", () => {
  if (!document.querySelector("#view-automations.active")) return;
  window.CWI18n?.applyLabels?.();
  void refreshAutoEngine();
  renderAutoRules();
  void refreshAutoHistory();
});

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
    CWUI.toast(autoTr("auto.toast.saved"), "success");
  } catch (e) {
    CWUI.toast(e.message, "error");
  }
});

$("#auto-clear-history").addEventListener("click", async () => {
  if (!(await CWConfirm(autoTr("auto.confirm.clearHistory")))) return;
  try {
    await api("/api/automations/history", { method: "DELETE" });
    await refreshAutoHistory();
    CWUI.toast(autoTr("auto.toast.historyCleared"), "success");
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

bindAppLayoutSync();

// Monitor de atividade (heartbeat) para encerramento automático do serviço ao fechar a aba
(function() {
  const tabId = Math.random().toString(36).substring(2, 15);

  function sendHeartbeat(isUnload = false) {
    const url = isUnload ? "/api/heartbeat/unload" : "/api/heartbeat";
    const payload = JSON.stringify({ tabId });
    if (isUnload && navigator.sendBeacon) {
      const blob = new Blob([payload], { type: "application/json" });
      navigator.sendBeacon(url, blob);
    } else {
      fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: payload,
        keepalive: isUnload
      }).catch(() => {});
    }
  }

  // Envia o primeiro ping
  sendHeartbeat();

  // Envia ping periódico a cada 15 segundos (seguro contra background throttling)
  setInterval(() => {
    sendHeartbeat();
  }, 15000);

  // Envia sinal ao fechar ou recarregar a aba
  window.addEventListener("beforeunload", () => {
    sendHeartbeat(true);
  });
})();
checkSession();
