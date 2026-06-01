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

| Funcionalidade | Detalhe |
|----------------|---------|
| Painel **Local** | Pastas do PC onde corre o `ContainerWay Web` (Windows) |
| Painel **Remoto** | Toggle **SFTP** (host) ou **Docker** (ficheiros dentro do contêiner) |
| Navegação | Breadcrumbs, subir, favoritos, atalhos (Home, Desktop, …), filtro por nome |
| Mudança de contexto | Ao trocar SFTP↔Docker ou contêiner, a vista **reinicia em `/`** |
| Operações | Nova pasta, renomear, apagar, copiar/colar entre painéis, comparar pastas |
| Transferências | Enviar / Receber, lote dos itens visíveis, fila com painel de progresso e histórico |
| **Sudo** | Só em modo **SFTP**: eleva privilégios para pastas/ficheiros protegidos no host |
| **Editor** | Abrir/editar texto (até 2 MB); pré-visualização de imagens (até 8 MB) |
| **Abrir externamente** | Programa predefinido do Windows ou Notepad++; remoto grava cópia local temporária com opção **Sincronizar remoto** |
| Toolbar | Ações, Enviar/Receber/Lote, Sudo, atualizar, indicador de fila |

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
| GET | `/api/docker/containers` | Contêineres em execução |
| POST | `/api/docker/restart` | Reiniciar contêiner |
| GET | `/api/docker/logs?id=` | Logs do contêiner |
| GET | `/api/docker/stats?id=` | Estatísticas do contêiner |
| GET | `/api/disks/summary` | Discos (lsblk + df) |
| GET / PUT | `/api/automations/rules` | Regras do host |
| GET / DELETE | `/api/automations/history` | Histórico / limpar |
| GET / POST | `/api/automations/engine` | Motor (`start` \| `stop`) |
| GET / PUT | `/api/admin/users` | Utilizadores (só admin) |
| GET / PUT / POST | `/api/admin/mail` | SMTP e teste |
| WS | `/api/ssh/terminal/ws` | Terminal interativo |

## Estado atual (telas)

| Tela | Disponível |
|------|------------|
| Login de acesso local | Sim |
| Ligação SSH (perfis) | Sim |
| Hub / menu da sessão | Sim |
| Explorador dual (SFTP + Docker, sudo, editor, externo, favoritos, lote, comparar) | Sim |
| Contêineres Docker (lista, reiniciar, logs, stats) | Sim |
| Discos (lsblk + df) | Sim |
| Terminal SSH (WebSocket + xterm) | Sim |
| Automações (regras, motor, histórico) | Sim |
| Admin: utilizadores e SMTP | Sim |
| i18n (PT / EN / ES) na web | Planeado |
| Assistente LVM completo | Só desktop |
| Multi-seleção, drag-and-drop, atalhos, comparar acionável, preview, dock | Disponível (`explorer-enhanced.js`) |

## Interface

- Tema **escuro** / **claro** (`localStorage`)
- Fundo animado (gradientes, grelha, glassmorphism)
- Cabeçalho Remoto: toggle **SFTP** / **Docker** + selector de contêiner compacto
- Toolbar do explorador numa única barra (Ações \| transferências \| Sudo / fila)
- Diálogos centrados; editor com pré-visualização de imagens

## Próximas fases (explorador web)

1. i18n (PT / EN / ES)
2. Assistente LVM completo (paridade desktop)
3. Sincronização espelhada de pastas

## Estrutura

| Caminho | Responsabilidade |
|---------|------------------|
| `cmd/containerway-web/` | Entrada do servidor HTTP |
| `internal/webapp/` | API, sessões, UI em `static/` |
| `internal/webapp/api_editor.go` | Leitura/gravação de ficheiros para editor |
| `internal/webapp/api_open_external.go` | Abrir no Windows e sincronizar remoto |
| `internal/webapp/api_sudo.go` | API sudo |
| `internal/webapp/shellopen.go` | `shellopen` / Notepad++ no Windows |
| `internal/containerfs/` | Listagem/leitura no contêiner (`exec`) |
| `internal/connectcfg/` | `connections.json` |
| `scripts/web/` | Scripts build/run |
