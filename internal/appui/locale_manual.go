package appui

// Corpo completo do manual (texto simples) por idioma.
var manualStrings = map[string]map[string]string{
	langPTBR: {"manual_body": manualBodyPT},
	langEN:   {"manual_body": manualBodyEN},
	langES:   {"manual_body": manualBodyES},
}

const manualBodyPT = `ContainerWay — Manual do usuário

0) Menu principal (Início da sessão)
- Após conectar ao servidor, esta é a porta de entrada: atalhos para cada módulo (arquivos, Docker, discos, terminal, automações e, para administradores, configurações).
- Campo de pesquisa filtra os cartões por palavras-chave (ex.: lvm, e-mail, ssh). As palavras-chave internas cobrem português, inglês e espanhol.
- Lista "Idioma": Português (BR), English ou Español — altera os textos da interface e é guardada nas preferências.
- No canto superior: botão "Manual do sistema" (este texto) e o seletor de tema por ícones (computador = padrão do sistema, paleta colorida = claro, tons de cinza = escuro). Cada clique aplica o tema de imediato; o ícone ativo fica em destaque.

1) Acesso local e conexão SSH
- Primeira tela: usuário e senha de acesso ao aplicativo (contas definidas pelo administrador).
- Segunda tela: dados SSH/SFTP — host, usuário remoto, senha ou chave, known_hosts opcional, jobs em paralelo, socket Docker/Podman remoto.
- "Testar conexão" valida SSH, SFTP e API de contêineres antes de "Conectar".
- É possível salvar e carregar perfis de conexão. Tema e outras opções ficam no formulário de conexão.

2) Gerenciador de arquivos (barra superior)
- "Voltar": retorna ao menu principal sem encerrar a sessão SSH.
- "Enviar" / "Receber": transferência entre o painel local (esquerda) e o remoto ou contêiner (direita), conforme o painel ativo.
- "Histórico": fila de operações, repetição de falhas, exportações.
- "Comparar": relatório de diferenças entre pastas local e remota (nome, tipo, tamanho, data).
- Estado "Sudo" e "Desativar sudo": quando o modo superusuário remoto está ativo (necessário para algumas operações em disco/LVM no servidor).
- "Sair": encerra a sessão e volta à tela de conexão.

3) Painéis e navegação
- Esquerda: computador local. Direita: servidor ou contêiner escolhido no seletor de contexto.
- Por painel: voltar pasta, subir, raiz do contexto, atualizar; atalhos favoritos (+/−); pesquisa e filtro (Tudo / Pastas / Arquivos).
- Duplo clique: abrir pasta; arquivo local abre no app padrão do sistema.

4) Transferências
- Envio local → servidor/contêiner; recepção remota → local.
- "Enviar visíveis" / "Receber visíveis": lote sobre linhas filtradas na lista.
- Upload único por SFTP: se já existir destino com o mesmo tamanho, o envio pode ser omitido (registro no log de auditoria).
- Durante jobs, a barra de status mostra fila e progresso.

5) Pesquisa e filtros nas listas
- Texto livre no nome; ext:log; tipo:pasta ou tipo:arquivo (ou type:folder / type:file em inglês).

6) Favoritos
- "+" salva a pasta atual nos atalhos do painel; "−" remove. Favoritos locais são globais; no servidor dependem do host e do contexto (host vs contêiner).

7) Comparar pastas e política local
- "Comparar" na barra do explorador gera o relatório entre os dois painéis.
- Política opcional: arquivo policy.json nas preferências do app ou variável CONTAINERWAY_FORBID_INSECURE_HOSTKEY=1 para impedir "Ignorar chave de host". O estado se resume em "Políticas" na central de automações.

8) Edição remota
- Abrir arquivo remoto para edição: o app sincroniza de volta quando detecta salvamento local.

9) Contêineres Docker (menu principal → cartão)
- Lista de contêineres em execução no host conectado; atualização, logs, reinício unitário ou em lote (com confirmação).

10) Discos e armazenamento (menu principal → cartão)
- Visão a partir de lsblk; abas para assistente LVM, resumo e detalhe técnico; filtro opcional de dispositivos loop (ex.: Snap).
- Operações sensíveis podem exigir sudo no servidor (ativar na própria janela quando disponível).

11) Terminal SSH (menu principal → cartão)
- Console remoto sobre a sessão já autenticada; modo ANSI ou compatibilidade textual.
- Atalhos úteis: gerenciador de tarefas, uso de disco, lista de comandos favoritos (quando existir).

12) Central de automações (menu principal → cartão)
- Regras com gatilho e ação no host; motor liga/desliga; runbooks e políticas de segurança.

13) Configurações (somente administrador — cartão no menu principal)
- Usuários de acesso ao app ContainerWay (local).
- Alertas por e-mail (SMTP, destinatários, teste de envio).

14) Histórico, sessão e auditoria
- "Histórico" no explorador: abas de sessão e log geral; filtrar, exportar, CSV de auditoria, abrir arquivos de log, repetir falhas.

15) Janela e atalhos de teclado (foco no gerenciador de arquivos)
- Enter: abrir. Backspace: subir nível. Tab: alternar painel.
- F3 ou Ctrl+F: focar pesquisa. F5: atualizar. F6: enviar/receber. Ctrl+Shift+F6: lote visível.
- F2: renomear. Del: excluir. Ctrl+Shift+N: nova pasta.

16) Dicas rápidas
- Permissões no servidor: avaliar sudo. Falhas em lote: usar Histórico. Sem resultados: revisar filtros e texto de pesquisa.`

const manualBodyEN = `ContainerWay — User manual

0) Session home (main menu)
- After you connect to the server, this is the entry point: shortcuts to each module (files, Docker, disks, terminal, automations, and for administrators, settings).
- The search field filters cards by keywords (e.g. lvm, e-mail, ssh). Built-in keywords cover Portuguese, English, and Spanish.
- The "Language" list: Português (BR), English, or Español — changes interface text and is stored in preferences.
- Top bar: "System manual" button (this text) and theme icons (monitor = follow OS, colour palette = light, grey tones = dark). Each click applies immediately; the active icon is highlighted.

1) Local access and SSH connection
- First screen: username and password for the app (accounts defined by the administrator).
- Second screen: SSH/SFTP data — host, remote user, password or key, optional known_hosts, parallel jobs, remote Docker/Podman socket.
- "Test connection" validates SSH, SFTP, and the container API before "Connect".
- You can save and load connection profiles. Theme and other options are on the connection form.

2) File manager (top bar)
- "Back": returns to the main menu without closing the SSH session.
- "Send" / "Receive": transfer between the local pane (left) and remote or container (right), depending on the active pane.
- "History": operation queue, retry failures, exports.
- "Compare": diff report between local and remote folders (name, type, size, date).
- "Sudo" state and "Disable sudo": when remote superuser mode is on (needed for some disk/LVM operations on the server).
- "Exit": ends the session and returns to the connection screen.

3) Panes and navigation
- Left: local computer. Right: server or container chosen in the context selector.
- Per pane: back, up, context root, refresh; favourite shortcuts (+/−); search and filter (All / Folders / Files).
- Double-click: open folder; local file opens with the system default app.

4) Transfers
- Send local → server/container; receive remote → local.
- "Send visible" / "Receive visible": batch on filtered rows.
- Single SFTP upload: if the destination already exists with the same size, the send may be skipped (audit log entry).
- During jobs, the status bar shows queue and progress.

5) Search and filters in lists
- Free text in the name; ext:log; type:folder or type:file (tipo:pasta / tipo:arquivo in Portuguese).

6) Favourites
- "+" saves the current folder to the pane shortcuts; "−" removes. Local favourites are global; on the server they depend on host and context (host vs container).

7) Compare folders and local policy
- "Compare" in the explorer bar builds the report between the two panes.
- Optional policy: policy.json in app preferences, or CONTAINERWAY_FORBID_INSECURE_HOSTKEY=1 to block "Ignore host key". Summary is under "Policies" in the automation centre.

8) Remote editing
- Open a remote file for editing: the app syncs back when it detects a local save.

9) Docker containers (main menu → card)
- List of containers running on the connected host; refresh, logs, single or batch restart (with confirmation).

10) Disks and storage (main menu → card)
- View from lsblk; tabs for LVM assistant, summary, and technical detail; optional loop-device filter (e.g. Snap).
- Sensitive operations may require sudo on the server (enable in that window when available).

11) SSH terminal (main menu → card)
- Remote console over the authenticated session; ANSI or plain-text compatibility.
- Useful shortcuts: task manager, disk usage, favourite commands list (when present).

12) Automation centre (main menu → card)
- Rules with trigger and action on the host; engine on/off; runbooks and security policies.

13) Settings (administrator only — card on main menu)
- ContainerWay app access users (local).
- E-mail alerts (SMTP, recipients, test send).

14) History, session, and audit
- "History" in the explorer: session and general log tabs; filter, export, audit CSV, open log files, retry failures.

15) Window and keyboard shortcuts (file manager focused)
- Enter: open. Backspace: go up. Tab: switch pane.
- F3 or Ctrl+F: focus search. F5: refresh. F6: send/receive. Ctrl+Shift+F6: visible batch.
- F2: rename. Del: delete. Ctrl+Shift+N: new folder.

16) Quick tips
- Server permissions: consider sudo. Batch failures: use History. No results: check filters and search text.`

const manualBodyES = `ContainerWay — Manual del usuario

0) Inicio de sesión (menú principal)
- Tras conectar al servidor, es el punto de entrada: accesos a cada módulo (archivos, Docker, discos, terminal, automatizaciones y, para administradores, configuración).
- El campo de búsqueda filtra las tarjetas por palabras clave (p. ej. lvm, e-mail, ssh). Las palabras clave internas cubren portugués, inglés y español.
- Lista "Idioma": Português (BR), English o Español — cambia los textos de la interfaz y se guarda en preferencias.
- Barra superior: botón "Manual del sistema" (este texto) y selector de tema por iconos (monitor = sistema, paleta = claro, tonos grises = oscuro). Cada clic aplica al momento; el icono activo se resalta.

1) Acceso local y conexión SSH
- Primera pantalla: usuario y contraseña de la aplicación (cuentas definidas por el administrador).
- Segunda pantalla: datos SSH/SFTP — host, usuario remoto, contraseña o clave, known_hosts opcional, trabajos en paralelo, socket Docker/Podman remoto.
- "Probar conexión" valida SSH, SFTP y la API de contenedores antes de "Conectar".
- Puede guardar y cargar perfiles de conexión. El tema y otras opciones están en el formulario de conexión.

2) Administrador de archivos (barra superior)
- "Volver": vuelve al menú principal sin cerrar la sesión SSH.
- "Enviar" / "Recibir": transferencia entre el panel local (izquierda) y el remoto o contenedor (derecha), según el panel activo.
- "Historial": cola de operaciones, reintentar fallos, exportaciones.
- "Comparar": informe de diferencias entre carpetas local y remota (nombre, tipo, tamaño, fecha).
- Estado "Sudo" y "Desactivar sudo": cuando el modo superusuario remoto está activo (necesario para algunas operaciones de disco/LVM en el servidor).
- "Salir": cierra la sesión y vuelve a la pantalla de conexión.

3) Paneles y navegación
- Izquierda: equipo local. Derecha: servidor o contenedor elegido en el selector de contexto.
- Por panel: atrás, subir, raíz del contexto, actualizar; atajos favoritos (+/−); búsqueda y filtro (Todo / Carpetas / Archivos).
- Doble clic: abrir carpeta; archivo local se abre con la app predeterminada del sistema.

4) Transferencias
- Envío local → servidor/contenedor; recepción remota → local.
- "Enviar visibles" / "Recibir visibles": lote sobre filas filtradas.
- Subida SFTP única: si el destino ya existe con el mismo tamaño, el envío puede omitirse (registro en auditoría).
- Durante trabajos, la barra de estado muestra cola y progreso.

5) Búsqueda y filtros en listas
- Texto libre en el nombre; ext:log; tipo:carpeta o tipo:archivo (o type:folder / type:file en inglés).

6) Favoritos
- "+" guarda la carpeta actual en atajos del panel; "−" quita. Los favoritos locales son globales; en el servidor dependen del host y del contexto (host vs contenedor).

7) Comparar carpetas y política local
- "Comparar" en la barra del explorador genera el informe entre los dos paneles.
- Política opcional: archivo policy.json en preferencias de la app o variable CONTAINERWAY_FORBID_INSECURE_HOSTKEY=1 para impedir "Ignorar clave de host". El resumen está en "Políticas" en la central de automatizaciones.

8) Edición remota
- Abrir archivo remoto para edición: la app sincroniza al detectar guardado local.

9) Contenedores Docker (menú principal → tarjeta)
- Lista de contenedores en ejecución en el host conectado; actualizar, logs, reinicio unitario o en lote (con confirmación).

10) Discos y almacenamiento (menú principal → tarjeta)
- Vista desde lsblk; pestañas para asistente LVM, resumen y detalle técnico; filtro opcional de dispositivos loop (p. ej. Snap).
- Operaciones sensibles pueden exigir sudo en el servidor (activar en esa ventana cuando esté disponible).

11) Terminal SSH (menú principal → tarjeta)
- Consola remota sobre la sesión ya autenticada; modo ANSI o compatibilidad de texto plano.
- Atajos útiles: administrador de tareas, uso de disco, lista de comandos favoritos (si existe).

12) Central de automatizaciones (menú principal → tarjeta)
- Reglas con disparador y acción en el host; motor encendido/apagado; runbooks y políticas de seguridad.

13) Configuración (solo administrador — tarjeta en el menú principal)
- Usuarios de acceso a la app ContainerWay (local).
- Alertas por e-mail (SMTP, destinatarios, prueba de envío).

14) Historial, sesión y auditoría
- "Historial" en el explorador: pestañas de sesión y log general; filtrar, exportar, CSV de auditoría, abrir archivos de log, repetir fallos.

15) Ventana y atajos de teclado (foco en el administrador de archivos)
- Enter: abrir. Retroceso: subir nivel. Tab: alternar panel.
- F3 o Ctrl+F: foco en búsqueda. F5: actualizar. F6: enviar/recibir. Ctrl+Shift+F6: lote visible.
- F2: renombrar. Supr: eliminar. Ctrl+Shift+N: nueva carpeta.

16) Consejos rápidos
- Permisos en el servidor: valorar sudo. Fallos en lote: usar Historial. Sin resultados: revisar filtros y texto de búsqueda.`
