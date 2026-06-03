/** Motion — transições estilo macOS (módulos, modais, painéis; respeita reduced-motion). */
const CWMotion = (() => {
  const reduced = () =>
    window.matchMedia("(prefers-reduced-motion: reduce)").matches ||
    document.documentElement.classList.contains("reduce-motion");

  const SCREEN_ORDER = [
    "hub",
    "explorer",
    "docker",
    "disks",
    "terminal",
    "automations",
    "settings",
  ];

  let prevScreen = "hub";
  const MODAL_OUT_MS = 240;
  const OVERLAY_OUT_MS = 220;

  function navDirection(next, prev) {
    if (next === prev) return "neutral";
    if (next === "hub") return "back";
    if (prev === "hub" || prev === "connect") return "forward";
    const ni = SCREEN_ORDER.indexOf(next);
    const pi = SCREEN_ORDER.indexOf(prev);
    if (ni >= 0 && pi >= 0 && ni !== pi) return ni > pi ? "forward" : "back";
    return "neutral";
  }

  function pulseClass(el, className) {
    if (!el || reduced()) return;
    el.classList.remove(className);
    void el.offsetWidth;
    el.classList.add(className);
    const clear = () => el.classList.remove(className);
    el.addEventListener("animationend", clear, { once: true });
  }

  function pulseHubCards() {
    if (reduced()) return;
    document.querySelectorAll("#hub-grid .hub-card-3d").forEach((el, i) => {
      el.classList.remove("hub-card-entered");
      el.style.setProperty("--delay", `${Math.min(i, 14) * 55}ms`);
      el.style.animation = "none";
      void el.offsetWidth;
      el.style.removeProperty("animation");
    });
  }

  function pulseScreenEnter(name, dir) {
    if (reduced()) return;
    const shell = document.querySelector(".shell-body");
    shell?.classList.remove("nav-forward", "nav-back");
    if (dir === "forward") shell?.classList.add("nav-forward");
    else if (dir === "back") shell?.classList.add("nav-back");

    const el = document.getElementById(`view-${name}`);
    if (!el) return;
    el.classList.remove("screen-enter");
    void el.offsetWidth;
    el.classList.add("screen-enter");

    if (name === "hub") pulseHubCards();
    if (name === "settings") {
      pulseClass(document.querySelector("#view-settings .settings-panel.active"), "panel-enter");
    }
    if (name === "disks") {
      pulseClass(document.querySelector("#view-disks .disks-section.active"), "section-enter");
    }
  }

  function bloomShell() {
    if (reduced()) return;
    const shell = document.getElementById("view-shell");
    if (!shell) return;
    shell.classList.remove("view-bloom");
    void shell.offsetWidth;
    shell.classList.add("view-bloom");
    shell.addEventListener(
      "animationend",
      () => shell.classList.remove("view-bloom"),
      { once: true }
    );
  }

  function wrapShowScreen() {
    const orig = window.showScreen;
    if (!orig || orig.__cwMotion) return;
    const wrapped = function (name) {
      const dir = navDirection(name, prevScreen);
      orig(name);
      pulseScreenEnter(name, dir);
      prevScreen = name;
    };
    wrapped.__cwMotion = true;
    window.showScreen = wrapped;
  }

  function wrapShowView() {
    const orig = window.showView;
    if (!orig || orig.__cwMotion) return;
    const wrapped = function (id) {
      const prev = document.querySelector(".view.active")?.id;
      orig(id);
      if (id === "#view-shell" && prev === "view-login") bloomShell();
    };
    wrapped.__cwMotion = true;
    window.showView = wrapped;
  }

  function closeDialog(dlg, returnValue) {
    HTMLDialogElement.prototype.close.call(dlg, returnValue);
  }

  function animatedClose(dlg, returnValue) {
    if (!dlg?.open) return;
    if (reduced() || dlg.classList.contains("modal-exit")) {
      closeDialog(dlg, returnValue);
      return;
    }
    dlg.classList.add("modal-exit");
    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      dlg.classList.remove("modal-exit");
      closeDialog(dlg, returnValue);
    };
    const onEnd = (ev) => {
      if (ev.target !== dlg) return;
      dlg.removeEventListener("animationend", onEnd);
      finish();
    };
    dlg.addEventListener("animationend", onEnd);
    setTimeout(finish, MODAL_OUT_MS);
  }

  function enhanceDialog(dlg) {
    if (!dlg || dlg.dataset.cwMotion) return;
    dlg.dataset.cwMotion = "1";

    dlg.close = function (returnValue) {
      if (!dlg.open) return;
      animatedClose(dlg, returnValue);
    };

    dlg.addEventListener("cancel", (e) => {
      if (reduced()) return;
      e.preventDefault();
      animatedClose(dlg);
    });
  }

  function enhanceDialogs() {
    document.querySelectorAll("dialog.modal-dialog").forEach(enhanceDialog);
  }

  function openOverlay(overlay) {
    if (!overlay) return;
    overlay.classList.remove("hidden", "overlay-exit");
    overlay.setAttribute("aria-hidden", "false");
    if (reduced()) return;
    overlay.classList.remove("overlay-enter");
    void overlay.offsetWidth;
    overlay.classList.add("overlay-enter");
  }

  function closeOverlay(overlay, onHidden) {
    if (!overlay || overlay.classList.contains("hidden")) return;
    const finish = () => {
      overlay.classList.add("hidden");
      overlay.classList.remove("overlay-enter", "overlay-exit");
      overlay.setAttribute("aria-hidden", "true");
      onHidden?.();
    };
    if (reduced() || overlay.classList.contains("overlay-exit")) {
      finish();
      return;
    }
    overlay.classList.remove("overlay-enter");
    overlay.classList.add("overlay-exit");
    let done = false;
    const end = () => {
      if (done) return;
      done = true;
      finish();
    };
    overlay.addEventListener(
      "animationend",
      (ev) => {
        if (ev.target === overlay) end();
      },
      { once: true }
    );
    setTimeout(end, OVERLAY_OUT_MS);
  }

  function init() {
    if (window.state?.screen && window.state.screen !== "connect") {
      prevScreen = window.state.screen;
    }
    wrapShowScreen();
    wrapShowView();
    enhanceDialogs();
    new MutationObserver((mutations) => {
      for (const m of mutations) {
        for (const node of m.addedNodes) {
          if (node.nodeType !== 1) continue;
          if (node.matches?.("dialog.modal-dialog")) enhanceDialog(node);
          node.querySelectorAll?.("dialog.modal-dialog").forEach(enhanceDialog);
        }
      }
    }).observe(document.body, { childList: true, subtree: true });
  }

  init();

  return {
    init,
    reduced,
    animatedClose,
    pulseEnter: pulseClass,
    pulseScreenEnter,
    pulseHubCards,
    openOverlay,
    closeOverlay,
    navDirection,
  };
})();

window.CWMotion = CWMotion;
