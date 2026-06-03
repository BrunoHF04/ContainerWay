# Permissões de telas e ações (UI web)

Ao criar um **novo módulo/tela** na interface web, o acesso deve ser configurável por usuário local (exceto admin, que tem tudo).

Textos da UI: [I18N_MULTILINGUE.md](I18N_MULTILINGUE.md). Português: [LOCALE_PT-BR.md](LOCALE_PT-BR.md).

---

## Regra

Cada tela navegável (`showScreen("…")`) precisa de um **ID estável** registrado em:

1. **Backend** — `internal/accessauth/permissions.go`
2. **Admin UI** — `PERM_SCREENS` em `internal/webapp/static/settings.js`
3. **Hub** — cartão em `MODULES` em `internal/webapp/static/app.js` (se aparecer no menu inicial)
4. **Guarda de rota** — `canScreen(id)` em `showScreen()` (já centralizado em `app.js`)
5. **Apps do ambiente Linux** — `screen` / mapa em `desktop-apps.js` (ver abaixo)
6. **API** — ações sensíveis com `HasAction` em `internal/webapp/api_permissions.go` (quando aplicável)

**Não** lançar tela só no HTML sem passar por `canScreen` e sem entrar na lista de permissões.

---

## IDs de tela atuais

| ID | Módulo | `view-*` em `index.html` |
|----|--------|---------------------------|
| `desktop` | Ambiente Linux (janelas) | `view-desktop` |
| `explorer` | Gerenciador de arquivos | `view-explorer` |
| `docker` | Contêineres Docker | `view-docker` |
| `disks` | Discos e armazenamento | `view-disks` |
| `services` | Serviços systemd | `view-services` |
| `terminal` | Terminal SSH | `view-terminal` |
| `automations` | Central de automações | `view-automations` |
| `settings` | Configurações (conta) | `view-settings` |

O **hub** (`showScreen("hub")`) não é permissão separada: é o menu inicial após conectar.

---

## Passo a passo — nova tela `foo`

### 1. Go — `permissions.go`

```go
const ScreenFoo = "foo"

func AllScreens() []string {
  return []string{
    // ...existentes,
    ScreenFoo,
  }
}
```

Incluir em `DefaultPermissions().Screens` se usuários padrão devem ver o módulo.

### 2. Teste — `permissions_test.go`

```go
if !p.HasScreen(ScreenFoo) { /* conforme política default */ }
```

### 3. JavaScript — `settings.js`

```javascript
const PERM_SCREENS = [
  // ...
  { id: "foo", label: "Nome legível na grade de permissões" },
];
```

Manter **mesmos IDs** que em Go (`SanitizePermissions` descarta IDs desconhecidos).

### 4. Hub — `app.js`

```javascript
const MODULES = [
  { id: "foo", icon: "…", accent: "…", title: "…", desc: "…", kw: "…" },
];
```

`renderHub()` já filtra com `canScreen(m.id)`.

### 5. HTML — subview

```html
<div id="view-foo" class="subview">…</div>
```

### 6. Navegação

```javascript
if (!canScreen("foo")) { /* toast; return */ }
showScreen("foo");
```

`showScreen` em `app.js` já bloqueia se `!canScreen(name)`.

### 7. Ações (opcional)

Se a tela chama APIs restritas, adicionar constante `ActionFoo…` em `permissions.go`, entrada em `PERM_ACTIONS` e mapeamento em `api_permissions.go` (`actionForPath` / handlers).

### 8. Ambiente Linux (se houver app em janela)

Em `desktop-apps.js`:

```javascript
register({
  id: "foo-app",
  screen: "foo",  // mesmo ID da tela no hub
  // ...
});
```

`CWDesktopApps.visible()` e `canUseApp()` usam esse `screen` com `canScreen()`.

Mapa hub ↔ app (botão ⌂ na janela): `APP_HUB_SCREEN` em `desktop.js`.

---

## Ações configuráveis (resumo)

| ID | Uso típico |
|----|------------|
| `files.read` / `write` / `delete` / `transfer` | SFTP / explorador |
| `docker.view` / `control` / `manage` | Docker |
| `disks.view` / `manage` | Discos e LVM |
| `services.view` / `control` | systemd |
| `terminal.use` | Terminal WS |
| `automations.manage` | Regras |
| `sudo.use` | Sudo no host |
| `settings.manage` | Usuários e SMTP (admin) |

Lista completa: `PERM_ACTIONS` em `settings.js` e `AllActions()` em Go.

---

## Comportamento

- **Admin** (`admin`): todas as telas e ações (`FullPermissions()`).
- **Usuário sem perfil**: `DefaultPermissions()` (compatível com versões antigas).
- **Perfil customizado**: apenas telas/ações marcadas na grade de Configurações → Usuários.

API HTTP valida **ações** por rota; telas são validadas no **frontend** (`canScreen` / `canAction`).

---

## Checklist (PR / revisão)

- [ ] `Screen…` e `AllScreens()` em `permissions.go`?
- [ ] `PERM_SCREENS` atualizado em `settings.js`?
- [ ] Cartão no `MODULES` (se módulo no hub)?
- [ ] `showScreen("id")` com o mesmo ID?
- [ ] Apps Linux com `screen: "<id>"` correto?
- [ ] Ações de API mapeadas (se houver operações sensíveis)?
- [ ] Teste em `permissions_test.go` (default ou custom)?

---

## Referência cruzada

| Ficheiro | Conteúdo |
|----------|----------|
| `internal/accessauth/permissions.go` | Fonte de verdade (IDs) |
| `internal/webapp/static/settings.js` | Labels na UI admin |
| `internal/webapp/static/app.js` | Hub + `showScreen` |
| `internal/webapp/static/desktop-apps.js` | Apps do desktop Linux |
| `internal/webapp/api_permissions.go` | Ações por rota HTTP |
