# ContainerWay

![ContainerWay - Gestão de Arquivos e Contêineres Remotos](assets/containerway-banner-simple.png)

Gestor de arquivos de painel duplo (estilo WinSCP), com foco em uso prático no dia a dia. O fluxo principal é no **Windows**; também é possível gerar **pacotes Linux** (`.deb` e `.flatpak`) pelo `scripts/build.ps1` para rodar o app em desktop Linux com X11/Wayland:

- painel esquerdo no **computador local**;
- painel direito no **host Linux** via **SSH/SFTP** ou dentro de **contêineres Docker** remotos;
- **Docker** ou API **compatível** (ex.: **Podman**) no servidor, via socket Unix remoto (por omissão `/var/run/docker.sock`) **sobre SSH**, sem expor a API em TCP; o caminho do socket é configurável por ligação.

Toda a interface está em **pt-BR**.

> **ContainerWay Web (browser):** versão em desenvolvimento na branch `dev_browser` — servidor HTTP local + UI no browser (sem CGO/OpenGL). Ver **[docs/WEB_UI.md](docs/WEB_UI.md)**. Scripts: `scripts/web/run.bat` / `scripts/web/run.sh` — ver [SCRIPTS.md](SCRIPTS.md).

## Navegacao rapida

<details>
<summary><strong>Acesso local ao aplicativo</strong></summary>

Antes da tela de conexão SSH/SFTP, o app pede **login de acesso local** (usuário e senha armazenados nas preferências do app no Windows).

- **Usuário padrão:** `admin`
- **Senha padrão:** `!q1w2e3r4$`
- **Cadastro de usuários:** após entrar como `admin`, use **Usuários** na tela inicial da sessão (cartão **Configurações**) ou na barra superior do explorador. A tela de gestão abre **em janela maximizada**, com **Voltar** para retornar ao hub ou ao explorador. O `admin` não pode ser removido; no mesmo lugar há o atalho **Abrir configuração de alertas por e-mail…**.
- **Logs:** o nome exibido nos logs segue o cadastro de cada usuário.
- **Primeiro acesso:** após o primeiro login de acesso com sucesso, o app pode mostrar um diálogo curto de boas-vindas (dicas de segurança e onde configurar política opcional); não volta a aparecer após confirmado.

> Atenção: isso **não** substitui autenticação do servidor SSH; é apenas uma trava local do app. Em ambientes sensíveis, altere a senha do `admin` e cadastre usuários com senhas fortes. Resumo de política local e ameaças: ver **[SECURITY.md](SECURITY.md)**.

</details>

<details>
<summary><strong>Após conectar (tela inicial da sessão)</strong></summary>

Depois de **Conectar** com sucesso, abre-se primeiro a **tela inicial da sessão** (janela compacta e centralizada), com:

- **Pesquisar módulos** (filtro por palavra-chave);
- **Gerenciador de arquivos** — abre o explorador em painel duplo (no Windows/macOS a janela tende a **maximizar**); ver também **[ARQUIVOS.md](ARQUIVOS.md)**;
- **Contêineres Docker** — lista e ações no host remoto;
- **Discos e armazenamento** — `lsblk`, `df`, LVM (ampliar/reduzir, snapshots, verificar FS, SMART, uso por pasta);
- **Serviços** — lista e controlo de unidades **systemd** no host (start/stop/restart, logs);
- **Central de automações** — regras com gatilho/ação, motor de execução e histórico operacional;
- **Terminal SSH** — console remoto integrado para executar comandos no host;
- **Configurações** (somente **admin**) — atalhos para **Usuários** e **Alertas por e-mail**.

No explorador, o botão **Início** volta para essa tela **sem desconectar**. O botão **Sair** encerra a sessão SSH e retorna ao login de acesso local (a janela volta ao tamanho adequado).


</details>

<details>
<summary><strong>Alertas por e-mail (admin)</strong></summary>

Somente o usuário **admin** vê **E-mail** na barra do explorador ou no cartão **Configurações** do hub. A configuração abre em **tela cheia maximizada**, com **Voltar** no topo.

- ativar o envio e cadastrar **vários destinatários** (lista com **Adicionar**, **Remover selecionado** e **Remover e-mail digitado** — este último dispensa seleção na lista);
- alterações na lista são **gravadas automaticamente** no disco; se o envio estiver ativo e a lista ficar vazia, o app **desativa o envio** e informa (evita estado inválido);
- painel em **duas colunas** (destinatários | SMTP): texto de ajuda em bloco **rolável e compacto**; host e porta na mesma linha;
- **Salvar** (demais campos SMTP), **Enviar teste** e **Teste só no remetente (Gmail)** na barra inferior;
- a lista de destinatários em disco usa JSON; o legado de um único e-mail não sobrescreve mais uma lista vazia.

**Quando o app envia e-mails**

1. **Após o login de acesso local** com sucesso: aviso de quem entrou (conta, nome exibido, computador, data/hora).
2. **Ao encerrar a sessão do explorador** (botão **Sair**, fechar a janela no explorador ou encerrar após erro ao listar Docker): mensagem com o **registro daquela sessão** (mesmo formato do log de atividades). O corpo pode ser **truncado** (últimas linhas) se a sessão for muito longa; o arquivo completo continua no log em disco.

**Dicas (Gmail / Microsoft 365)**

- Gmail costuma usar `smtp.gmail.com`, porta **587**, conta com verificação em duas etapas e **senha de app** (pode colar com espaços; o app remove na hora de autenticar).
- O **From** deve ser coerente com a conta usada no SMTP.
- Mensagens para domínios corporativos (`@empresa`) podem ir para **spam**, **lixo** ou **quarentena** do Exchange; o teste só no remetente ajuda a ver se o Gmail aceita o envio.

**Privacidade:** usuário, senha SMTP e destinatários ficam nas **preferências locais** do aplicativo no Windows (como as credenciais de acesso local), não no repositório do projeto.

> Encerrar o processo pelo Gerenciador de Tarefas ou fechar o app na tela de login pode impedir o envio do resumo de sessão.


</details>

<details>
<summary><strong>Visão geral das funcionalidades</strong></summary>

### Conexão e login

- Conexão SSH com:
  - senha;
  - chave **OpenSSH/PEM**;
  - chave **PPK** (PuTTY), incluindo senha da chave.
- Configuração de verificação de host:
  - `known_hosts` (um ou mais arquivos separados por `|`);
  - ou opção explícita para ignorar chave de host (inseguro).
- **Política local (opcional, corporativa):** impedir «Ignorar chave de host» via variável de ambiente `CONTAINERWAY_FORBID_INSECURE_HOSTKEY=1` (ou `true`/`yes`) e/ou ficheiro `%APPDATA%\ContainerWay\policy.json` (Windows) / pasta de configuração do utilizador + `ContainerWay/policy.json` com `{"forbidInsecureHostKey":true}`. Com política ativa, a opção insegura fica desativada na UI.
- **Socket Docker/Podman (remoto):** na aba `Chave e segurança`, campo opcional para o caminho Unix no servidor (útil para Podman em `/run/user/.../podman/podman.sock`); o valor é guardado no perfil de ligação.
- Perfis de conexão:
  - salvar, carregar e excluir conexão;
  - manter segredo no disco (opcional) ou lembrar só na sessão atual.
- Teste rápido de conexão com status por etapa:
  - `SSH`;
  - `SFTP`;
  - `Docker/Podman` (API no socket configurado).
- Validação visual em tempo real no login:
  - host obrigatório;
  - usuário obrigatório;
  - senha ou chave obrigatória;
  - paralelismo entre `1` e `16`;
  - caminho da chave válido quando preenchido.
- Layout de login por abas:
  - `Conexões SSH/SFTP`;
  - `Chave e segurança`.
- **Tema:** `Padrão do sistema`, `Claro` ou `Escuro`; com padrão do sistema, o app **reaplica** o tema quando as definições do SO mudam (listener Fyne).

### Navegação e usabilidade no explorador

- Navegação por dois painéis:
  - esquerda: computador local;
  - direita: host remoto ou contêiner selecionado.
- Ações principais no topo:
  - **Início** (volta à tela inicial da sessão);
  - `Enviar`;
  - `Receber`;
  - `Histórico`;
  - **Comparar** (relatório entre as pastas atuais dos dois painéis: nomes, tipo, tamanho e data de modificação);
  - `?` (manual completo do sistema);
  - **Usuários** e **E-mail** (somente para o usuário `admin`).
- Barra de navegação por painel com:
  - voltar;
  - subir nível;
  - início;
  - atualizar.
- Breadcrumbs clicáveis para navegação rápida por níveis.

### Central de automacoes (MVP)

- Regras por host com persistência em JSON local (`automations-<host>.json`).
- Motor de automações em background (liga/desliga na UI) com varredura periódica.
- Gatilho implementado: contêiner parado -> ação de `docker restart` com cooldown.
- **Webhook opcional (HTTPS):** por regra, URL para **POST JSON** após reinício bem-sucedido (`source`, `rule`, `target`, `message`, `time`); falhas registam-se no log de auditoria.
- Botão **Políticas:** resume política local (`policy.json` / variável de ambiente) e o estado «bloquear ignorar chave de host».
- Retry com backoff para falhas de restart (até 3 tentativas).
- Gestão de regras na tela: criar, editar, ativar/desativar e excluir.
- Histórico operacional na própria tela (inclui ações manuais e eventos do motor), com exportação `.txt` e limpeza.
- Busca por painel (local e remoto) com:
  - texto por nome;
  - filtro por extensão (`ext:log`);
  - filtro por tipo (`tipo:pasta`, `tipo:arquivo`);
  - seletor rápido (`Tudo`, `Pastas`, `Arquivos`);
  - limpeza automática ao trocar de pasta/contexto.
- Favoritos por painel:
  - adicionar pasta atual (`+`);
  - remover pasta atual (`-`);
  - **local:** lista global de atalhos;
  - **servidor/contêiner:** atalhos guardados por **host e contexto** (host vs contêiner), com compatibilidade com a lista global antiga para leitura;
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
  - status de tarefa e texto com **fila pendente** e **jobs em execução** (`[fila:N exec:M] …`);
  - workers paralelos configuráveis (`1` a `16`).
- Envio de **ficheiro único** para o host por SFTP (sem sudo): se o destino já existir com o **mesmo tamanho**, o upload é **omitido** (evento no log de auditoria).

### Histórico, retry e log geral

- Janela de histórico com abas:
  - `Sessão`;
  - `Log geral`.
- Filtro por texto no histórico e no log geral.
- Exportação de histórico filtrado para `.log`.
- **Exportar trilha CSV:** gera `containerway-audit.csv` (na pasta dos logs) a partir do log geral filtrado, com colunas `timestamp`, `level`, `scope`, `operator`, `message`.
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

- O cliente usa a API **compatível com Docker** no socket Unix remoto (Docker ou Podman, conforme o caminho configurado na ligação).
- Lista apenas contêineres **em execução**.
- Identificação amigável com nome e ID curto no seletor.
- Listagem de diretório preferencial por `docker exec ls -1Ap` (direta e rápida).
- Fallback para método por tar quando necessário.
- No gerenciador de contêineres, os botões de reinício priorizam **recriação via Compose**:
  - tenta `docker compose up -d --force-recreate --pull always <serviço>`;
  - fallback para `docker compose ... --force-recreate` e `docker-compose ...`;
  - quando o contêiner não é Compose, usa `docker restart` como fallback.

### Discos e armazenamento no servidor

- Acesso pelo cartão **Discos e armazenamento** na tela inicial da sessão (também encontrável na pesquisa de módulos).
- Dados obtidos por script remoto: `lsblk -J`, `df`, mapeamento `df` ↔ dispositivos (`readlink -f`), blocos LVM (`lvs`/`vgs`/`pvs` quando disponíveis).
- Aba **Armazenamento no host**: lista com dispositivo, tipo, montagem, tamanho e barra de uso quando há `df`; filtro de texto; ordenação; opção de mostrar dispositivos loop (Snap); actualização manual e opcional automática a cada 90 s com a aba visível.
- Aba **Assistente LVM** (web: paridade alargada; desktop: ampliação): escolha de LV, resumo do VG (total/livre), modo de tamanho **relativo (+/- GiB)** ou **absoluto**, ampliar e **reduzir** LV, snapshots (criar/listar/remover), verificar FS, redimensionar só o FS, fstrim, renomear/criar LV, activar/desactivar VG, atalho **Analisar montagem** (aba Arquivos). Requer **sudo** para alterações.
- Aba **Armazenamento no host** (web): seleccionar linha e consultar **SMART** do disco físico.
- Aba **Arquivos** (web): uso por pasta (estilo TreeSize), com sudo em caminhos protegidos.
- Aba **Detalhe técnico**: saída bruta (`df`, LVS, VGS, PVS) para depuração.
- Layout da lista usa `Border` no Fyne (em vez de `HBox` sozinho) para a coluna **Tamanho / uso** ocupar o espaço horizontal restante; a barra de progresso fica à esquerda dessa zona e o texto expande à direita.

### Terminal SSH (integrado)

- Acesso direto pelo cartão **Terminal SSH** no hub e pelo botão **Terminal** no explorador.
- Sessão SSH interativa reaproveitando a conexão já autenticada no login.
- Suporte de entrada para:
  - digitação contínua no terminal;
  - `Tab` (autocomplete);
  - setas, `Home`, `End`, `Delete`, `PageUp` e `PageDown`;
  - `Ctrl+C` (botão e atalho de teclado).
- Botão **Limpar** envia `clear` para o shell remoto.
- Modo de execução com fallback:
  - tenta renderização ANSI/VT para melhor compatibilidade visual;
  - se houver falha no ambiente, cai automaticamente para o modo de compatibilidade estável (sem encerrar o app).

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


</details>

<details>
<summary><strong>Atalhos de teclado</strong></summary>

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

</details>

<details>
<summary><strong>ContainerWay Web (interface no browser)</strong></summary>

Versão **experimental** (branch `dev_browser`): o mesmo motor Go (SSH/SFTP) com UI web local.

| Item | Detalhe |
|------|---------|
| Documentação | [docs/WEB_UI.md](docs/WEB_UI.md) |
| Executar (Windows) | `scripts\web\run.bat` |
| Executar (Linux/macOS) | `scripts/web/run.sh` |
| Compilar | `scripts\web\build.bat` / `scripts/web/build.sh` |
| URL padrão | http://127.0.0.1:8765 (apenas localhost) |
| Binário | `containerway-web.exe` (Windows) / `containerway-web` (Linux) na raiz, após build |

**Já disponível:**

- Login de acesso local e perfis SSH;
- Hub com cartões (explorador, Docker, discos, terminal, automações, admin);
- Explorador em painel duplo: local + remoto (**SFTP** ou ficheiros no **contêiner** Docker);
- Transferências (enviar/receber, lote, fila e histórico);
- Sudo no host SFTP; editor de texto e pré-visualização de imagens; abrir com programa predefinido / Notepad++ (Windows);
- Favoritos, comparar pastas, copiar/colar entre painéis;
- Ao mudar de contêiner ou de SFTP para Docker, a pasta remota **reinicia em `/`**;
- Módulos Docker (lista, logs, stats), discos (assistente LVM completo na web), serviços systemd, terminal WebSocket, automações e gestão de utilizadores/SMTP (admin).
- Explorador: confirmação antes de **Enviar**, **Receber** e transferências em **lote**.

**Ainda só no desktop (ou em evolução na web):** i18n completo de todos os textos novos; redução de LV na UI Fyne do assistente; algumas acções do diálogo «Comparar pastas» sem confirmação individual.

Documentação do explorador: [ARQUIVOS.md](ARQUIVOS.md). API e UI web: [WEB_UI.md](WEB_UI.md).

</details>

<details>
<summary><strong>Tecnologias utilizadas</strong></summary>

| Área | Tecnologias / pacotes |
|------|------------------------|
| Linguagem | Go |
| UI desktop | [fyne.io/fyne/v2](https://fyne.io/) |
| UI web (MVP) | HTML/CSS/JS embutido em `internal/webapp/static/` + API HTTP Go |
| SSH | `golang.org/x/crypto/ssh` |
| Host key (`known_hosts`) | `golang.org/x/crypto/ssh/knownhosts` |
| SFTP | `github.com/pkg/sftp` |
| Chave PPK (PuTTY) | [`github.com/kayrus/putty`](https://github.com/kayrus/putty) |
| E-mail (SMTP) | `net/smtp` (pacote interno `internal/mailnotify`) |
| Docker remoto | `github.com/docker/docker` (cliente Moby) |
| Emulação de terminal ANSI/VT | `github.com/hinshun/vt10x` |
| Concorrência | goroutines, `sync`, `sync/atomic` |


</details>

<details>
<summary><strong>Requisitos</strong></summary>

- [Go](https://go.dev/dl/) na versão indicada no `go.mod` (hoje **1.26.1+**).
- No Windows, Fyne requer **CGO**:
  - GCC ou Clang no `PATH`.
- Para gerar **`.deb` e `.flatpak`** com `scripts/build.ps1`: **Docker Desktop** em execução (o script sobe um container Linux com Go, `dpkg-deb` e `flatpak-builder`; o container roda com `--privileged` para o empacotamento Flatpak funcionar no Docker).
- Opções comuns no Windows:
  - [MSYS2](https://www.msys2.org/) + `mingw-w64-x86_64-gcc`;
  - LLVM-MinGW via Winget:
    - `winget install MartinStorsjo.LLVM-MinGW.UCRT`
- O `scripts/build.ps1` já:
  - recarrega `PATH` de usuário/sistema;
  - define `CGO_ENABLED=1`;
  - compila com `-H=windowsgui` (sem console abrindo junto).
- Execução no Windows Server (RDP/VM):
  - o app detecta falha de OpenGL automaticamente;
  - baixa runtime de fallback (`opengl32sw-64.7z`), extrai e relança o processo;
  - cria `opengl32.dll` local quando necessário e marca o arquivo como oculto.
- Servidor remoto:
  - OpenSSH com SFTP;
  - permissão de leitura/escrita no socket da API de contentores quando for usar o explorador de contêineres (por omissão `/var/run/docker.sock`; **Podman** costuma expor socket noutro caminho, configurável na ligação).


</details>

<details>
<summary><strong>Compilação</strong></summary>

```powershell
.\scripts\build.ps1
```

Saídas típicas (após o build concluir):

| Plataforma | Caminho |
|------------|---------|
| Windows | `ContainerWay.exe` na raiz do repositório |
| Linux (Debian/Ubuntu) | `dist/deb/containerway_<versão>_amd64.deb` |
| Linux (Flatpak bundle) | `dist/flatpak/containerway-<versão>.flatpak` |

A **versão** dos pacotes Linux vem de `-Version`, se informado; senão, de `git describe --tags --always`. Para nomes só com hash de commit, o `.deb` usa prefixo Debian `0~` (ex.: `containerway_0~abc1234_amd64.deb`). Os diretórios `dist/` e `.flatpak-builder/` ficam no `.gitignore` (não entram no Git).

**Instalação rápida no Linux (máquina de destino):**

- Pacote Debian: `sudo dpkg -i dist/deb/containerway_<versão>_amd64.deb` (instale dependências faltantes com `sudo apt-get install -f` se o `dpkg` avisar).
- Flatpak: `flatpak install --bundle dist/flatpak/containerway-<versão>.flatpak`

Opções úteis:

```powershell
# Definir versão do pacote (aparece no nome do .deb e do .flatpak)
.\scripts\build.ps1 -Version 1.2.3

# Build somente Windows
.\scripts\build.ps1 -SkipLinux

# Build somente Linux (deb + flatpak), sem recompilar o .exe
.\scripts\build.ps1 -SkipWindows
```

Build de validação sem GUI (CI/ambiente sem GCC para Fyne):

```powershell
go build -tags ci -o containerway_ci.exe ./cmd/containerway/
```

**CI no GitHub:** o workflow `.github/workflows/ci.yml` executa `go test` em pacotes internos sem depender da UI Fyne e compila `cmd/containerway-web` (sem CGO).

**Build da versão web (sem CGO):**

```powershell
.\scripts\web\build.bat
# ou: go build -o containerway-web.exe ./cmd/containerway-web/
```


</details>

<details>
<summary><strong>Execução</strong></summary>

```powershell
.\ContainerWay.exe
```

Fluxo recomendado:

1. Abra a aba `Conexões SSH/SFTP`.
2. Selecione uma conexão salva ou preencha uma nova.
3. Ajuste `Chave e segurança` (se necessário).
4. Use `Testar conexão` para validar acesso.
5. Clique em `Conectar`.
6. Na **tela inicial da sessão**, abra o **Gerenciador de arquivos** (ou outro módulo).
7. No painel direito do explorador, escolha o contexto:
   - pastas do servidor;
   - ou um contêiner em execução.

Ao iniciar no Windows Server sem OpenGL nativo, o aplicativo tenta autoajuste de runtime e pode relançar automaticamente uma vez.


</details>

<details>
<summary><strong>Compatibilidade gráfica no Windows Server</strong></summary>

Para reduzir intervenção manual em servidores sem OpenGL nativo, o `ContainerWay.exe`:

1. testa criação de contexto OpenGL na inicialização;
2. se falhar, baixa pacote de software rendering (Mesa llvmpipe);
3. extrai o pacote (com fallback em Go puro quando `tar` não suporta `.7z`);
4. prepara `opengl32.dll` em caminho local carregável;
5. relança o processo com ambiente ajustado.

Diagnóstico:

- Log de bootstrap: `%LOCALAPPDATA%\ContainerWay\startup.log`.
- Cache de runtime: `%LOCALAPPDATA%\ContainerWay\runtime-mesa`.

</details>

<details>
<summary><strong>Estrutura do projeto</strong></summary>

| Caminho | Responsabilidade |
|---------|------------------|
| `cmd/containerway` | Ponto de entrada do app desktop |
| `cmd/containerway-web` | Ponto de entrada do servidor web local |
| `internal/webapp` | API HTTP, sessões e UI estática embutida (browser) |
| `internal/connectcfg` | Perfis SSH (`connections.json`), partilhado desktop/web |
| `internal/accessauth` | Autenticação de acesso local (browser) |
| `internal/configdir` | Caminhos de configuração do utilizador |
| `internal/appui` | Interface Fyne (login, explorador, ações, atalhos) |
| `docs/` | Documentação Markdown ([índice](README.md)) |
| `scripts/build.ps1` | Build desktop e pacotes Linux |
| `scripts/web/` | Scripts run/build da versão web |
| `scripts/release/` | Publicação GitHub |
| `internal/session` | Conexão SSH, cliente SFTP e cliente Docker |
| `internal/hostfs` | Operações no host remoto via SFTP |
| `internal/containerfs` | Operações em arquivos de contêiner |
| `internal/localfs` | Operações no sistema de arquivos local |
| `internal/fsutil` | Utilitários de entrada/listagem e ordenação |
| `internal/tarxfer` | Transferências recursivas com tar |
| `internal/transfer` | Fila, progresso e workers de transferência |
| `packaging/linux` | Manifesto Flatpak (`.json`), `.desktop` e metainfo para o `.deb` |


</details>

<details>
<summary><strong>Documentação para desenvolvimento</strong></summary>

- Guia rápido de manutenção e estudo: [SUMARIO_DESENVOLVEDOR.md](SUMARIO_DESENVOLVEDOR.md).
- Versão web (browser): [WEB_UI.md](WEB_UI.md).
- Scripts: [SCRIPTS.md](SCRIPTS.md).
- Entrada do repositório: [README.md](../README.md).
- Convenção adotada no código Go:
  - comentários de função em pt-BR imediatamente acima da função;
  - texto curto e objetivo, focando intenção e efeito da rotina.
- Recomendação para novas mudanças:
  - ao criar uma função nova, já adicionar o comentário no mesmo commit;
  - ao refatorar, manter o comentário alinhado com o comportamento atual.


</details>

<details>
<summary><strong>Segurança</strong></summary>

- Em produção, prefira validação de host por `known_hosts` e evite `Ignorar chave de host`.
- Segredos podem ser:
  - persistidos na conexão local (quando marcado);
  - ou mantidos somente na sessão atual (não persistente em disco).
- Fluxo sudo é usado apenas quando necessário para acesso a caminhos protegidos.

</details>

---

## Créditos

- Coordenador e idealizador do projeto: [Bruno Fernandes](https://bruno-fernandes.online)
- Apoio no desenvolvimento: [Hugo Januario](https://hugojanuario.online)
