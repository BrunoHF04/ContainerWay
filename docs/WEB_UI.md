# ContainerWay Web (MVP)

Interface web local que reutiliza o motor Go do ContainerWay (SSH/SFTP, listagens local/remota, Docker).

> Desenvolvimento ativo na branch **`dev_browser`**. Quando estiver estável, merge manual para `main`.

## Para utilizadores finais (recomendado)

O browser é só a **interface**. O programa real é um **executável local** que levanta o servidor e abre o navegador sozinho.

| Plataforma | O que instalar / executar |
|------------|---------------------------|
| **Windows** | Duplo clique em **`ContainerWay Web.exe`** (na raiz, após build) |
| **Linux (Debian/Ubuntu)** | `sudo dpkg -i dist/deb-web/containerway-web_*_amd64.deb` → menu **ContainerWay Web** |
| **Linux (manual)** | Comando `containerway-web` no PATH |

Comportamento ao iniciar:

1. Sobe o servidor em **http://127.0.0.1:8765** (só neste PC).
2. Abre o navegador predefinido na página de login.
3. Se já estiver a correr, um segundo clique **reabre o browser** (não duplica o servidor).

**Encerrar:** feche o processo `ContainerWay Web` / `containerway-web` no Gestor de Tarefas (Windows) ou termine o processo no Linux. Log em `%LocalAppData%\ContainerWay\web.log` (Windows sem consola).

Após alterações ao código, recompile o executável e faça **Ctrl+F5** no browser (ficheiros estáticos vêm embutidos no binário).

## Compilar o executável

```powershell
.\scripts\build-web.ps1
# ou, com build completo do repo:
.\scripts\build.ps1 -SkipLinux
```

Gera **`ContainerWay Web.exe`** na raiz (Windows, sem janela de consola).

Pacote Linux só web:

```powershell
.\scripts\build.ps1 -SkipWindows
# inclui dist/deb-web/containerway-web_<versão>_amd64.deb
```

## Desenvolvimento (scripts / go run)

Para quem trabalha no código — [SCRIPTS.md](SCRIPTS.md):

```bat
.\scripts\web\run.bat
```

```powershell
go run ./cmd/containerway-web/
```

Flags: `-addr 127.0.0.1:9000`, `-no-browser` (não abrir o navegador).

## Autenticação

- Login de **acesso local** (mesmas contas que o app desktop Fyne).
- Lê `access.users` de `%AppData%\io.containerway.app\preferences.json` quando existir.
- Conta padrão: `admin` / `!q1w2e3r4$` (altere no app desktop em produção).

## Explorador (painel duplo)

Guia completo (desktop + web, atalhos, paridade): **[ARQUIVOS.md](ARQUIVOS.md)**.

Padrões visuais (scrollbar, animações de menus/popovers): **[UI_PADROES.md](UI_PADROES.md)**.

| Funcionalidade | Detalhe |
|----------------|---------|
| Painel **Local** | Pastas do PC onde corre o `ContainerWay Web` (Windows) |
| Painel **Remoto** | Toggle **SFTP** (host) ou **Docker** (ficheiros dentro do contêiner) |
| Navegação | Breadcrumbs, subir, favoritos, atalhos (Home, Desktop, …), filtro por nome |
| Mudança de contexto | Ao trocar SFTP↔Docker ou contêiner, a vista **reinicia em `/`** |
| Operações | Nova pasta, renomear, apagar, copiar/colar entre painéis, comparar pastas |
| Transferências | Enviar / Receber, lote dos itens visíveis — **confirmação antes de enfileirar**; fila com painel de progresso e histórico |
| **Sudo** | Só em modo **SFTP**: eleva privilégios para pastas/ficheiros protegidos no host |
| **Editor** | Abrir/editar texto (até 2 MB); pré-visualização de imagens (até 8 MB) |
| **Abrir externamente** | Programa predefinido do Windows ou Notepad++; remoto grava cópia local temporária com opção **Sincronizar remoto** |
| Toolbar | Ações, Enviar/Receber/Lote, sync espelhada, cancelar fila, modo compacto, idioma PT/EN/ES, Sudo, atualizar, indicador de fila |
| Filtro avançado | `ext:pdf`, `tipo:dir` / `type:file` além de texto livre |
| Atalhos | `Ctrl+C` / `Ctrl+V` copiar/colar; `F5` atualizar; `F2` renomear; `Del` apagar; `Tab` alternar painel |
| Comparar | Secções com Enviar e Receber; itens «Diferentes» com ambas as direcções |
| Upload | Arrastar ficheiros ou pastas para o painel Remoto (estrutura preservada) |
| Ligação | **Testar ligação** no ecrã SSH (sem manter sessão) |
| Histórico | Operações recentes no dock de transferências (`GET/DELETE /api/explorer/operations`) |
| Edição externa | Poll de alterações (`GET /api/remote/external-status`) e botão **Sincronizar remoto** |

Listagem dentro de contêineres: `docker exec ls` (sem depender de `CopyFromContainer` para pastas); leitura de ficheiros para edição via `exec cat`.

## API

| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/api/health` | Estado do serviço |
| POST | `/api/auth/login` | Login local |
| POST | `/api/auth/logout` | Logout |
| GET | `/api/auth/me` | Utilizador atual |
| GET | `/api/connections` | Lista de perfis SSH |
| GET | `/api/connections?name=` | Detalhe de um perfil |
| POST | `/api/connections` | Guardar/atualizar perfil |
| DELETE | `/api/connections?name=` | Apagar perfil |
| POST | `/api/ssh/connect` | Ligação SSH/SFTP |
| POST | `/api/ssh/disconnect` | Desligar SSH |
| GET | `/api/ssh/status` | SSH ativo? |
| GET/POST | `/api/ssh/sudo` | Estado e activação de sudo no host SFTP |
| GET | `/api/local/list?path=` | Listagem local |
| GET/PUT | `/api/local/file?path=` | Ler/gravar ficheiro local (texto ou imagem base64) |
| POST | `/api/local/open-external` | Abrir ficheiro local no Windows (`editor`: `default` \| `notepad++`) |
| GET | `/api/remote/list?path=` | Listagem remota SFTP |
| GET | `/api/remote/list?path=&containerId=` | Listagem dentro do contêiner |
| GET/PUT | `/api/remote/file?path=` | Ler/gravar remoto (SFTP ou contêiner) |
| POST | `/api/remote/open-external` | Copiar remoto para temp local e abrir no Windows |
| POST | `/api/remote/sync-external` | Enviar ficheiro editado local de volta ao remoto |
| POST | `/api/transfer/push` | Enviar local → remoto (opcional `containerId`) |
| POST | `/api/transfer/pull` | Receber remoto → local |
| POST | `/api/transfer/batch` | Lote push/pull dos itens visíveis |
| GET | `/api/transfer/status` | Fila de transferências |
| POST | `/api/local/mkdir` | Criar pasta local |
| POST | `/api/local/rename` | Renomear local |
| POST/DELETE | `/api/local/delete` | Apagar local |
| GET | `/api/local/shortcuts` | Atalhos Home/Desktop/… |
| POST | `/api/remote/mkdir` | Criar pasta remota |
| POST | `/api/remote/rename` | Renomear remoto |
| POST/DELETE | `/api/remote/delete` | Apagar remoto |
| GET | `/api/explorer/compare` | Comparar pastas dos dois painéis |
| GET/PUT | `/api/explorer/favorites` | Favoritos (partilhados com desktop) |
| GET/POST/DELETE | `/api/explorer/clipboard` | Copiar metadados para colar |
| POST | `/api/explorer/paste` | Colar / mover entre painéis |
| GET | `/api/docker/containers` | Lista (`?all=1`, `?metrics=1`) |
| GET | `/api/docker/export` | Export CSV |
| POST | `/api/docker/restart` | Reiniciar (Compose recreate se aplicável) |
| POST | `/api/docker/restart-batch` | Reiniciar vários |
| POST | `/api/docker/stop` \| `start` \| `pause` \| `unpause` \| `remove` | Ciclo de vida |
| GET | `/api/docker/networks` | Redes |
| GET | `/api/docker/system` | Uso de disco (system df) |
| POST | `/api/docker/images/prune` | Limpar imagens dangling |
| POST | `/api/docker/volumes/prune` | Limpar volumes não usados |
| POST | `/api/docker/buildcache/prune` | Limpar cache de build |
| POST | `/api/docker/containers/create` | Criar e iniciar contêiner |
| POST | `/api/docker/networks/create` | Criar rede |
| POST | `/api/docker/networks/remove` | Remover rede |
| POST | `/api/docker/volumes/create` | Criar volume |
| GET | `/api/docker/logs?id=` | Logs do contêiner |
| GET | `/api/docker/stats?id=` | Métricas (`?raw=1` para JSON bruto) |
| GET | `/api/docker/inspect?id=` | Inspect resumido |
| WS | `/api/docker/exec/ws?id=` | Consola no contêiner |
| GET | `/api/disks/summary` | Discos (legado, resumo simples) |
| GET | `/api/disks/probe` | Discos: lsblk, df, LVM (`lvRecords`, `vgStats`), tabela host |
| POST | `/api/disks/extend-lv` | Ampliar LV (`sizeMode`: `delta` ou `absolute`) + resize FS |
| POST | `/api/disks/shrink-lv` | Reduzir LV (ext/btrfs; xfs bloqueado na UI) |
| POST | `/api/disks/fsck` | Verificar FS (read-only) |
| POST | `/api/disks/snapshot-create` | Criar snapshot LVM |
| POST | `/api/disks/snapshot-remove` | Remover snapshot |
| POST | `/api/disks/lv-create` | Criar LV no VG (+ `mkfs` opcional) |
| POST | `/api/disks/resize-fs` | Só redimensionar FS (`grow`: true/false) |
| POST | `/api/disks/lv-rename` | Renomear LV |
| POST | `/api/disks/vg-change` | Activar/desactivar VG (`activate`) |
| POST | `/api/disks/fstrim` | `fstrim` no ponto de montagem |
| GET | `/api/disks/smart?dev=` | SMART (`smartctl`) do disco físico |
| GET | `/api/disks/usage?path=` | Uso por pasta (TreeSize/ncdu) |
| GET | `/api/services` | Lista serviços systemd |
| POST | `/api/services/control` | start/stop/restart/reload |
| GET | `/api/services/logs` | Logs de unidade (`?unit=&lines=`) |
| GET / PUT | `/api/automations/rules` | Regras do host |
| GET / DELETE | `/api/automations/history` | Histórico / limpar |
| GET / POST | `/api/automations/engine` | Motor (`start` \| `stop`) |
| GET / PUT | `/api/admin/users` | Utilizadores (só admin) |
| GET / PUT / POST | `/api/admin/mail` | SMTP e teste |
| GET | `/api/compose-opt/discover` | Descobrir YAML Compose/Swarm no host (`roots`, `max`, `depth`) |
| GET / POST | `/api/compose-opt/analyze` | Analisar YAML e sugerir otimizações (`path` ou `content`, `mode`) |
| POST | `/api/compose-opt/validate` | Validar YAML no host (`docker stack config` ou `docker compose config`) |
| WS | `/api/ssh/terminal/ws` | Terminal interativo |

## Estado atual (telas)

| Tela | Disponível |
|------|------------|
| Login de acesso local | Sim |
| Ligação SSH (perfis) | Sim |
| Hub / menu da sessão | Sim |
| Explorador dual (SFTP + Docker, sudo, editor, externo, favoritos, lote, comparar) | Sim |
| Contêineres Docker (métricas, ciclo de vida, lote, logs live, consola, CSV) | Sim |
| Discos (lsblk, LVM completo, snapshots, SMART, arquivos, técnico) | Sim |
| Serviços systemd (lista, controlo, logs) | Sim |
| Terminal SSH (WebSocket + xterm) | Sim |
| Automações (regras, motor, histórico) | Sim |
| Otimizador Compose/Swarm YAML (descoberta, diff, gravação remota) | Sim |
| Admin: utilizadores e SMTP | Sim |
| i18n (pt-BR / EN / ES) — explorador e módulo Discos | Parcial |
| Multi-seleção, drag-and-drop, atalhos, comparar acionável, preview, dock | Disponível (`explorer-enhanced.js`) |

## Interface

- Tema **escuro** / **claro** (`localStorage`)
- Fundo animado (gradientes, grelha, glassmorphism)
- Cabeçalho Remoto: toggle **SFTP** / **Docker** + selector de contêiner compacto
- Toolbar do explorador numa única barra (Ações \| transferências \| Sudo / fila)
- Diálogos centrados; editor com pré-visualização de imagens

## Discos e LVM (assistente web)

| Funcionalidade | Detalhe |
|----------------|---------|
| Sondagem | `lsblk`, `df`, `lvs`/`vgs`/`pvs`; resposta JSON com `lvRecords` e `vgStats` |
| Resumo VG | Espaço total e livre do VG do LV seleccionado |
| Tamanho | Modo **+/- GiB** ou **tamanho final** (absoluto) para ampliar/reduzir |
| Snapshots | Criar, listar por LV de origem, remover (confirmação forte) |
| Manutenção | Verificar FS, só resize FS, fstrim, renomear/criar LV, `vgchange` |
| Host | Seleccionar linha → **SMART do disco** |
| Arquivos | **Analisar montagem** abre a aba Arquivos no path montado |

Código: `internal/diskutil/` (scripts bash), `internal/webapp/disks.go`, `disks_ops.go`, `static/disks-manager.js`.

## Próximas fases (explorador web)

1. i18n completo (PT / EN / ES) em todos os novos textos do assistente LVM
2. Sincronização espelhada de pastas
3. Paridade desktop: redução de LV também na UI Fyne (`diskstorage.go`)

## Estrutura

| Caminho | Responsabilidade |
|---------|------------------|
| `cmd/containerway-web/` | Entrada do servidor HTTP |
| `internal/webapp/` | API, sessões, UI em `static/` |
| `internal/webapp/disks_ops.go` | Operações LVM (fsck, snapshot, smart, …) |
| `internal/diskutil/` | `ProbeScript`, parse LVS/VGS, scripts `grow`/`shrink`/`ops` |
| `internal/webapp/static/disks-manager.js` | Módulo Discos na UI |
| `internal/webapp/static/services-manager.js` | Módulo Serviços systemd |
| `internal/composeopt/` | Análise e patch de YAML Compose/Swarm (CPU, RAM, JVM, Swarm) |
| `internal/webapp/api_compose_opt.go` | API discover/analyze/validate do otimizador |
| `internal/webapp/static/compose-opt-manager.js` | UI do otimizador YAML |
| `internal/webapp/api_editor.go` | Leitura/gravação de ficheiros para editor |
| `internal/webapp/api_open_external.go` | Abrir no Windows e sincronizar remoto |
| `internal/webapp/api_sudo.go` | API sudo |
| `internal/webapp/shellopen.go` | `shellopen` / Notepad++ no Windows |
| `internal/containerfs/` | Listagem/leitura no contêiner (`exec`) |
| `internal/connectcfg/` | `connections.json` |
| `scripts/web/` | Scripts build/run |
