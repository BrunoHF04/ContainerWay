# ContainerWay Web (MVP)

Interface web local que reutiliza o motor Go do ContainerWay (SSH/SFTP, listagens local/remota).

> Desenvolvimento ativo na branch **`dev_browser`**. Quando estiver estável, merge manual para `main`.

## Executar

**Windows (duplo clique ou terminal na raiz do repo):**

```bat
run-web.bat
```

**Linux / macOS:**

```sh
chmod +x run-web.sh build-web.sh scripts/web/*.sh
./run-web.sh
```

Scripts completos em `scripts/web/`. Porta personalizada: `run-web.bat 127.0.0.1:9000` ou `./run-web.sh 127.0.0.1:9000`.

Por omissão escuta em **http://127.0.0.1:8765** (só localhost). Se existir `containerway-web.exe` / `containerway-web` na raiz, usa o executável; senão faz `go run`.

**Compilar:**

```bat
build-web.bat
```

```sh
./build-web.sh
```

**Go direto (sem scripts):**

```powershell
go run ./cmd/containerway-web/
go run ./cmd/containerway-web/ -addr 127.0.0.1:9000
go build -o containerway-web.exe ./cmd/containerway-web/
```

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
| GET | `/api/connections` | Perfis SSH (sem segredos) |
| POST | `/api/ssh/connect` | Ligação SSH/SFTP |
| POST | `/api/ssh/disconnect` | Desligar SSH |
| GET | `/api/ssh/status` | SSH ativo? |
| GET | `/api/local/list?path=` | Listagem local |
| GET | `/api/remote/list?path=` | Listagem remota (SFTP) |

## Estado atual (telas)

| Tela | Disponível |
|------|------------|
| Login de acesso local | Sim |
| Explorador dual (local + remoto) | Sim |
| Hub / módulos da sessão | Não |
| Transferências | Não |
| Terminal SSH | Não |
| Docker, discos, automações | Não |

## Próximas fases

1. Transferências (enviar/receber) e fila de progresso
2. Terminal SSH (WebSocket + xterm.js)
3. Docker, discos, automações
4. Tema claro/escuro e i18n

## Estrutura

| Caminho | Responsabilidade |
|---------|------------------|
| `cmd/containerway-web/` | Ponto de entrada do servidor HTTP |
| `internal/webapp/` | API, sessões, UI embutida em `static/` |
| `internal/connectcfg/` | Perfis `connections.json` (partilhado com desktop) |
| `internal/accessauth/` | Autenticação local |
| `internal/configdir/` | Caminhos de configuração |
| `scripts/web/` | Scripts `build` e `run` (`.bat` / `.sh`) |
| `run-web.bat`, `build-web.bat` | Atalhos na raiz (Windows) |
| `run-web.sh`, `build-web.sh` | Atalhos na raiz (Unix) |
