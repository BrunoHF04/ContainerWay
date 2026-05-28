# ContainerWay Web (MVP)

Interface web local que reutiliza o motor Go do ContainerWay (SSH/SFTP, listagens local/remota).

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

## API (MVP)

| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/api/health` | Estado do serviço |
| POST | `/api/auth/login` | Login local |
| POST | `/api/auth/logout` | Logout |
| GET | `/api/auth/me` | Utilizador atual |
| GET | `/api/connections` | Lista de perfis SSH |
| GET | `/api/connections?name=` | Detalhe de um perfil (para preencher o formulário) |
| POST | `/api/connections` | Guardar/atualizar perfil |
| DELETE | `/api/connections?name=` | Apagar perfil |
| POST | `/api/ssh/connect` | Ligação SSH/SFTP |
| POST | `/api/ssh/disconnect` | Desligar SSH |
| GET | `/api/ssh/status` | SSH ativo? |
| GET | `/api/local/list?path=` | Listagem local |
| GET | `/api/remote/list?path=` | Listagem remota (SFTP) |
| GET | `/api/transfer/status` | Fila de transferências |
| POST | `/api/transfer/push` | Enviar ficheiro/pasta local → remoto (ou contêiner com `containerId`) |
| POST | `/api/transfer/pull` | Receber ficheiro/pasta remoto → local (ou de contêiner) |
| POST | `/api/transfer/batch` | Lote push/pull dos itens visíveis |
| POST | `/api/local/mkdir` | Criar pasta local |
| POST | `/api/local/rename` | Renomear local |
| POST/DELETE | `/api/local/delete` | Apagar local |
| GET | `/api/local/shortcuts` | Atalhos Home/Desktop/… |
| POST | `/api/remote/mkdir` | Criar pasta remota (host ou `containerId`) |
| POST | `/api/remote/rename` | Renomear remoto |
| POST/DELETE | `/api/remote/delete` | Apagar remoto |
| GET | `/api/explorer/compare` | Comparar pastas dos dois painéis |
| GET/PUT | `/api/explorer/favorites` | Favoritos (partilhados com desktop) |
| GET/POST/DELETE | `/api/explorer/clipboard` | Copiar metadados para colar |
| POST | `/api/explorer/paste` | Colar / mover entre painéis |
| GET | `/api/remote/list?containerId=` | Listar dentro de contêiner |
| GET | `/api/docker/containers` | Contêineres em execução |
| POST | `/api/docker/restart` | Reiniciar contêiner (`{"id"}`) |
| GET | `/api/docker/logs?id=` | Logs do contêiner |
| GET | `/api/docker/stats?id=` | Estatísticas do contêiner |
| GET | `/api/disks/summary` | Discos (lsblk + df) |
| GET / PUT | `/api/automations/rules` | Listar / gravar regras do host |
| GET / DELETE | `/api/automations/history` | Histórico / limpar |
| GET / POST | `/api/automations/engine` | Estado do motor (`{"action":"start"|"stop"}`) |
| GET / PUT | `/api/admin/users` | Utilizadores de acesso (só admin) |
| GET / PUT / POST | `/api/admin/mail` | SMTP e teste (`{"mode":"self"|"recipients"}`) |
| WS | `/api/ssh/terminal/ws` | Terminal interativo |

## Estado atual (telas)

| Tela | Disponível |
|------|------------|
| Login de acesso local | Sim |
| Ligação SSH | Sim |
| Hub / menu da sessão (cartões) | Sim |
| Explorador dual (renomear, apagar, nova pasta, copiar/colar, comparar, favoritos, lote, contêiner) | Sim |
| Contêineres Docker (lista, reiniciar, logs, stats) | Sim |
| Discos (lsblk + df) | Sim |
| Terminal SSH (WebSocket + xterm) | Sim |
| Automações (editar regras, motor, histórico) | Sim |
| Configurações (info admin) | Sim |
| Gestão de utilizadores e SMTP (admin) | Sim |
| Transferência para contêineres | Sim (painel remoto → Contêiner) |
| Painel de histórico de transferências (web) | Sim |
| Editor remoto / modo sudo / i18n / LVM completo | Só desktop (fase seguinte) |

## Interface

- Tema **escuro** / **claro** (botão no login e na barra superior; preferência em `localStorage`)
- Fundo animado (gradientes, grelha, glassmorphism)
- Transições entre ecrãs e cartões do hub com entrada escalonada

## Próximas fases

1. Editor remoto integrado, modo sudo, teste de ligação SSH na UI
2. i18n (PT / EN / ES)
3. Assistente LVM completo (como no desktop)

## Estrutura

| Caminho | Responsabilidade |
|---------|------------------|
| `cmd/containerway-web/` | Ponto de entrada do servidor HTTP |
| `internal/webapp/` | API, sessões, UI embutida em `static/` |
| `internal/connectcfg/` | Perfis `connections.json` (partilhado com desktop) |
| `internal/accessauth/` | Autenticação local |
| `internal/configdir/` | Caminhos de configuração |
| `scripts/web/` | Scripts `build` e `run` (`.bat` / `.sh`) |
| `docs/` | Documentação (este ficheiro, [SCRIPTS.md](SCRIPTS.md), etc.) |
