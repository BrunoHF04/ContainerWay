# ContainerWay

Gestor de arquivos de painel duplo (estilo WinSCP) para **Windows**, com foco em uso prático no dia a dia:

- painel esquerdo no **computador local**;
- painel direito no **host Linux** via **SSH/SFTP** ou dentro de **contêineres Docker** remotos;
- Docker acessado pelo socket Unix remoto (`/var/run/docker.sock`) **sobre SSH**, sem abrir API Docker em TCP.

Toda a interface está em **pt-BR**.

## Acesso local ao aplicativo

Antes da tela de conexão SSH/SFTP, o app pede **login de acesso local** (usuário e senha armazenados nas preferências do app no Windows).

- **Usuário padrão:** `admin`
- **Senha padrão:** `!q1w2e3r4$`
- **Cadastro de usuários:** após entrar como `admin`, use o botão **Usuários** na barra superior do explorador para criar, atualizar ou remover usuários de acesso local (o `admin` não pode ser removido).
- **Logs:** o nome exibido nos logs segue o cadastro de cada usuário.

> Atenção: isso **não** substitui autenticação do servidor SSH; é apenas uma trava local do app. Em ambientes sensíveis, altere a senha do `admin` e cadastre usuários com senhas fortes.

## Visão geral das funcionalidades

### Conexão e login

- Conexão SSH com:
  - senha;
  - chave **OpenSSH/PEM**;
  - chave **PPK** (PuTTY), incluindo senha da chave.
- Configuração de verificação de host:
  - `known_hosts` (um ou mais arquivos separados por `|`);
  - ou opção explícita para ignorar chave de host (inseguro).
- Perfis de conexão:
  - salvar, carregar e excluir conexão;
  - manter segredo no disco (opcional) ou lembrar só na sessão atual.
- Teste rápido de conexão com status por etapa:
  - `SSH`;
  - `SFTP`;
  - `Docker`.
- Validação visual em tempo real no login:
  - host obrigatório;
  - usuário obrigatório;
  - senha ou chave obrigatória;
  - paralelismo entre `1` e `16`;
  - caminho da chave válido quando preenchido.
- Layout de login por abas:
  - `Conexões SSH/SFTP`;
  - `Chave e segurança`.

### Navegação e usabilidade no explorador

- Navegação por dois painéis:
  - esquerda: computador local;
  - direita: host remoto ou contêiner selecionado.
- Ações principais no topo:
  - `Enviar`;
  - `Receber`;
  - `Histórico`;
  - `?` (manual completo do sistema).
- Barra de navegação por painel com:
  - voltar;
  - subir nível;
  - início;
  - atualizar.
- Breadcrumbs clicáveis para navegação rápida por níveis.
- Busca por painel (local e remoto) com:
  - texto por nome;
  - filtro por extensão (`ext:log`);
  - filtro por tipo (`tipo:pasta`, `tipo:arquivo`);
  - seletor rápido (`Tudo`, `Pastas`, `Arquivos`);
  - limpeza automática ao trocar de pasta/contexto.
- Favoritos por painel:
  - adicionar pasta atual (`+`);
  - remover pasta atual (`-`);
  - persistência entre sessões.
- Duplo clique:
  - pasta abre;
  - arquivo local abre no app padrão do Windows.
- Edição remota estilo WinSCP:
  - abre arquivo remoto localmente;
  - monitora alterações;
  - envia de volta automaticamente ao salvar.
- Menus de contexto com ações de:
  - abrir;
  - enviar/receber;
  - atualizar;
  - copiar/colar;
  - renomear/excluir/criar pasta (conforme painel/contexto).
- Atalhos em diálogos de formulário:
  - `Enter` confirma;
  - `Esc` cancela.

### Transferências

- Transferência de arquivos e pastas:
  - local ↔ host;
  - local ↔ contêiner;
  - host ↔ contêiner (via fluxo interno de transferência).
- Suporte recursivo para diretórios.
- Drag-and-drop entre painéis para iniciar envio/recebimento.
- Copiar/colar entre painéis pelo menu de contexto.
- Transferência em lote de itens visíveis:
  - `Enviar visíveis`;
  - `Receber visíveis`;
  - confirmação antes de executar;
  - progresso por itens concluídos.
- Fila de transferências com:
  - progresso;
  - status de tarefa;
  - workers paralelos configuráveis (`1` a `16`).

### Histórico, retry e log geral

- Janela de histórico com abas:
  - `Sessão`;
  - `Log geral`.
- Filtro por texto no histórico e no log geral.
- Exportação de histórico filtrado para `.log`.
- Ações de recuperação:
  - tentar novamente última falha;
  - tentar novamente todas as falhas (com confirmação).
- Abertura rápida de:
  - arquivo do log geral;
  - pasta de logs.
- Persistência:
  - histórico de operações salvo entre sessões;
  - log geral acumulativo em arquivo com níveis `INFO` e `ERROR`.

### Docker remoto

- Lista apenas contêineres **em execução**.
- Identificação amigável com nome e ID curto no seletor.
- Listagem de diretório preferencial por `docker exec ls -1Ap` (direta e rápida).
- Fallback para método por tar quando necessário.

### Ordenação de itens

- Ordenação estilo WinSCP:
  - `..` no topo;
  - depois pastas em ordem alfabética;
  - depois arquivos em ordem alfabética.

### Suporte a sudo em pastas protegidas (host remoto)

- Ao detectar permissão negada, o app pode abrir fluxo de elevação.
- Usuário informa credenciais sudo em diálogo.
- Fallback automático para `root` quando usuário informado não eleva para `uid=0`.
- Indicador visual de sudo ativo + ação para desativar.
- Cache temporário de validação sudo durante a sessão (TTL interno), evitando pedir senha repetidamente.
- Mensagens mais didáticas para cenários comuns (ex.: senha incorreta, usuário sem sudo, requisito de TTY).
- Operações com sudo ativo:
  - listagem de diretório;
  - abrir/editar arquivo;
  - upload/download de arquivo;
  - upload/download recursivo de pasta.

## Atalhos de teclado

- `Enter`: abrir pasta no painel ativo.
- `Backspace`: subir um nível no painel ativo.
- `Tab`: alternar foco entre painel esquerdo e direito.
- `F3` / `Ctrl+F`: focar busca do painel ativo.
- `F5`: atualizar painéis.
- `F6`: transferir conforme o painel ativo (`Enviar` / `Receber`).
- `Ctrl+Shift+F6`: transferir itens visíveis em lote no painel ativo.
- `F2`: renomear item selecionado.
- `Del`: excluir item selecionado (com confirmação).
- `Ctrl+Shift+N`: criar pasta no painel ativo.

## Tecnologias utilizadas

| Área | Tecnologias / pacotes |
|------|------------------------|
| Linguagem | Go |
| UI desktop | [fyne.io/fyne/v2](https://fyne.io/) |
| SSH | `golang.org/x/crypto/ssh` |
| Host key (`known_hosts`) | `golang.org/x/crypto/ssh/knownhosts` |
| SFTP | `github.com/pkg/sftp` |
| Chave PPK (PuTTY) | [`github.com/kayrus/putty`](https://github.com/kayrus/putty) |
| Docker remoto | `github.com/docker/docker` (cliente Moby) |
| Concorrência | goroutines, `sync`, `sync/atomic` |

## Requisitos

- [Go](https://go.dev/dl/) 1.21+ (ou versão definida no `go.mod`).
- No Windows, Fyne requer **CGO**:
  - GCC ou Clang no `PATH`.
- Opções comuns no Windows:
  - [MSYS2](https://www.msys2.org/) + `mingw-w64-x86_64-gcc`;
  - LLVM-MinGW via Winget:
    - `winget install MartinStorsjo.LLVM-MinGW.UCRT`
- O `build.ps1` já:
  - recarrega `PATH` de usuário/sistema;
  - define `CGO_ENABLED=1`;
  - compila com `-H=windowsgui` (sem console abrindo junto).
- Servidor remoto:
  - OpenSSH com SFTP;
  - permissão de acesso ao Docker socket (`/var/run/docker.sock`) quando for usar contêineres.

## Compilação

```powershell
.\build.ps1
```

Saída: `ContainerWay.exe` na raiz.

Build de validação sem GUI (CI/ambiente sem GCC para Fyne):

```powershell
go build -tags ci -o containerway_ci.exe ./cmd/containerway/
```

## Execução

```powershell
.\ContainerWay.exe
```

Fluxo recomendado:

1. Abra a aba `Conexões SSH/SFTP`.
2. Selecione uma conexão salva ou preencha uma nova.
3. Ajuste `Chave e segurança` (se necessário).
4. Use `Testar conexão` para validar acesso.
5. Clique em `Conectar`.
6. No painel direito, escolha contexto:
   - pastas do servidor;
   - ou um contêiner em execução.

## Estrutura do projeto

| Caminho | Responsabilidade |
|---------|------------------|
| `cmd/containerway` | Ponto de entrada do app |
| `internal/appui` | Interface Fyne (login, explorador, ações, atalhos) |
| `internal/session` | Conexão SSH, cliente SFTP e cliente Docker |
| `internal/hostfs` | Operações no host remoto via SFTP |
| `internal/containerfs` | Operações em arquivos de contêiner |
| `internal/localfs` | Operações no sistema de arquivos local |
| `internal/fsutil` | Utilitários de entrada/listagem e ordenação |
| `internal/tarxfer` | Transferências recursivas com tar |
| `internal/transfer` | Fila, progresso e workers de transferência |

## Segurança

- Em produção, prefira validação de host por `known_hosts` e evite `Ignorar chave de host`.
- Segredos podem ser:
  - persistidos na conexão local (quando marcado);
  - ou mantidos somente na sessão atual (não persistente em disco).
- Fluxo sudo é usado apenas quando necessário para acesso a caminhos protegidos.
