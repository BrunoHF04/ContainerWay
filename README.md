# ContainerWay

Gestor de arquivos com painel duplo (estilo WinSCP) para **Windows**: navega no disco local, no **host Linux** via **SFTP/SSH** e dentro de **contêineres Docker** na mesma máquina remota — sem expor a API do Docker em TCP.

## Funcionalidades

- **Conexão SSH**: usuário, senha e/ou chave **OpenSSH/PEM** ou **PPK** (PuTTY), via [`github.com/kayrus/putty`](https://github.com/kayrus/putty), com senha da chave opcional.
- **Chave de host**: `known_hosts` (um ou vários arquivos separados por `|`) ou opção explícita **Ignorar chave de host** (inseguro; por padrão ligada para facilitar testes).
- **Docker sobre SSH**: o cliente Moby usa um dialer que abre `unix:///var/run/docker.sock` no servidor através do canal SSH (stream local), alinhado com o OpenSSH moderno.
- **Host remoto**: listagem e transferências via **SFTP** no mesmo túnel SSH.
- **Contêineres**: no menu aparecem só os **em execução**, com nome em destaque e ID curto; navegação com `ContainerStatPath` + arquivo em **tar** (`CopyFromContainer` / `CopyToContainer`), transferências em **stream**.
- **Pastas**: envio/recebimento recursivo (tar para contêiner; árvore SFTP para host; cópia local recursiva).
- **Fila de transferências** com barra de progresso e **vários workers em paralelo** (1–16, configurável no login).
- **Interface**: tema escuro com acento ciano, cartão no login compacto, ícones nas listas e na barra de ferramentas (Fyne); ícone da aplicação/janela (PNG 64×64 gerado em código).

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
- No **Windows**, a UI **Fyne** usa **CGO** (GLFW): é preciso **GCC ou Clang** no `PATH`. Opções comuns:
  - [MSYS2](https://www.msys2.org/) com o pacote `mingw-w64-x86_64-gcc`; ou
  - **LLVM-MinGW** via Winget: `winget install MartinStorsjo.LLVM-MinGW.UCRT` (feche e reabra o terminal depois da instalação).
  O script `build.ps1` define `CGO_ENABLED=1` e recarrega o `PATH` do sistema e do usuário antes de compilar.
- No servidor: **OpenSSH** com SFTP; usuário com permissão de leitura/escrita em `/var/run/docker.sock` (ou grupo `docker`, conforme a política da máquina).

A interface da aplicação está em **português do Brasil (pt-BR)**.

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

1. Preencha host (ex.: `192.168.1.10` ou `servidor:22`), usuário e credenciais; opcionalmente `known_hosts`, desmarque **Ignorar chave de host** em produção, e defina **Paralelismo** (número de transferências simultâneas).
2. Após conectar, no menu do lado direito escolha **pastas do servidor** ou um **contêiner em execução** (só os ligados aparecem).
3. Use **Abrir** para entrar em pastas; **Enviar** / **Receber** para **arquivos ou pastas** selecionados.

### No explorador (uso simples)

- **Esquerda**: arquivos do seu **computador local**.
- **Direita**: um texto de ajuda indica que só aparecem **contêineres ligados** (em execução). A primeira opção do menu são as **pastas do servidor fora dos contêineres** (SFTP no Linux remoto); abaixo vêm os contêineres, cada um como **`nome (ID curto)`** (nomes muito longos são encurtados com `…`).
- A barra abaixo do menu mostra a **pasta atual** no servidor ou **dentro do contêiner** (com o ID), para saber sempre onde está.

## Estrutura do código

| Pacote | Descrição |
|--------|-----------|
| `cmd/containerway` | Entrada da aplicação |
| `internal/appui` | Interface Fyne (login, explorador, transferências) |
| `internal/session` | SSH, SFTP, cliente Docker; `hostkey.go` para `known_hosts` |
| `internal/hostfs` | Operações SFTP no host |
| `internal/containerfs` | Operações Docker (tar / API de arquivo) |
| `internal/localfs` | Listagem do sistema de arquivos local |
| `internal/tarxfer` | Tar recursivo (local ↔ contêiner / SFTP) |
| `internal/transfer` | Fila, progresso e workers paralelos |

## Segurança (nota)

Com **Ignorar chave de host** desligado, o cliente usa `golang.org/x/crypto/ssh/knownhosts` sobre os arquivos indicados (ou `%USERPROFILE%\.ssh\known_hosts` se existir). Se não houver arquivos válidos, a conexão falha até informar caminhos ou marcar de novo a opção de ignorar a verificação.
