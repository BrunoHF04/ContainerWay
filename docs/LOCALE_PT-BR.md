# Português no projeto — somente pt-BR

Este repositório usa **português brasileiro (pt-BR)** em todo texto em português voltado a pessoas (interface, mensagens de erro, ajuda, documentação em PT, e-mails de teste, etc.).

**Não use português de Portugal (pt-PT)** em código, assets estáticos ou docs novas/alteradas — mesmo que soe “mais formal” ou seja ortografia antiga com `c` (`actual`, `ficheiro`).

## Regra para agentes e contribuidores

Antes de escrever ou revisar qualquer string em português:

1. Confirme que o idioma alvo é **pt-BR** (não pt-PT).
2. Consulte a tabela de equivalências abaixo se houver dúvida.
3. Não misture variantes na mesma frase (ex.: “usuário” + “ficheiro”).
4. Idiomas **EN** e **ES** têm seus próprios blocos em `locale_*.go` e `i18n.js` — não copie grafia pt-PT para o bloco `pt` / `langPTBR`.

## Onde o português aparece

| Área | Caminhos típicos |
|------|------------------|
| UI web | `internal/webapp/static/i18n.js`, `index.html`, `*.js` em `static/` |
| UI desktop (Fyne) | `internal/appui/locale_*.go` (`langPTBR`) |
| Manual embutido | `internal/appui/locale_manual.go` (`manualBodyPT`) |
| Erros API / backend | mensagens em `internal/webapp/`, `internal/diskutil/`, `internal/containerfs/`, etc. |
| Documentação | `docs/*.md`, `README.md` (quando em português) |

Comentários só para desenvolvedores podem ficar em inglês ou português; se forem em português, use pt-BR.

## Equivalências frequentes (pt-PT → pt-BR)

| Evitar (pt-PT) | Usar (pt-BR) |
|----------------|--------------|
| ficheiro / ficheiros | arquivo / arquivos |
| sistema de ficheiros | sistema de arquivos |
| utilizador | usuário |
| ligação / ligar-se / Ligue-se | conexão / conectar-se / Conecte-se |
| aceder | acessar |
| ecrã | tela |
| guardar (perfil/dados) | salvar |
| guardado | salvo |
| eliminar (UI) | excluir / apagar (conforme contexto existente) |
| registo | registro |
| definições | configurações |
| actual / actualizar / actualização | atual / atualizar / atualização |
| Activar / Desactivar | Ativar / Desativar |
| Seleccione | Selecione |
| Tem a certeza que… | Tem certeza de que… |
| exactamente | exatamente |
| encolher (FS/volume) | reduzir |
| libertar (espaço) | liberar |
| demasiado / demasiados | muito / muitos / pequeno demais |
| por defeito | por padrão |
| detetado / detetada | detectado / detectada |
| cópia de segurança | backup |
| planear | planejar |
| separador (aba UI) | aba |
| contacto | contato |

## Ortografia

- Preferir formas brasileiras: **conexão**, **ação**, **usuário**, **atualização**.
- Evitar grafia europeia com **c** onde o Brasil usa **t**: `atual` (não `actual`), `contato` (não `contacto`).
- Duplo **c** em verbos: `selecionar`, `conectar` (não `seleccionar`).

## Exceções aceitáveis

- **Aliases de busca/filtro** que reconhecem termos digitados pelo usuário (ex.: aceitar `ficheiro` no filtro `tipo:file` em `explorer-complete.js`), desde que os rótulos e mensagens oficiais da UI permaneçam em pt-BR.
- Blocos de idioma **espanhol** (`langES`, `es` em `i18n.js`) e **inglês** — não são pt-BR; não “corrigir” ES/EN para pt-BR.
- Nomes próprios, comandos Linux, `systemctl`, caminhos e identificadores técnicos — manter como estão.

## Checklist rápido de revisão

- [ ] Strings novas no mapa `pt` / `langPTBR`?
- [ ] HTML/JS estático sem “Ligue-se”, “Activar”, “ficheiro”?
- [ ] Mensagens de erro JSON/API em pt-BR?
- [ ] Documentação nova em português segue esta página?

## Referência de idioma na UI

- Web: `lang="pt-BR"` em `index.html`; chave de locale `pt` em `i18n.js` = **pt-BR** (rótulo: “Português (BR)”).
- Desktop: constante `langPTBR` em `internal/appui/locale_hub.go`.

Em caso de dúvida entre duas redações brasileiras válidas, alinhe ao tom já usado em `locale_ui.go` (bloco `langPTBR`) e em `i18n.js` (`DISKS_PT` / `STR.pt`).
