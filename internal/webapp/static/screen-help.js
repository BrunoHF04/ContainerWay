/** Ajuda contextual por ecrã (? na barra superior). */
(function () {
  const SCREENS = {
    connect: {
      title: "Ligação SSH/SFTP",
      html: `
        <p class="muted">Configure o acesso ao servidor remoto antes de abrir os módulos.</p>
        <h4>Funcionalidades</h4>
        <ul>
          <li><strong>Perfil</strong> — carregar, guardar ou apagar ligações em <code>connections.json</code>.</li>
          <li><strong>Host / Utilizador / Senha</strong> — credenciais SSH; opcionalmente chave no perfil (desktop).</li>
          <li><strong>Guardar senha no perfil</strong> — persiste localmente no seu PC.</li>
          <li>Após <strong>Conectar</strong>, abre o menu da sessão (hub).</li>
        </ul>
        <h4>Atalhos gerais</h4>
        <ul class="shortcut-list">
          <li><kbd>Ctrl</kbd>+<kbd>K</kbd> Paleta de comandos</li>
          <li><kbd>?</kbd> Ajuda desta tela</li>
          <li><kbd>Esc</kbd> Fechar diálogos</li>
        </ul>
      `,
    },
    hub: {
      title: "Menu da sessão",
      html: `
        <p class="muted">Escolha o módulo depois de ligado ao SSH. Use a pesquisa para filtrar cartões.</p>
        <h4>Módulos</h4>
        <ul>
          <li><strong>Gerenciador de arquivos</strong> — painel duplo local/remoto, SFTP e Docker.</li>
          <li><strong>Contêineres Docker</strong> — lista, logs, estatísticas e reinício.</li>
          <li><strong>Discos e armazenamento</strong> — <code>lsblk</code> e <code>df</code> no host.</li>
          <li><strong>Terminal SSH</strong> — consola interactiva no browser.</li>
          <li><strong>Central de automações</strong> — regras e motor por host.</li>
          <li><strong>Configurações</strong> — utilizadores e SMTP (admin).</li>
        </ul>
        <h4>Barra superior</h4>
        <ul>
          <li><strong>SSH: online/offline</strong> — estado da ligação.</li>
          <li><strong>ms</strong> — latência de listagem remota (referência).</li>
          <li><strong>Desconectar</strong> — fecha SSH; <strong>Sair</strong> — termina sessão web.</li>
        </ul>
        <h4>Atalhos</h4>
        <ul class="shortcut-list">
          <li><kbd>Ctrl</kbd>+<kbd>K</kbd> Ir a um módulo</li>
          <li><kbd>?</kbd> Ajuda</li>
        </ul>
      `,
    },
    explorer: {
      title: "Gerenciador de arquivos",
      html: `
        <p class="muted">Painel <strong>Local</strong> (seu PC) e <strong>Remoto</strong> (host SFTP ou ficheiros dentro de um contêiner Docker). O caminho aparece só nos <strong>breadcrumbs</strong> (clique para subir níveis).</p>

        <h4>Barra de ferramentas</h4>
        <ul>
          <li><strong>Ações</strong> — nova pasta, renomear, apagar, copiar/colar, abrir/editar, comparar pastas.</li>
          <li><strong>Enviar / Receber</strong> — transferir o(s) item(ns) seleccionado(s).</li>
          <li><strong>Lote→ / ←Lote</strong> — enviar ou receber todos os itens visíveis (após filtro).</li>
          <li><strong>Sudo</strong> — elevar privilégios no host SFTP (pastas protegidas). Não aplica em modo Docker.</li>
          <li><strong>↻</strong> — actualizar ambos os painéis.</li>
          <li><strong>▤</strong> — mostrar/ocultar o painel de transferências em baixo.</li>
        </ul>

        <h4>Painel Remoto</h4>
        <ul>
          <li><strong>SFTP</strong> — ficheiros no servidor Linux ligado por SSH.</li>
          <li><strong>Docker</strong> — selector de contêiner + filtro de pesquisa; ao mudar contêiner ou modo, a pasta volta a <code>/</code>.</li>
          <li>Ícones: favoritos (estrela), subir, actualizar; menu de atalhos guardados.</li>
        </ul>

        <h4>Lista de ficheiros</h4>
        <ul>
          <li>Clique nas colunas <strong>Nome / Tamanho / Data</strong> para ordenar.</li>
          <li><strong>Filtrar ficheiros</strong> — pesquisa por nome na pasta actual.</li>
          <li><strong>Duplo clique</strong> — abrir pasta ou editor de ficheiro.</li>
          <li><strong>Botão direito</strong> — menu de contexto (enviar, receber, editar, etc.).</li>
          <li><strong>Pré-visualização</strong> — painel à direita para texto/imagem (redimensionável).</li>
        </ul>

        <h4>Seleção e arrastar</h4>
        <ul>
          <li><kbd>Ctrl</kbd>+clique — adicionar/remover da selecção.</li>
          <li><kbd>Shift</kbd>+clique — selecção em intervalo.</li>
          <li>Arrastar ficheiros entre painéis — enviar (local→remoto) ou receber (remoto→local).</li>
          <li>Largar ficheiros do Windows no painel <strong>Remoto</strong> — upload para a pasta actual.</li>
        </ul>

        <h4>Comparar pastas</h4>
        <ul>
          <li>Compara as pastas abertas nos dois painéis; permite <strong>Enviar →</strong> ou <strong>← Receber</strong> por diferença.</li>
        </ul>

        <h4>Transferências</h4>
        <ul>
          <li>Barra inferior <strong>Transferências</strong> — progresso e histórico; arraste a borda para redimensionar.</li>
          <li>Separador vertical entre Local/Remoto — arraste para ajustar largura dos painéis.</li>
        </ul>

        <h4>Atalhos de teclado (explorador)</h4>
        <ul class="shortcut-list">
          <li><kbd>F5</kbd> Actualizar painel activo (Local ou Remoto)</li>
          <li><kbd>Tab</kbd> Alternar painel activo</li>
          <li><kbd>Backspace</kbd> Subir uma pasta</li>
          <li><kbd>Enter</kbd> Abrir pasta ou editar ficheiro</li>
          <li><kbd>F2</kbd> Renomear</li>
          <li><kbd>Del</kbd> Apagar (com confirmação)</li>
          <li><kbd>F6</kbd> Enviar (painel Local) ou Receber (painel Remoto)</li>
          <li><kbd>Shift</kbd>+<kbd>F6</kbd> Lote no painel activo</li>
          <li><kbd>Ctrl</kbd>+<kbd>K</kbd> Paleta de comandos</li>
          <li><kbd>?</kbd> Esta ajuda</li>
          <li><kbd>Esc</kbd> Fechar diálogos</li>
        </ul>
      `,
    },
    docker: {
      title: "Docker",
      html: `
        <p class="muted">Gestão Docker no host remoto: <strong>Contêineres</strong>, <strong>Imagens</strong> e <strong>Volumes</strong>.</p>
        <h4>Abas</h4>
        <ul>
          <li><strong>Contêineres</strong> — métricas, ciclo de vida, logs, consola, Compose.</li>
          <li><strong>Imagens</strong> — listagem, filtro, dangling, remover (com confirmação).</li>
          <li><strong>Volumes</strong> — listagem, copiar nome, remover.</li>
        </ul>
        <h4>Contêineres</h4>
        <ul>
          <li><strong>Lista</strong> / <strong>Grelha</strong> — alterne o modo de visualização (a preferência fica guardada).</li>
          <li><strong>Filtros rápidos</strong> — Todos, em execução, parados, alerta, críticos, Compose, Swarm.</li>
          <li><strong>Resumo</strong> — chips sob o título com totais e médias de CPU/RAM.</li>
          <li><strong>Agrupar Compose</strong> — secções por projeto; clique no cabeçalho para colapsar.</li>
          <li><strong>★ Fixar</strong> — mantém contêineres no topo da lista.</li>
          <li><strong>Cores dos cartões</strong> — neutro no normal; <strong>amarelo</strong> ≥75% CPU/RAM; <strong>vermelho</strong> ≥90%.</li>
          <li><strong>Clique num cartão</strong> — vista dividida com detalhes, sparklines e inspect.</li>
          <li><strong>Ferramentas</strong> — copiar ID/nome, logs, stats, explorador, consola, ciclo de vida.</li>
          <li><strong>Portas</strong> — mapeamentos publicados no cartão quando existirem.</li>
          <li>Seleção em lote: iniciar, reiniciar, parar, remover; checkbox <strong>Selecionar todos visíveis</strong>.</li>
          <li>URL <code>#docker=ID</code> — abre directamente o detalhe do contêiner.</li>
          <li><strong>Intervalo</strong> — auto-atualizar a cada 5–60 s (preferência guardada).</li>
          <li><strong>Logs</strong> — pesquisa, destaque de erros/stderr, «Desde reinício», quebra de linha, live.</li>
          <li><strong>Stats</strong> — modal com gráficos em tempo real (actualiza a cada 2 s).</li>
          <li><strong>📁</strong> no cartão — abre o explorador de ficheiros do contêiner.</li>
          <li>Lote com erros — diálogo lista IDs que falharam.</li>
          <li><strong>Agrupar Compose</strong> — botão «Reiniciar projeto» por grupo (<code>compose up --force-recreate</code>).</li>
          <li><strong>Expandir / Colapsar</strong> — com agrupamento Compose activo.</li>
          <li><strong>Regra automática</strong> — cria regra na Central de automações (reinício se parar).</li>
          <li>Toast quando novos contêineres ficam críticos durante auto-atualizar.</li>
          <li>Reinício de um contêiner tenta <em>compose recreate</em> do serviço quando há labels Compose.</li>
        </ul>
        <h4>Atalhos neste ecrã</h4>
        <ul class="shortcut-list">
          <li><kbd>R</kbd> Atualizar lista</li>
          <li><kbd>/</kbd> Focar pesquisa</li>
          <li><kbd>Esc</kbd> Voltar à lista (fecha detalhes)</li>
        </ul>
        <h4>Atalhos gerais</h4>
        <ul class="shortcut-list">
          <li><kbd>Ctrl</kbd>+<kbd>K</kbd> Mudar de módulo</li>
          <li><kbd>?</kbd> Ajuda</li>
        </ul>
      `,
    },
    disks: {
      title: "Discos e armazenamento",
      html: `
        <p class="muted">Visão de blocos e uso de disco no servidor (<code>lsblk</code>, <code>df</code>).</p>
        <ul>
          <li><strong>Atualizar</strong> — nova sondagem remota.</li>
          <li>Tabela com dispositivo, tipo, montagem e tamanho.</li>
          <li>Bloco inferior com saída <code>df</code> para referência.</li>
        </ul>
      `,
    },
    terminal: {
      title: "Terminal SSH",
      html: `
        <p class="muted">Consola interactiva no host remoto (xterm.js + WebSocket).</p>
        <ul>
          <li><strong>Reconectar</strong> — nova sessão de terminal se a ligação cair.</li>
          <li>Clique na área preta para focar; digite comandos como num terminal normal.</li>
          <li><kbd>Ctrl</kbd>+<kbd>C</kbd> — interromper comando (quando focado no terminal).</li>
        </ul>
      `,
    },
    automations: {
      title: "Central de automações",
      html: `
        <p class="muted">Regras por host com motor em segundo plano (partilhado com o desktop).</p>
        <ul>
          <li><strong>Iniciar / Parar motor</strong> — activa verificação periódica de contêineres.</li>
          <li><strong>Regras</strong> — criar, editar, activar; gatilho «contêiner parado» → reinício.</li>
          <li><strong>Histórico</strong> — eventos recentes; <strong>Limpar</strong> apaga o histórico.</li>
          <li><strong>Atualizar</strong> — recarrega regras e histórico do disco.</li>
        </ul>
      `,
    },
    settings: {
      title: "Configurações",
      html: `
        <p class="muted">Conta local e, para <strong>admin</strong>, gestão de utilizadores e e-mail.</p>
        <ul>
          <li>Informação da versão ContainerWay Web.</li>
          <li>Atalhos para módulos de administração (utilizadores, SMTP).</li>
        </ul>
      `,
    },
    default: {
      title: "ContainerWay Web",
      html: `
        <p class="muted">Ajuda geral da aplicação.</p>
        <ul class="shortcut-list">
          <li><kbd>Ctrl</kbd>+<kbd>K</kbd> Paleta de comandos — saltar para módulos e acções.</li>
          <li><kbd>?</kbd> Ajuda da tela actual</li>
          <li><kbd>Esc</kbd> Fechar diálogos</li>
        </ul>
        <p>Abra um módulo no menu da sessão para ver ajuda específica.</p>
      `,
    },
  };

  function currentScreenId() {
    return state.screen || "connect";
  }

  window.openScreenHelp = function openScreenHelp() {
    const dlg = $("#screen-help-dialog");
    const titleEl = $("#screen-help-title");
    const bodyEl = $("#screen-help-body");
    if (!dlg || !bodyEl) return;
    const id = currentScreenId();
    const page = SCREENS[id] || SCREENS.default;
    if (titleEl) titleEl.textContent = page.title;
    bodyEl.innerHTML = page.html;
    dlg.showModal();
  };

  $("#screen-help-close")?.addEventListener("click", () => $("#screen-help-dialog")?.close());
})();
