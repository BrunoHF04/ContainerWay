/** Preferências locais da interface web (localStorage). */
(function () {
  const PREFIX = "cw-web-";

  const defaults = {
    theme: "dark",
    lang: "pt",
    homeScreen: "hub",
    reduceMotion: false,
    toastSeconds: 5,
    sshDisconnectOnLogout: false,
    sshAutoReconnect: false,
    lastSSHProfile: "",
    explorerExternalEditor: "default",
    explorerCompactToolbar: false,
    explorerConfirmDelete: true,
    dockerMetricsSec: 15,
    dockerLogLines: 200,
    disksAutoRefresh: true,
    terminalFontSize: 14,
    terminalFavorites: "",
    transferKeepHistory: true,
    desktopWallpaper: "ubuntu",
    desktopSounds: false,
    desktopCompact: false,
    terminalCmdSections: "",
  };

  function key(k) {
    return PREFIX + k;
  }

  function get(name) {
    const def = defaults[name];
    const raw = localStorage.getItem(key(name));
    if (raw == null) return def;
    if (typeof def === "boolean") return raw === "true";
    if (typeof def === "number") {
      const n = Number(raw);
      return Number.isFinite(n) ? n : def;
    }
    return raw;
  }

  function set(name, value) {
    localStorage.setItem(key(name), String(value));
    document.dispatchEvent(new CustomEvent("cw-pref-change", { detail: { name, value } }));
  }

  function getAll() {
    const out = {};
    for (const k of Object.keys(defaults)) out[k] = get(k);
    return out;
  }

  function applyThemeFromPrefs() {
    let theme = get("theme");
    if (theme === "system") {
      theme = window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
    }
    document.documentElement.setAttribute("data-theme", theme);
    if (typeof syncMascotIcons === "function") syncMascotIcons(theme);
    return theme;
  }

  function langSelects() {
    return [...document.querySelectorAll("[data-app-lang]")];
  }

  function syncLangSelects(code) {
    for (const sel of langSelects()) {
      if (sel.value !== code) sel.value = code;
      window.CWUI?.refreshSelect?.(sel);
    }
  }

  function bindAppLang() {
    for (const sel of langSelects()) {
      if (sel.dataset.langBound) continue;
      sel.dataset.langBound = "1";
      sel.addEventListener("change", (e) => {
        const code = e.target.value;
        if (window.CWI18n?.setLang) window.CWI18n.setLang(code);
        else {
          set("lang", code);
          syncLangSelects(code);
        }
      });
    }
  }

  function applyLangFromPrefs() {
    const code = get("lang");
    if (window.CWI18n?.setLang) window.CWI18n.setLang(code);
    syncLangSelects(window.CWI18n?.lang?.() || code);
  }

  function applyReduceMotion() {
    document.documentElement.classList.toggle("reduce-motion", !!get("reduceMotion"));
  }

  function toastDurationMs() {
    const sec = Math.min(30, Math.max(2, Number(get("toastSeconds")) || 5));
    return sec * 1000;
  }

  function saveForm(form, fieldMap) {
    for (const [name, type] of Object.entries(fieldMap)) {
      const el = form.elements[name];
      if (!el) continue;
      if (type === "bool") set(name, !!el.checked);
      else if (type === "num") set(name, Number(el.value) || defaults[name]);
      else set(name, String(el.value ?? "").trim());
    }
  }

  function fillForm(form, fieldMap) {
    for (const [name, type] of Object.entries(fieldMap)) {
      const el = form.elements[name];
      if (!el) continue;
      const v = get(name);
      if (type === "bool") el.checked = !!v;
      else el.value = v ?? "";
    }
  }

  const interfaceFields = {
    theme: "str",
    lang: "str",
    homeScreen: "str",
    reduceMotion: "bool",
    toastSeconds: "num",
  };

  const sessionFields = {
    sshDisconnectOnLogout: "bool",
    sshAutoReconnect: "bool",
  };

  const moduleFields = {
    explorerExternalEditor: "str",
    explorerCompactToolbar: "bool",
    explorerConfirmDelete: "bool",
    dockerMetricsSec: "num",
    dockerLogLines: "num",
    disksAutoRefresh: "bool",
    terminalFontSize: "num",
    terminalFavorites: "str",
    transferKeepHistory: "bool",
  };

  function init() {
    applyReduceMotion();
    bindAppLang();
    applyLangFromPrefs();
    requestAnimationFrame(() => {
      window.CWUI?.enhanceSelects?.(document.getElementById("app-topbar"));
      window.CWUI?.enhanceSelects?.(document.querySelector(".linux-panel-end"));
    });
  }

  document.addEventListener("cw-lang-change", () => {
    syncLangSelects(window.CWI18n?.lang?.() || get("lang"));
  });

  window.CWWebPrefs = {
    defaults,
    get,
    set,
    getAll,
    applyThemeFromPrefs,
    applyLangFromPrefs,
    applyReduceMotion,
    toastDurationMs,
    saveForm,
    fillForm,
    interfaceFields,
    sessionFields,
    moduleFields,
    init,
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
