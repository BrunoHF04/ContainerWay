# Segurança do ContainerWay

## Modelo de ameaça (resumo)

- O **login local** do aplicativo (utilizador `admin` e outros) é uma **trava no posto de trabalho**: não substitui a autenticação SSH nem controla quem acede ao servidor.
- **Credenciais SSH**, chaves, **SMTP** e listas de e-mail ficam nas **preferências locais** do utilizador no sistema operativo (ver documentação principal do projeto).
- A opção **«Ignorar chave de host SSH»** é **insegura** (vulnerável a ataques man-in-the-middle) e só deve ser usada em redes de confiança ou testes.

## Política opcional (empresa)

Administradores podem impedir ligações com chave de host ignorada:

1. Variável de ambiente: `CONTAINERWAY_FORBID_INSECURE_HOSTKEY=1` (ou `true` / `yes`).
2. Ou ficheiro `%APPDATA%\ContainerWay\policy.json` (Windows) / pasta de configuração do utilizador + `ContainerWay/policy.json` (Linux/macOS), por exemplo:

```json
{"forbidInsecureHostKey": true}
```

Com isto ativo, a caixa «Ignorar chave de host» fica desativada e o valor enviado à sessão SSH força validação de host.

## Docker / Podman remoto

O cliente API usa o **socket Unix no servidor** (por omissão `/var/run/docker.sock`). Para **Podman**, configure na ligação o caminho adequado (ex.: socket em `/run/user/…/podman/podman.sock`). A API deve ser compatível com o cliente Docker usado pelo projeto.

## Reportar problemas

Para vulnerabilidades, contacte os mantenedores do repositório com descrição reprodutível e impacto; evite anexar credenciais ou dados pessoais.
