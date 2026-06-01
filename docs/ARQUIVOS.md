# Gerenciador de arquivos (explorador)

Guia dedicado ao **explorador em painel duplo** do ContainerWay — transferências local ↔ host ↔ contêiner, no **desktop (Fyne)** e na **interface web** (`dev_browser`).

| Versão | Como abrir | Documentação relacionada |
|--------|------------|---------------------------|
| **Desktop** | Hub → **Gerenciador de arquivos** | [GUIA.md](GUIA.md) (secções *Navegação*, *Transferências*, atalhos) |
| **Web** | Hub → cartão do explorador | [WEB_UI.md](WEB_UI.md) (API HTTP, build, roadmap web) |

---

## Layout comum

Dois painéis lado a lado:

| Painel | Conteúdo |
|--------|----------|
| **Local** | Ficheiros do PC onde corre o ContainerWay (Windows no desktop e na web) |
| **Remoto** | Host Linux via **SFTP**, ou ficheiros **dentro de um contêiner** Docker/Podman em execução |

Em cada painel:

- **Breadcrumbs** clicáveis e barra de caminho;
- **Filtro** por nome (limpa ao mudar de pasta ou de contexto remoto);
- **Favoritos** e atalhos (Home, Desktop, … no local);
- Menu de contexto e operações de ficheiros.

---

## Painel remoto: SFTP vs Docker

### Desktop

No painel direito escolhe-se o contexto: **host (SFTP)** ou **contêiner** na lista de contêineres em execução.

### Web

No cabeçalho do painel **Remoto**:

- Toggle **SFTP** — ficheiros no host remoto;
- Toggle **Docker** — selector de contêiner + listagem dentro do contentor.

**Comportamento (web):** ao mudar de SFTP para Docker, de contêiner, ou ao voltar para SFTP, a pasta remota **reinicia sempre em `/`** (não mantém a subpasta anterior).

**Sudo (web, só SFTP):** botão **Sudo** na toolbar — credenciais para listar/editar pastas protegidas no host. Não se aplica ao modo Docker.

---

## Operações de ficheiros

| Operação | Desktop | Web |
|----------|---------|-----|
| Nova pasta | Sim | Sim (menu Ações / contexto) |
| Renomear | Sim | Sim |
| Apagar | Sim | Sim (com confirmação) |
| Copiar / colar entre painéis | Sim | Sim (clipboard no servidor) |
| Comparar pastas atuais | Sim | Sim (relatório em diálogo) |
| Enviar / Receber item | Sim | Sim |
| Lote (itens visíveis) | Sim | Sim (`Lote→` / `←Lote`) |
| Abrir pasta (duplo clique) | Sim | Sim |
| Editar ficheiro no browser | — | Sim (texto ≤ 2 MB; imagem ≤ 8 MB) |
| Abrir com app Windows | Sim (local e remoto monitorizado) | Sim (predefinido / Notepad++; remoto com **Sincronizar remoto**) |
| Drag-and-drop entre painéis | Sim | Planeado |
| Multi-seleção | Implícito em vários fluxos | Planeado |
| Histórico de operações (janela) | Sim | Parcial (log de transferências) |

---

## Transferências

Fluxos suportados:

- **Local ↔ host** (SFTP);
- **Local ↔ contêiner**;
- **Host ↔ contêiner** (via motor interno no desktop).

Características:

- Pastas recursivas;
- Fila com workers paralelos (1–16, configurável no login desktop);
- No desktop: drag-and-drop e «Enviar visíveis» / «Receber visíveis» com confirmação;
- Na web: painel de progresso e histórico de transferências; upload SFTP de ficheiro único pode **omitir** se o destino já tiver o mesmo tamanho (comportamento partilhado com o motor).

---

## Busca e ordenação

### Desktop

- Busca avançada: `ext:log`, `tipo:pasta`, `tipo:arquivo`, seletor Tudo/Pastas/Arquivos;
- Ordenação estilo WinSCP: `..`, pastas A–Z, ficheiros A–Z.

### Web

- Filtro simples por nome no painel;
- Ordenação no servidor (mesma lógica WinSCP no backend).

---

## Atalhos de teclado (desktop)

| Tecla | Ação |
|-------|------|
| `Enter` | Abrir pasta |
| `Backspace` | Subir um nível |
| `Tab` | Alternar painel esquerdo/direito |
| `F3` / `Ctrl+F` | Focar busca do painel ativo |
| `F5` | Atualizar |
| `F6` | Enviar ou Receber (conforme painel) |
| `Ctrl+Shift+F6` | Lote no painel ativo |
| `F2` | Renomear |
| `Del` | Apagar |
| `Ctrl+Shift+N` | Nova pasta |

Na web, atalhos dedicados do explorador estão **planeados** (hoje: `Ctrl+K` paleta, `?` ajuda, `Esc` fechar diálogos).

---

## Contêineres (listagem de ficheiros)

- Apenas contêineres **em execução** aparecem no selector.
- Listagem preferencial: `docker exec ls` (rápida; compatível com Docker/Podman recentes sem depender da API Archive para pastas).
- Leitura para editor na web: `exec cat`.

Requisito no servidor: acesso ao socket Docker/Podman (caminho configurável no perfil de ligação — ver [GUIA.md](GUIA.md)).

---

## Favoritos

| Lado | Desktop | Web |
|------|---------|-----|
| Local | Lista global em preferências | Partilhada com desktop (`/api/explorer/favorites?side=left`) |
| Remoto | Por host e contexto (host vs contêiner) | `side=right`; mesma API na web |

Botão **estrela** no cabeçalho do painel: guarda a pasta atual.

---

## Edição remota (estilo WinSCP)

### Desktop

Abre o ficheiro remoto num editor local, monitora alterações e envia de volta ao guardar.

### Web

1. **Abrir / editar** no diálogo integrado, ou  
2. **Programa predefinido** / **Notepad++** — cópia temporária no PC Windows; para remoto, usar **Sincronizar remoto** após editar fora do browser.

Limites na web: texto **2 MB**; imagens em pré-visualização **8 MB**.

---

## Comparar pastas

Compara o conteúdo das **pastas atuais** dos dois painéis (nomes, tipo, tamanho, data no desktop). Na web o resultado aparece num diálogo de texto; melhorias com tabela acionável estão no roadmap ([WEB_UI.md](WEB_UI.md)).

---

## API web (referência rápida)

Rotas principais do explorador — detalhe completo em [WEB_UI.md](WEB_UI.md):

- Listagem: `GET /api/local/list`, `GET /api/remote/list` (+ `containerId` opcional)
- Ficheiros: `GET|PUT /api/local/file`, `GET|PUT /api/remote/file`
- Externo: `POST /api/local/open-external`, `POST /api/remote/open-external`, `POST /api/remote/sync-external`
- Transferência: `POST /api/transfer/push|pull|batch`, `GET /api/transfer/status`
- Explorador: `GET /api/explorer/compare`, favoritos, clipboard, paste
- Sudo: `GET|POST /api/ssh/sudo`

---

## Paridade e roadmap (web)

| Funcionalidade | Estado na web |
|----------------|---------------|
| Painel duplo, SFTP, Docker, favoritos, lote, sudo, editor | Disponível |
| Comparar (relatório texto) | Disponível |
| Multi-seleção, drag-and-drop, atalhos F5/F2 | Planeado |
| Comparar com ações por ficheiro | Planeado |
| i18n PT/EN/ES | Planeado |

---

## Código (desenvolvedores)

| Área | Caminho |
|------|---------|
| UI desktop | `internal/appui/appui.go`, `dirrow.go`, `foldercompare.go` |
| UI web | `internal/webapp/static/explorer.js`, `index.html` |
| API web | `internal/webapp/api_files.go`, `api_explorer.go`, `api_editor.go`, `api_transfer.go` |
| Host remoto | `internal/hostfs/` |
| Contêiner | `internal/containerfs/` |
| Transferências | `internal/transfer/`, `internal/tarxfer/` |

Mapa geral: [SUMARIO_DESENVOLVEDOR.md](SUMARIO_DESENVOLVEDOR.md).
