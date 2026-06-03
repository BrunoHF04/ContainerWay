# Internacionalização (pt-BR, EN, ES)

Regra obrigatória para **qualquer texto visível ao usuário** na UI web, mensagens de API voltadas ao browser e novos módulos.

Relacionado: [LOCALE_PT-BR.md](LOCALE_PT-BR.md) (variante portuguesa), [UI_PADROES.md](UI_PADROES.md) (visual).

---

## Regra

Ao adicionar ou alterar rótulos, botões, placeholders, toasts, títulos de diálogo ou mensagens de erro mostradas na interface:

1. Incluir a chave nos **três** idiomas: `pt`, `en` e `es`.
2. No português, usar **pt-BR** (ver [LOCALE_PT-BR.md](LOCALE_PT-BR.md)).
3. Disparar `applyLabels` / `data-i18n` para que a troca na navbar (`#app-lang`) atualize a tela sem recarregar.

**Não** entregar só em português “para depois traduzir”.

---

## Onde colocar as traduções (web)

| Tipo | Arquivo | Formato |
|------|---------|---------|
| UI web (principal) | `internal/webapp/static/i18n.js` + `i18n-modules.js` | Objeto `STR` com chaves `pt`, `en`, `es` (módulos extra fundidos em `i18n.js`) |
| HTML estático | `internal/webapp/static/index.html` | Atributo `data-i18n="chave"` ou `data-i18n-placeholder` |
| Módulo com apply próprio | `disks-manager.js`, `terminal-tools.js`, etc. | Escutar `cw-lang-change` e chamar `CWI18n.t()` / `applyLabels()` |
| Hub (cartões) | `app.js` → `MODULES` | Chaves `module.<id>.title` / `module.<id>.desc` em `i18n.js` |
| Desktop Linux (apps) | `desktop-apps.js` | Chaves `desktop.app.<id>.label` / `.desc` quando expostas ao usuário |

### Convenção de chaves

- `explorer.*` — gerenciador de arquivos (painel duplo)
- `disks.*` — discos e LVM
- `term.*` — terminal e lista de comandos
- `module.*` — cartões do hub
- `desktop.app.*` — rótulos do ambiente Linux
- `services.*` — serviços systemd (quando migrado para i18n)
- `docker.*` — Docker (quando migrado)
- `ssh.*`, `toast.*`, `filter.*` — comum

Use prefixo do módulo; evite chaves genéricas duplicadas.

### Exemplo mínimo

```javascript
// i18n.js — nos três blocos pt, en, es:
"module.foo.title": "Meu módulo",        // pt-BR
"module.foo.desc": "Descrição curta.",

// index.html ou JS:
<h2 data-i18n="module.foo.title">Meu módulo</h2>
```

Após `DOMContentLoaded`, `CWI18n.applyLabels()` percorre `[data-i18n]`.

### Texto dinâmico em JS

```javascript
const t = (k, vars) => window.CWI18n?.t(k, vars) || k;
CWUI.toast(t("foo.saved"), "success");
```

---

## Desktop (Fyne)

Textos do app nativo: `internal/appui/locale_*.go` (`langPTBR`, `langEN`, `langES`). Nova string no desktop exige entrada nos **três** blocos.

---

## API / backend

Mensagens de erro JSON exibidas na UI web devem estar em pt-BR ou, se forem chaves, mapeadas em `i18n.js`. Preferir chave estável (`errors.permissionDenied`) em vez de frase fixa só em português no Go.

---

## Checklist (PR / revisão)

- [ ] Chave criada em `STR.pt`, `STR.en` e `STR.es`?
- [ ] Português revisado como **pt-BR**?
- [ ] Elementos HTML com `data-i18n` (ou módulo chama `t()` em `cw-lang-change`)?
- [ ] Placeholders com `data-i18n-placeholder` quando aplicável?
- [ ] Toasts e diálogos novos usam `CWI18n.t`?
- [ ] Testou os três idiomas na navbar?

---

## Exceções

- Identificadores técnicos, comandos shell, caminhos, nomes de serviços Docker.
- Logs só para desenvolvedores (comentários em código).
- Documentação em `docs/` pode ter versão só em português, salvo quando for manual do usuário multilíngue.
