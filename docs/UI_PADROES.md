# Padrões de UI (ContainerWay Web)

Referência para **scrollbar**, **animações de superfície** e tokens de movimento na interface web (`internal/webapp/static/app.css`).

| Público | Uso |
|---------|-----|
| Desenvolvedor / agente | Ao criar painéis roláveis, menus, popovers ou modais |
| Design | Consistência dark/light e `prefers-reduced-motion` |

Documentação relacionada: [WEB_UI.md](WEB_UI.md), [LOCALE_PT-BR.md](LOCALE_PT-BR.md).

---

## 1. Scrollbar (barra de rolagem)

### Objetivo

Barras **finas**, **sem setas** (estilo antigo do Windows), cor derivada do tema, hover com destaque `--accent`. Igual em Hub, explorador, Linux desktop, selects, configurações e modais.

### Tokens (`:root`)

| Token | Valor típico | Função |
|-------|----------------|--------|
| `--scrollbar-size` | `7px` | Largura/altura (WebKit) |
| `--scrollbar-track` | `transparent` | Trilho |
| `--scrollbar-thumb` | mix com `--text-secondary` | Polegar |
| `--scrollbar-thumb-hover` | mix com `--accent` | Polegar ao passar o rato |

### Classe utilitária: `.cw-scroll`

Em **qualquer** novo bloco com `overflow: auto` ou `overflow-y: auto`, adicione a classe:

```html
<div class="meu-painel cw-scroll">…</div>
```

Isso aplica o padrão sem editar a lista longa em `app.css`.

### Lista legada em `app.css`

Além de `.cw-scroll`, o ficheiro define o mesmo estilo via `:is(...)` para dezenas de seletores já existentes (`.file-list`, `.cw-select-list`, `.linux-scroll`, `.settings-panels`, etc.).

**Ao adicionar um painel rolável sem `.cw-scroll`:** inclua o seletor na lista `:is(...)` do bloco `/* Scrollbars — padrão global */` em `app.css` (todas as variantes `::-webkit-scrollbar*` e `scrollbar-width`).

### O que evitar

- Scrollbar nativa grossa com setas (não estilizar = regressão visual).
- `::-webkit-scrollbar` isolado num componente sem seguir thumb/track/corner do padrão.
- Largura fixa em px fora de `--scrollbar-size` (ex.: `6px` só num ecrã).

### Firefox

`scrollbar-width: thin` e `scrollbar-color: thumb track` no mesmo bloco que WebKit.

---

## 2. Animações de superfície

### Tokens

| Token | Uso |
|-------|-----|
| `--ease-smooth` | Curvas de entrada suave |
| `--motion-pop-in` | `0.24s` — popovers curtos |
| `--motion-sheet-in` / `--motion-sheet-out` | Modais `<dialog>` |
| `--motion-spring` | Janelas Linux desktop |

### Entrada padrão: `cw-surface-in`

Menus e popovers que **aparecem** (não modais full-screen):

```css
@keyframes cw-surface-in {
  from { opacity: 0; transform: translateY(6px) scale(0.97); }
  to   { opacity: 1; transform: translateY(0) scale(1); }
}
```

**Já aplicado em:**

| Elemento | Notas |
|----------|--------|
| `.panel-menu[open] > .panel-menu-list:not(.is-floating)` | Favoritos / atalhos no explorador |
| `.explorer-menu[open] > .explorer-menu-panel:not(.is-floating)` | Menu Ações |
| `.panel-menu-list.is-floating`, `.explorer-menu-panel.is-floating` | `float-menu-in` (portal no `body`) |
| `.cw-select-list` | `cw-select-in` ao abrir dropdown |
| `.ctx-menu:not(.hidden)` | Menu de contexto do explorador |
| `.linux-notify-pop:not([hidden])` | Painel de notificações (Linux UI) |
| `.modal-dialog[open]` | `modal-sheet-in` + backdrop |
| `.linux-win.is-opening` | Nova janela no desktop Linux |

**Classe opcional:** `.cw-animate-in` — mesma animação `cw-surface-in` para superfícies novas.

### O que não animar de novo

Componentes que **já** têm keyframe dedicado: modais (`modal-sheet-in`), menu flutuante (`float-menu-in`), select (`cw-select-in`), tiles do menu Iniciar (`linux-start-tile-in`), janelas (`linux-win-open`). Reutilize o keyframe existente em vez de inventar outro.

### `prefers-reduced-motion: reduce`

O projeto desativa ou encurta animações globais no topo de `app.css` e em blocos específicos (menus flutuantes, tiles Iniciar, `cw-animate-in`, etc.). Novas animações **devem** respeitar o mesmo media query.

---

## 3. Checklist — novo componente UI

1. [ ] Área rolável tem `.cw-scroll` **ou** seletor na lista `:is(...)` de scrollbars.
2. [ ] Popover/menu novo usa `cw-surface-in` / `float-menu-in` / padrão do tipo (select, modal).
3. [ ] `prefers-reduced-motion` considerado.
4. [ ] Cores vêm de variáveis CSS (`--surface-solid`, `--border-strong`, …), não hex fixo (exceto semântica tipo erro).

---

## 4. Ficheiros principais

| Ficheiro | Conteúdo |
|----------|----------|
| `internal/webapp/static/app.css` | Tokens, scrollbars `:is(...)`, keyframes, `.cw-animate-in` |
| `internal/webapp/static/ui.js` | Select customizado (`.cw-select-list`), paleta de comandos |
| `internal/webapp/static/motion.js` | Saída de modais (`.modal-exit`) |

---

## 5. Histórico

- **2026-06:** Padronização global de scrollbars; `cw-surface-in` em menus inline, contexto e notificações Linux; documento criado após ajuste do dropdown «Atalhos» no gerenciador de arquivos (modo Linux).
