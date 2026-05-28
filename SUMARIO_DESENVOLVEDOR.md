# Sumario do Desenvolvedor

Este arquivo serve como guia rapido para manutencao do projeto `ContainerWay`.

## Visao Geral

- Aplicacao desktop em Go com interface grafica (`Fyne`) para gerenciamento e transferencia de arquivos entre host e conteineres.
- **Versao web (MVP):** branch `dev_browser` — `cmd/containerway-web`, `internal/webapp`, documentacao em `docs/WEB_UI.md`.
- Entrada principal desktop: `cmd/containerway/main.go`.
- Entrada principal web: `cmd/containerway-web/main.go`.
- Modulo com maior concentracao de regras de UI desktop: `internal/appui/appui.go`.

## Mapa de Pastas

- `cmd/containerway/`: executavel principal da aplicacao desktop.
- `cmd/containerway-web/`: servidor HTTP local da versao browser.
- `cmd/iconforge/`: utilitario para gerar/converter icones da aplicacao.
- `docs/WEB_UI.md`: guia da versao web (API, scripts, roadmap).
- `scripts/web/`: build e execucao da versao web (`.bat` / `.sh`).
- `internal/appui/`: telas, componentes visuais, tema e acoes de UI.
- `internal/containerfs/`: operacoes de sistema de arquivos no lado do conteiner.
- `internal/hostfs/`: operacoes de sistema de ficheiros no **host remoto Linux** via SFTP.
- `internal/localfs/`: utilitarios de leitura/escrita local usados por outros modulos.
- `internal/fsutil/`: tipos e helpers compartilhados de representacao de entradas de arquivos.
- `internal/session/`: sessao remota, credenciais, chaves SSH e cliente Docker/Podman via dial Unix remoto.
- `internal/policy/`: leitura de politica local opcional (ex.: proibir ignorar chave de host) via `policy.json` ou variavel de ambiente.
- `internal/tarxfer/`: transferencia de arquivos via tar/stream.
- `internal/transfer/`: orquestracao de transferencias entre origem e destino.
- `internal/mailnotify/`: envio de notificacoes por e-mail (SMTP).
- `internal/webapp/`: servidor HTTP, sessoes e ficheiros estaticos da UI web.
- `internal/connectcfg/`: perfis `connections.json` (desktop + web).
- `internal/accessauth/`: login de acesso local para a UI web.
- `internal/configdir/`: diretorio de configuracao ContainerWay e preferencias Fyne.
- `assets/`: recursos estaticos (icones, imagens e afins).
- `packaging/linux/`: arquivos de empacotamento Linux (manifesto Flatpak, entrada `.desktop`, metainfo AppStream usados no `.deb` e no bundle).
- `SECURITY.md`: notas de seguranca e politica local (referencia para auditores e administradores).
- `.github/workflows/`: CI (testes Go em pacotes internos sem compilar a UI Fyne no runner Ubuntu).

## Build e releases

- **`build.ps1`** (na raiz):
  - **Windows:** `go build` local com `CGO_ENABLED=1` e `-H=windowsgui` → `ContainerWay.exe`.
  - **Linux:** `docker run` com imagem `golang:1.26-bookworm`, volume do repo em `/src`, script gerado em `dist/build-linux.sh`. Dentro do container: dependências (`build-essential`, libs X11/GL, `flatpak`, `flatpak-builder`, `appstream-compose`, etc.), compilação `GOOS=linux GOARCH=amd64`, montagem do `.deb` e `flatpak-builder` + `flatpak build-bundle`. O container usa **`--privileged`** para o `bwrap` do Flatpak; usa **`--disable-rofiles-fuse`** porque FUSE costuma nao estar disponivel no Docker.
- **Saidas:** `dist/linux/containerway` (binario Linux), `dist/deb/containerway_<versao>_amd64.deb`, `dist/flatpak/containerway-<versao>.flatpak`. Pastas `dist/` e `.flatpak-builder/` estao no `.gitignore`.
- **Versao dos pacotes:** parametro `-Version` ou saida de `git describe`; versao Debian e sanitizada e, se nao comecar com digito, recebe prefixo `0~` (requisito do campo `Version` do `.deb`).

## Mapa de Arquivos Go (o que cada um faz)

- `cmd/containerway/main.go`: bootstrap do executavel, logs de startup e fallback automatico de OpenGL no Windows Server.
- `cmd/iconforge/main.go`: comandos para geracao/manipulacao de icones.
- `internal/appui/appui.go`: fluxo principal da UI, janelas, estados e eventos.
- `internal/appui/appicon.go`: carga e definicao de icone da aplicacao.
- `internal/appui/connections.go`: perfis de ligacao persistidos (`connections.json`), incluindo campo opcional `dockerSocket` (socket Unix remoto Docker/Podman).
- `internal/appui/dirrow.go`: componente visual para linhas de diretorio/arquivo.
- `internal/appui/foldercompare.go`: relatorio de comparacao entre listagens dos dois paineis do explorador.
- `internal/appui/securityui.go`: dicas de primeiro acesso, dialogo de politica de seguranca, exportacao de auditoria para CSV, abertura do dialogo «Comparar pastas».
- `internal/appui/automations.go`: central de automacoes (regras JSON por host, motor, webhook opcional, historico).
- `internal/appui/dockercontainers.go`: listagem de conteineres, metricas e reinicio/recriacao de servicos Docker Compose na UI.
- `internal/appui/diskstorage.go`: modulo **Discos e armazenamento** (hub): sondagem remota (`lsblk`, `df`, LVM), abas assistente / host / detalhe tecnico, lista com uso e correcao de layout (colunas fixas a esquerda + `Border` para area «Tamanho / uso» expandir).
- `internal/appui/theme.go`: definicao e aplicacao de tema visual.
- `internal/appui/window_maximize_darwin.go`: comportamento de maximizar janela no macOS.
- `internal/appui/window_maximize_windows.go`: comportamento de maximizar janela no Windows.
- `internal/appui/window_maximize_stub.go`: fallback para plataformas sem implementacao especifica.
- `internal/containerfs/containerfs.go`: leitura/listagem e operacoes de arquivos no conteiner.
- `internal/hostfs/hostfs.go`: leitura/listagem, `Stat`, criacao/remocao no host remoto via SFTP.
- `internal/localfs/localfs.go`: operacoes locais auxiliares de arquivo.
- `internal/fsutil/entry.go`: estrutura padrao de entrada de arquivo/diretorio.
- `internal/session/session.go`: SSH, SFTP e cliente API no socket Unix remoto (caminho configuravel em `Credentials.DockerUnixSocket`; padrao `/var/run/docker.sock`).
- `internal/session/hostkey.go`: validacao/manuseio de host key.
- `internal/session/keys.go`: leitura/carregamento de chaves SSH.
- `internal/tarxfer/tarxfer.go`: empacotamento/desempacotamento e stream de transferencia.
- `internal/transfer/transfer.go`: coordenacao da logica de transferencia (fila, workers paralelos, `Queued`/`Running`).
- `internal/mailnotify/mailnotify.go`: configuracao e envio de e-mails de notificacao.

## Prioridade de Estudo (sugestao)

1. `README.md`, `docs/WEB_UI.md` e `SECURITY.md`
2. `build.ps1` e `packaging/linux/` (releases Linux)
3. `cmd/containerway/main.go` (desktop) ou `cmd/containerway-web` (web)
4. `internal/appui/appui.go`
5. `internal/policy/policy.go`
6. `internal/session/session.go`
7. `internal/transfer/transfer.go`
8. `internal/containerfs/containerfs.go` e `internal/hostfs/hostfs.go`

## Convencao de Comentarios de Funcao

- Todas as funcoes devem ter comentario imediatamente acima.
- Comentarios devem ser curtos, objetivos e em pt-BR.
- Sempre descrever intencao/efeito da funcao (o "por que" e "o que"), evitando obviedades.

## Notas recentes de manutencao

- **Politica local:** `internal/policy` + ficheiro `%APPDATA%\\ContainerWay\\policy.json` (ou equivalente) com `forbidInsecureHostKey`, ou env `CONTAINERWAY_FORBID_INSECURE_HOSTKEY`; UI em login e botao **Políticas** na central de automacoes (`securityui.go`).
- **Docker/Podman remoto:** campo de socket na ligacao (`connections.json` / `savedConnection.dockerSocket`) → `session.Credentials.DockerUnixSocket`.
- **Explorador:** botao **Comparar** (`foldercompare.go`); favoritos do painel direito por host/contexto em preferencias (`remoteFavoritesPreferenceKey` em `appui.go`); fila de transferencias mostra `Queued`/`Running` (`internal/transfer/transfer.go`); upload SFTP ficheiro unico pode omitir se destino ja tem mesmo tamanho (`hostfs.Stat` + `appui.go`).
- **Historico:** exportacao CSV da trilha de auditoria (`writeAuditLogCSVBytes` em `securityui.go`).
- **Automacoes:** campo `webhookURL` nas regras; POST apos restart bem-sucedido (`automations.go`).
- **CI:** `.github/workflows/ci.yml` — `go test` em `internal/policy`, `internal/transfer`, `internal/hostfs`, `internal/session` (evita CGO/Fyne no runner).
- **Discos e armazenamento (`diskstorage.go`):** cartao no hub (`appui.go`); evitar `HBox` como unico layout para a ultima coluna de tabelas largas — no Fyne o `HBox` so atribui a cada filho a sua `MinSize().Width` e o «extra» so vai para `layout.Spacer`; para esticar conteudo use `container.NewBorder` (fixos em `left`, conteudo variavel no `center`) ou espacers explícitos.
- Reinicio de contêineres na UI:
  - para contêineres Compose, a acao de "Reiniciar" tenta recriar servico com `docker compose up -d --force-recreate` (com tentativa de pull).
  - para contêineres fora de Compose, o fallback continua sendo `docker restart`.
- Bootstrap grafico no Windows Server:
  - quando OpenGL nativo falha, o executavel baixa e extrai runtime de software (`opengl32sw-64.7z`), prepara `opengl32.dll` local, relanca o processo e registra em `startup.log`.
  - a DLL local criada ao lado do executavel e marcada como oculta para nao poluir a area de trabalho.

