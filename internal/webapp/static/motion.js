/** Motion — transições estilo macOS (modais, módulos, respeita reduced-motion). */
const CWMotion = (() => {
  const reduced = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  let prevScreen = "hub";
  const MODAL_OUT_MS = 240;

  function navDirection(next, prev) {
    if (next === prev) return "neutral";
    if (next === "hub") return "back";
    if (prev === "hub") return "forward";
    return "neutral";
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

  function init() {
    wrapShowScreen();
    wrapShowView();
    enhanceDialogs();
  }

  init();

  return { init, reduced, animatedClose };
})();

window.CWMotion = CWMotion;
