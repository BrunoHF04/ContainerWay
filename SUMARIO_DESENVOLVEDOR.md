# Sumario do Desenvolvedor

Este arquivo serve como guia rapido para manutencao do projeto `ContainerWay`.

## Visao Geral

- Aplicacao desktop em Go com interface grafica (`Fyne`) para gerenciamento e transferencia de arquivos entre host e conteineres.
- Entrada principal da aplicacao: `cmd/containerway/main.go`.
- Modulo com maior concentracao de regras de UI: `internal/appui/appui.go`.

## Mapa de Pastas

- `cmd/containerway/`: executavel principal da aplicacao.
- `cmd/iconforge/`: utilitario para gerar/converter icones da aplicacao.
- `internal/appui/`: telas, componentes visuais, tema e acoes de UI.
- `internal/containerfs/`: operacoes de sistema de arquivos no lado do conteiner.
- `internal/hostfs/`: operacoes de sistema de arquivos no computador local.
- `internal/localfs/`: utilitarios de leitura/escrita local usados por outros modulos.
- `internal/fsutil/`: tipos e helpers compartilhados de representacao de entradas de arquivos.
- `internal/session/`: sessao remota, credenciais e chaves SSH.
- `internal/tarxfer/`: transferencia de arquivos via tar/stream.
- `internal/transfer/`: orquestracao de transferencias entre origem e destino.
- `internal/mailnotify/`: envio de notificacoes por e-mail (SMTP).
- `assets/`: recursos estaticos (icones, imagens e afins).

## Mapa de Arquivos Go (o que cada um faz)

- `cmd/containerway/main.go`: inicializa e sobe a aplicacao principal.
- `cmd/iconforge/main.go`: comandos para geracao/manipulacao de icones.
- `internal/appui/appui.go`: fluxo principal da UI, janelas, estados e eventos.
- `internal/appui/appicon.go`: carga e definicao de icone da aplicacao.
- `internal/appui/connections.go`: tela/acoes relacionadas a conexoes.
- `internal/appui/dirrow.go`: componente visual para linhas de diretorio/arquivo.
- `internal/appui/dockercontainers.go`: listagem e manipulacao de conteineres Docker na UI.
- `internal/appui/theme.go`: definicao e aplicacao de tema visual.
- `internal/appui/window_maximize_darwin.go`: comportamento de maximizar janela no macOS.
- `internal/appui/window_maximize_windows.go`: comportamento de maximizar janela no Windows.
- `internal/appui/window_maximize_stub.go`: fallback para plataformas sem implementacao especifica.
- `internal/containerfs/containerfs.go`: leitura/listagem e operacoes de arquivos no conteiner.
- `internal/hostfs/hostfs.go`: leitura/listagem e operacoes de arquivos no host.
- `internal/localfs/localfs.go`: operacoes locais auxiliares de arquivo.
- `internal/fsutil/entry.go`: estrutura padrao de entrada de arquivo/diretorio.
- `internal/session/session.go`: criacao e gerenciamento de sessao remota.
- `internal/session/hostkey.go`: validacao/manuseio de host key.
- `internal/session/keys.go`: leitura/carregamento de chaves SSH.
- `internal/tarxfer/tarxfer.go`: empacotamento/desempacotamento e stream de transferencia.
- `internal/transfer/transfer.go`: coordenacao da logica de transferencia.
- `internal/mailnotify/mailnotify.go`: configuracao e envio de e-mails de notificacao.

## Prioridade de Estudo (sugestao)

1. `README.md`
2. `cmd/containerway/main.go`
3. `internal/appui/appui.go`
4. `internal/session/session.go`
5. `internal/transfer/transfer.go`
6. `internal/containerfs/containerfs.go` e `internal/hostfs/hostfs.go`

## Convencao de Comentarios de Funcao

- Todas as funcoes devem ter comentario imediatamente acima.
- Comentarios devem ser curtos, objetivos e em pt-BR.
- Sempre descrever intencao/efeito da funcao (o "por que" e "o que"), evitando obviedades.

