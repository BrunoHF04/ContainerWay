# ContainerWay

Gestor de ficheiros com painel duplo (estilo WinSCP) para **Windows**: navega no disco local, no **host Linux** via **SFTP/SSH** e dentro de **contentores Docker** na mesma máquina remota — sem expor a API do Docker em TCP.

## Funcionalidades

- **Ligação SSH**: utilizador, senha e/ou chave privada **OpenSSH/PEM** (com passphrase opcional). Ficheiros **PPK** do PuTTY não são suportados diretamente; converta com `puttygen key.ppk -O private-openssh -o key.pem`.
- **Docker sobre SSH**: o cliente Moby usa um dialer que abre `unix:///var/run/docker.sock` no servidor através do canal SSH (stream local), alinhado com o OpenSSH moderno.
- **Host remoto**: listagem e transferências via **SFTP** no mesmo túnel SSH.
- **Contentores**: listagem de contentores, navegação com `ContainerStatPath` + arquivo em **tar** (`CopyFromContainer` / `CopyToContainer`), transferências em **stream** (sem pastas temporárias no host para o fluxo principal de ficheiros).
- **Fila de transferências** com barra de progresso (sequencial).

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

1. Preencha host (ex.: `192.168.1.10` ou `servidor:22`), utilizador e credenciais.
2. Após ligar, escolha **Host (SFTP)** ou um **contentor** no menu superior do painel direito.
3. Use **Abrir pasta** para entrar em diretórios; **Enviar →** / **← Receber** para ficheiros selecionados (nesta versão, apenas ficheiros, não pastas inteiras).

## Estrutura do código

| Pacote | Descrição |
|--------|-----------|
| `cmd/containerway` | Entrada da aplicação |
| `internal/appui` | Interface Fyne (login, explorador, transferências) |
| `internal/session` | SSH, SFTP e cliente Docker sobre socket Unix remoto |
| `internal/hostfs` | Operações SFTP no host |
| `internal/containerfs` | Operações Docker (tar / API de arquivo) |
| `internal/localfs` | Listagem do sistema de ficheiros local |
| `internal/transfer` | Fila e progresso |

## Segurança (nota)

A verificação de chave de host SSH está desativada para protótipo (`InsecureIgnoreHostKey`). Para uso real, configure `known_hosts` ou equivalente.