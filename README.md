# ContainerWay

Gestor de arquivos com painel duplo (estilo WinSCP) para **Windows**: navega no disco local, no **host Linux** via **SFTP/SSH** e dentro de **contêineres Docker** na mesma máquina remota — sem expor a API do Docker em TCP.

## Funcionalidades

- **Conexão SSH**: usuário, senha e/ou chave **OpenSSH/PEM** ou **PPK** (PuTTY), via [`github.com/kayrus/putty`](https://github.com/kayrus/putty), com senha da chave opcional.
- **Chave de host**: `known_hosts` (um ou vários arquivos separados por `|`) ou opção explícita **Ignorar chave de host** (inseguro; por padrão ligada para facilitar testes).
- **Docker sobre SSH**: o cliente Moby usa um dialer que abre `unix:///var/run/docker.sock` no servidor através do canal SSH (stream local), alinhado com o OpenSSH moderno.
- **Host remoto**: listagem e transferências via **SFTP** no mesmo túnel SSH.
- **Contêineres**: no menu aparecem só os **em execução**, com nome em destaque e ID curto; a listagem de diretório usa `docker exec ls` (não-recursivo) para abrir pastas com resposta rápida e sem travar.
- **Pastas**: envio/recebimento recursivo (tar para contêiner; árvore SFTP para host; cópia local recursiva).
- **Fila de transferências** com barra de progresso e **vários workers em paralelo** (1–16, configurável no login).
- **Conexões salvas**: na tela inicial é possível salvar, carregar e excluir perfis de conexão (com opção de guardar senha/chave localmente).
- **Interface**: tema escuro com acento ciano, layout mais compacto no explorador, botões de navegação por painel, pesquisa local/remota, breadcrumbs clicáveis e ícone da aplicação/janela.
- **Ações de arquivo**: duplo clique em pasta abre; duplo clique em arquivo local abre no app padrão; arquivo remoto pode abrir para edição remota com sincronização automática ao salvar (estilo WinSCP).
- **Gestão de itens**: renomear, excluir e criar pasta no painel ativo (menu de contexto e atalhos).
- **Modo sudo no host remoto**: ao receber `permission denied`, o app pode solicitar credenciais para elevar acesso, listando/abrindo/editando/transferindo arquivos protegidos no host.

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

Gera `containerway.exe` na raiz do repositório (`CGO_ENABLED=1`). **Sempre use este `.exe` recém-gerado** depois de atualizar o código; uma cópia antiga no Ambiente de trabalho ou outra pasta continua com o comportamento antigo.

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
3. Use os perfis em **Conexões salvas** para preencher o login mais rápido e clique em **Conectar**.
4. No explorador, use **duplo clique** para abrir pasta ou o botão **Abrir**; **Enviar** / **Receber** para transferências.

### No explorador (uso simples)

- **Esquerda**: arquivos do seu **computador local** (com atalhos rápidos para Diretório inicial, Desktop, Documentos, Downloads).
- **Direita**: escolha **pastas do servidor** (SFTP) ou um **contêiner em execução**; o conteúdo mostrado sempre acompanha o contexto selecionado.
- Cada painel tem barra de navegação com **voltar**, **subir**, **início** e **atualizar**, além de busca por nome.
- **Breadcrumb clicável** permite saltar direto para qualquer nível de pasta.
- **Menu de contexto** (botão direito) oferece abrir, transferir e atualizar lista.
- **Atalhos de teclado**:
  - `Enter`: abre pasta no painel ativo
  - `Backspace`: sobe um nível
  - `Tab`: alterna painel ativo (local/servidor)
  - `F3` e `Ctrl+F`: foca a busca do painel ativo
  - `F5`: atualiza os dois painéis
  - `F6`: transfere conforme o painel ativo (`Enviar` no local / `Receber` no servidor)
  - `F2`: renomear item selecionado
  - `Del`: excluir item selecionado (com confirmação)
  - `Ctrl+Shift+N`: criar nova pasta no painel ativo
- **Busca inteligente por painel**: ao navegar para outra pasta, o campo de busca do painel é limpo automaticamente para evitar lista “vazia” por filtro antigo.

### Pastas protegidas (sudo)

- Ao entrar em pasta remota com restrição de permissão, o app exibe o diálogo **Acesso negado**.
- Você pode informar usuário/senha para tentativa de elevação com `sudo`.
- Se o usuário informado não elevar para `uid=0`, o app tenta fallback para `root` automaticamente com a mesma senha.
- Com sudo ativo, o lado remoto passa a suportar:
  - listagem de diretórios protegidos,
  - abrir/editar arquivo remoto com sincronização,
  - enviar e receber arquivos protegidos.

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
