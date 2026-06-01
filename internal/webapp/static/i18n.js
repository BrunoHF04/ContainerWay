/** Traduções PT / EN / ES — explorador e UI comum. */
(function () {
  const STR = {
    pt: {
      "explorer.send": "Enviar",
      "explorer.receive": "Receber",
      "explorer.syncMirror": "Sync espelhada",
      "explorer.cancelQueue": "Cancelar fila",
      "explorer.opHistory": "Histórico",
      "explorer.compact": "Compacto",
      "explorer.lang": "Idioma",
      "compare.diff": "Diferentes",
      "filter.hint": "Filtro: texto, ext:pdf, tipo:dir|file",
      "ssh.test": "Testar ligação",
      "ssh.testing": "A testar SSH…",
      "external.modified": "Alterado externamente — clique Sincronizar",
      "toast.syncQueued": "Sincronização enfileirada",
      "toast.queueCleared": "Fila cancelada",
    },
    en: {
      "explorer.send": "Send",
      "explorer.receive": "Receive",
      "explorer.syncMirror": "Mirror sync",
      "explorer.cancelQueue": "Cancel queue",
      "explorer.opHistory": "History",
      "explorer.compact": "Compact",
      "explorer.lang": "Language",
      "compare.diff": "Different",
      "filter.hint": "Filter: text, ext:pdf, type:dir|file",
      "ssh.test": "Test connection",
      "ssh.testing": "Testing SSH…",
      "external.modified": "Changed externally — click Sync",
      "toast.syncQueued": "Sync queued",
      "toast.queueCleared": "Queue cleared",
    },
    es: {
      "explorer.send": "Enviar",
      "explorer.receive": "Recibir",
      "explorer.syncMirror": "Sync espejo",
      "explorer.cancelQueue": "Cancelar cola",
      "explorer.opHistory": "Historial",
      "explorer.compact": "Compacto",
      "explorer.lang": "Idioma",
      "compare.diff": "Diferentes",
      "filter.hint": "Filtro: texto, ext:pdf, tipo:dir|file",
      "ssh.test": "Probar conexión",
      "ssh.testing": "Probando SSH…",
      "external.modified": "Modificado externamente — pulse Sincronizar",
      "toast.syncQueued": "Sincronización en cola",
      "toast.queueCleared": "Cola cancelada",
    },
  };

  const LABELS = {
    pt: { local: "Local", remote: "Remoto", send: "Enviar", receive: "Receber" },
    en: { local: "Local", remote: "Remote", send: "Send", receive: "Receive" },
    es: { local: "Local", remote: "Remoto", send: "Enviar", receive: "Recibir" },
  };

  let lang = localStorage.getItem("cw-lang") || "pt";
  if (!STR[lang]) lang = "pt";

  function t(key) {
    return STR[lang]?.[key] || STR.pt[key] || key;
  }

  function setLang(code) {
    if (!STR[code]) return;
    lang = code;
    localStorage.setItem("cw-lang", code);
    document.documentElement.lang = code === "en" ? "en" : code === "es" ? "es" : "pt-BR";
    applyLabels();
    document.dispatchEvent(new CustomEvent("cw-lang-change", { detail: { lang: code } }));
  }

  function applyLabels() {
    const L = LABELS[lang] || LABELS.pt;
    document.querySelectorAll("[data-i18n]").forEach((el) => {
      const k = el.dataset.i18n;
      if (k === "explorer.local") el.textContent = L.local;
      if (k === "explorer.remote") el.textContent = L.remote;
      if (k === "explorer.send") el.textContent = L.send;
      if (k === "explorer.receive") el.textContent = L.receive;
    });
    const hint = $("#local-filter");
    if (hint && !hint.dataset.userTyped) hint.placeholder = t("filter.hint");
    const rh = $("#remote-filter");
    if (rh && !rh.dataset.userTyped) rh.placeholder = t("filter.hint");
    const bt = $("#btn-ssh-test");
    if (bt) bt.textContent = t("ssh.test");
    ["btn-sync-mirror", "btn-cancel-xfer", "btn-compact-explorer"].forEach((id) => {
      const el = $(`#${id}`);
      if (!el) return;
      const map = {
        "btn-sync-mirror": "explorer.syncMirror",
        "btn-cancel-xfer": "explorer.cancelQueue",
        "btn-compact-explorer": "explorer.compact",
      };
      if (map[id]) el.textContent = t(map[id]);
    });
  }

  window.CWI18n = { t, setLang, lang: () => lang, applyLabels };
  applyLabels();
})();
