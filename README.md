# ContainerWay

Gestor de ficheiros com painel duplo (estilo WinSCP) para **Windows**: navega no disco local, no **host Linux** via **SFTP/SSH** e dentro de **contentores Docker** na mesma máquina remota — sem expor a API do Docker em TCP.

## Funcionalidades

- **Ligação SSH**: utilizador, senha e/ou chave **OpenSSH/PEM** ou **PPK** (PuTTY), via [`github.com/kayrus/putty`](https://github.com/kayrus/putty), com passphrase opcional.
- **Chave de host**: `known_hosts` (um ou vários ficheiros separados por `|`) ou opção explícita **Ignorar chave de host** (inseguro; por omissão ligada para facilitar testes).
- **Docker sobre SSH**: o cliente Moby usa um dialer que abre `unix:///var/run/docker.sock` no servidor através do canal SSH (stream local), alinhado com o OpenSSH moderno.
- **Host remoto**: listagem e transferências via **SFTP** no mesmo túnel SSH.
- **Contentores**: listagem de contentores, navegação com `ContainerStatPath` + arquivo em **tar** (`CopyFromContainer` / `CopyToContainer`), transferências em **stream**.
- **Pastas**: envio/recibo recursivo (tar para contentor; árvore SFTP para host; cópia local recursiva).
- **Fila de transferências** com barra de progresso e **vários workers em paralelo** (1–16, configurável no login).

## Stack (resumo)

| Área | Pacotes |
|------|---------|
| UI | [fyne.io/fyne/v2](https://fyne.io/) |
| SSH / known_hosts | `golang.org/x/crypto/ssh`, `golang.org/x/crypto/ssh/knownhosts` |
| SFTP | `github.com/pkg/sftp` |
| PPK | `github.com/kayrus/putty` |
| Docker API | `github.com/docker/docker` (client Moby) |

## Requisitos

- [Go](https://go.dev/dl/) 1.21 ou superior (o `go.mod` do projeto pode fixar uma versão mais recente).
- No **Windows**, compilação gráfica **Fyne** com o driver GLFW: **GCC** no `PATH` (por exemplo [MSYS2](https://www.msys2.org/) com `mingw-w64-x86_64-gcc`).
- No servidor: **OpenSSH** com SFTP; utilizador com permissão de leitura/escrita em `/var/run/docker.sock` (ou grupo `docker`, conforme a política da máquina).

## Compilar

```powershell
.\build.ps1
```

Gera `containerway.exe` na raiz do repositório (`CGO_ENABLED=1`).

Para validar apenas a compilação do código **sem** GUI (CI / máquina sem GCC):

```powershell
go build -tags ci -o containerway_ci.exe ./cmd/containerway/
```

## Executar

```powershell
.\containerway.exe
```

1. Preencha host (ex.: `192.168.1.10` ou `servidor:22`), utilizador e credenciais; opcionalmente `known_hosts`, desmarque **Ignorar chave de host** em produção, e defina **Paralelo** (número de jobs simultâneos).
2. Após ligar, escolha **Host (SFTP)** ou um **contentor** no menu superior do painel direito.
3. Use **Abrir pasta** para entrar em diretórios; **Enviar →** / **← Receber** para **ficheiros ou pastas** selecionados.

## Estrutura do código

| Pacote | Descrição |
|--------|-----------|
| `cmd/containerway` | Entrada da aplicação |
| `internal/appui` | Interface Fyne (login, explorador, transferências) |
| `internal/session` | SSH, SFTP, cliente Docker; `hostkey.go` para `known_hosts` |
| `internal/hostfs` | Operações SFTP no host |
| `internal/containerfs` | Operações Docker (tar / API de arquivo) |
| `internal/localfs` | Listagem do sistema de ficheiros local |
| `internal/tarxfer` | Tar recursivo (local ↔ contentor / SFTP) |
| `internal/transfer` | Fila, progresso e workers paralelos |

## Segurança (nota)

Com **Ignorar chave de host** desligado, o cliente usa `golang.org/x/crypto/ssh/knownhosts` sobre os ficheiros indicados (ou `%USERPROFILE%\.ssh\known_hosts` se existir). Se não houver ficheiros válidos, a ligação falha até indicar caminhos ou voltar a ignorar a verificação.