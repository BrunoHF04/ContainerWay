package webapp

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"containerway/internal/accessauth"
	"containerway/internal/configdir"
	"containerway/internal/fsutil"
	"containerway/internal/hostfs"
	"containerway/internal/localfs"
	"containerway/internal/session"
	"containerway/internal/webprefs"

	"tailscale.com/tsnet"
)

//go:embed static/*
var staticEmbed embed.FS

// Options parâmetros de arranque do servidor web.
type Options struct {
	Addr         string
	OpenBrowser  bool
	AutoShutdown bool

	// Configurações do Tailscale / Headscale
	TSAuthKey    string
	TSHostname   string
	TSSSHPort    int
	TSControlURL string

	// Configurações do Servidor Headscale (Admin)
	HSURL        string
	HSApiKey     string
}

// Run inicia o servidor HTTP do ContainerWay Web.
func Run(opts Options) error {
	addr := strings.TrimSpace(opts.Addr)
	if addr == "" {
		addr = "127.0.0.1:8765"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
		port = "8765"
	}
	baseURL := formatBaseURL(host, port)

	if opts.OpenBrowser && tryReuseRunningInstance(baseURL) {
		log.Printf("ContainerWay Web já em execução — browser reaberto (%s)", baseURL)
		return nil
	}

	srv := &Server{
		httpServer: &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
			Handler:           nil,
		},
		opts: opts,
	}
	srv.httpServer.Handler = srv.routes()

	// Inicialização da VPN Tailscale / Headscale
	tsAuthKey := strings.TrimSpace(opts.TSAuthKey)
	if tsAuthKey == "" {
		tsAuthKey = strings.TrimSpace(os.Getenv("CONTAINERWAY_TS_AUTHKEY"))
	}
	if tsAuthKey != "" {
		tsHostname := strings.TrimSpace(opts.TSHostname)
		if tsHostname == "" {
			tsHostname = strings.TrimSpace(os.Getenv("CONTAINERWAY_TS_HOSTNAME"))
		}
		if tsHostname == "" {
			tsHostname = "containerway-client"
		}
		tsSSHPort := opts.TSSSHPort
		if envSSHPort := os.Getenv("CONTAINERWAY_TS_SSH_PORT"); envSSHPort != "" {
			if p, err := strconv.Atoi(envSSHPort); err == nil {
				tsSSHPort = p
			}
		}
		tsControlURL := strings.TrimSpace(opts.TSControlURL)
		if tsControlURL == "" {
			tsControlURL = strings.TrimSpace(os.Getenv("CONTAINERWAY_TS_CONTROL_URL"))
		}

		cfgDir, err := configdir.Root()
		if err != nil {
			log.Printf("Tailscale: erro ao obter diretório de configurações: %v. Usando diretório temporário.", err)
		}
		tsStateDir := ""
		if cfgDir != "" {
			tsStateDir = filepath.Join(cfgDir, "tailscale")
			_ = os.MkdirAll(tsStateDir, 0o755)
		}

		tsSrv := &tsnet.Server{
			Hostname:   tsHostname,
			AuthKey:    tsAuthKey,
			ControlURL: tsControlURL,
			Dir:        tsStateDir,
		}
		defer tsSrv.Close()

		// 1. Escutar tráfego HTTP na VPN
		tsHTTPListener, err := tsSrv.Listen("tcp", ":8765")
		if err != nil {
			log.Printf("Tailscale: erro ao iniciar listener HTTP na VPN: %v", err)
		} else {
			log.Printf("Tailscale: Escutando na VPN na porta 8765. Nome do nó: %s", tsHostname)
			go func() {
				if err := srv.httpServer.Serve(tsHTTPListener); err != nil && err != http.ErrServerClosed {
					log.Printf("Tailscale: erro ao servir HTTP na VPN: %v", err)
				}
			}()
		}

		// 2. Proxy SSH para a porta física local
		if tsSSHPort > 0 {
			tsSSHListener, err := tsSrv.Listen("tcp", fmt.Sprintf(":%d", tsSSHPort))
			if err != nil {
				log.Printf("Tailscale: erro ao iniciar listener SSH na VPN na porta %d: %v", tsSSHPort, err)
			} else {
				log.Printf("Tailscale: SSH Proxy ativo na porta VPN %d -> localhost (SSH físico)", tsSSHPort)
				go handleSSHProxy(tsSSHListener)
			}
		}
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if opts.OpenBrowser && isAddrInUse(err) && tryReuseRunningInstance(baseURL) {
			log.Printf("porta ocupada — reutilizando instância (%s)", baseURL)
			return nil
		}
		return err
	}
	host, port, _ = net.SplitHostPort(ln.Addr().String())
	baseURL = formatBaseURL(host, port)
	log.Printf("ContainerWay Web em %s (SO=%s/%s)", baseURL, runtime.GOOS, runtime.GOARCH)

	if opts.OpenBrowser {
		go func() {
			time.Sleep(400 * time.Millisecond)
			if err := OpenBrowser(baseURL); err != nil {
				log.Printf("não foi possível abrir o browser: %v — abra manualmente: %s", err, baseURL)
			}
		}()
	} else {
		log.Printf("Abra no browser: %s", baseURL)
	}

	if opts.AutoShutdown {
		StartHeartbeatMonitor()
	}
	return srv.httpServer.Serve(ln)
}

// Server agrupa estado HTTP e sessões SSH.
type Server struct {
	httpServer *http.Server
	store      sessionStore
	opts       Options
}

// routes regista handlers da API e ficheiros estáticos.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Endpoints de Automação de Deploy
	mux.HandleFunc("/api/deploy/generate", s.handleDeployGenerate)
	mux.HandleFunc("/api/deploy/install.sh", s.handleDeployInstallScript)
	mux.HandleFunc("/api/deploy/download", s.handleDeployDownload)
	staticRoot, err := fs.Sub(staticEmbed, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(staticRoot))

	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/auth/login", s.handleAuthLogin)
	mux.HandleFunc("/api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("/api/auth/me", s.handleAuthMe)
	mux.HandleFunc("/api/connections", s.handleConnections)
	mux.HandleFunc("/api/ssh/connect", s.handleSSHConnect)
	mux.HandleFunc("/api/ssh/test", s.handleSSHTest)
	mux.HandleFunc("/api/ssh/disconnect", s.handleSSHDisconnect)
	mux.HandleFunc("/api/ssh/status", s.handleSSHStatus)
	mux.HandleFunc("/api/ssh/client-ip", s.handleSSHClientIP)
	mux.HandleFunc("/api/ssh/sudo", s.handleSSHSudo)
	mux.HandleFunc("/api/local/list", s.handleLocalList)
	mux.HandleFunc("/api/remote/list", s.handleRemoteList)
	mux.HandleFunc("/api/transfer/status", s.handleTransferStatus)
	mux.HandleFunc("/api/transfer/push", s.handleTransferPush)
	mux.HandleFunc("/api/transfer/pull", s.handleTransferPull)
	mux.HandleFunc("/api/transfer/batch", s.handleTransferBatch)
	mux.HandleFunc("/api/transfer/upload", s.handleTransferUpload)
	mux.HandleFunc("/api/transfer/cancel", s.handleTransferCancel)
	mux.HandleFunc("/api/explorer/operations", s.handleExplorerOperations)
	mux.HandleFunc("/api/explorer/sync-mirror", s.handleExplorerSyncMirror)
	mux.HandleFunc("/api/remote/external-status", s.handleRemoteExternalStatus)
	mux.HandleFunc("/api/local/mkdir", s.handleLocalMkdir)
	mux.HandleFunc("/api/local/rename", s.handleLocalRename)
	mux.HandleFunc("/api/local/delete", s.handleLocalDelete)
	mux.HandleFunc("/api/local/shortcuts", s.handleLocalShortcuts)
	mux.HandleFunc("/api/local/file", s.handleLocalFile)
	mux.HandleFunc("/api/local/open-external", s.handleLocalOpenExternal)
	mux.HandleFunc("/api/remote/file", s.handleRemoteFile)
	mux.HandleFunc("/api/remote/open-external", s.handleRemoteOpenExternal)
	mux.HandleFunc("/api/remote/sync-external", s.handleRemoteSyncExternal)
	mux.HandleFunc("/api/remote/mkdir", s.handleRemoteMkdir)
	mux.HandleFunc("/api/remote/rename", s.handleRemoteRename)
	mux.HandleFunc("/api/remote/delete", s.handleRemoteDelete)
	mux.HandleFunc("/api/explorer/compare", s.handleExplorerCompare)
	mux.HandleFunc("/api/explorer/favorites", s.handleExplorerFavorites)
	mux.HandleFunc("/api/explorer/clipboard", s.handleExplorerClipboard)
	mux.HandleFunc("/api/explorer/paste", s.handleExplorerPaste)
	mux.HandleFunc("/api/compose-opt/discover", s.handleComposeOptDiscover)
	mux.HandleFunc("/api/compose-opt/analyze", s.handleComposeOptAnalyze)
	mux.HandleFunc("/api/compose-opt/validate", s.handleComposeOptValidate)
	mux.HandleFunc("/api/docker/containers", s.handleDockerContainers)
	mux.HandleFunc("/api/docker/export", s.handleDockerExport)
	mux.HandleFunc("/api/docker/restart", s.handleDockerRestart)
	mux.HandleFunc("/api/docker/restart-batch", s.handleDockerRestartBatch)
	mux.HandleFunc("/api/docker/compose/restart-project", s.handleDockerComposeRestartProject)
	mux.HandleFunc("/api/docker/images", s.handleDockerImages)
	mux.HandleFunc("/api/docker/volumes", s.handleDockerVolumes)
	mux.HandleFunc("/api/docker/volumes/backup", s.handleDockerVolumeBackup)
	mux.HandleFunc("/api/docker/volumes/restore", s.handleDockerVolumeRestore)
	mux.HandleFunc("/api/backup/db/discover", s.handleDBDiscover)
	mux.HandleFunc("/api/backup/db/run", s.handleDBBackupRun)
	mux.HandleFunc("/api/backup/db/routines", s.handleDBBackupRoutines)
	mux.HandleFunc("/api/backup/db/restore", s.handleDBBackupRestore)
	mux.HandleFunc("/api/backup/db/test-db", s.handleDBTestConnection)
	mux.HandleFunc("/api/backup/db/test-smb", s.handleDBTestSMB)
	mux.HandleFunc("/api/backup/db/test-ssh", s.handleDBTestSSH)
	mux.HandleFunc("/api/ssh/setup-local", s.handleEnableLocalSSH)
	mux.HandleFunc("/api/docker/images/remove", s.handleDockerImageRemove)
	mux.HandleFunc("/api/docker/volumes/remove", s.handleDockerVolumeRemove)
	mux.HandleFunc("/api/docker/networks", s.handleDockerNetworks)
	mux.HandleFunc("/api/docker/system", s.handleDockerSystem)
	mux.HandleFunc("/api/docker/images/prune", s.handleDockerImagesPrune)
	mux.HandleFunc("/api/docker/volumes/prune", s.handleDockerVolumesPrune)
	mux.HandleFunc("/api/docker/buildcache/prune", s.handleDockerBuildCachePrune)
	mux.HandleFunc("/api/docker/containers/create", s.handleDockerContainerCreate)
	mux.HandleFunc("/api/docker/networks/create", s.handleDockerNetworkCreate)
	mux.HandleFunc("/api/docker/networks/remove", s.handleDockerNetworkRemove)
	mux.HandleFunc("/api/docker/volumes/create", s.handleDockerVolumeCreate)
	mux.HandleFunc("/api/docker/stop", s.handleDockerLifecycle("stop"))
	mux.HandleFunc("/api/docker/start", s.handleDockerLifecycle("start"))
	mux.HandleFunc("/api/docker/pause", s.handleDockerLifecycle("pause"))
	mux.HandleFunc("/api/docker/unpause", s.handleDockerLifecycle("unpause"))
	mux.HandleFunc("/api/docker/remove", s.handleDockerLifecycle("remove"))
	mux.HandleFunc("/api/docker/logs", s.handleDockerLogs)
	mux.HandleFunc("/api/docker/stats", s.handleDockerStats)
	mux.HandleFunc("/api/docker/inspect", s.handleDockerInspect)
	mux.HandleFunc("/api/docker/exec/ws", s.handleDockerExecWS)
	mux.HandleFunc("/api/desktop/overview", s.handleDesktopOverview)
	mux.HandleFunc("/api/desktop/network", s.handleDesktopNetwork)
	mux.HandleFunc("/api/desktop/network/apply", s.handleDesktopNetworkApply)
	mux.HandleFunc("/api/desktop/network/iface", s.handleDesktopNetworkIface)
	mux.HandleFunc("/api/desktop/services", s.handleDesktopServices)
	mux.HandleFunc("/api/desktop/service/control", s.handleDesktopServiceControl)
	mux.HandleFunc("/api/disks/summary", s.handleDisksSummary)
	mux.HandleFunc("/api/disks/probe", s.handleDisksProbe)
	mux.HandleFunc("/api/disks/extend-lv", s.handleDisksExtendLV)
	mux.HandleFunc("/api/disks/shrink-lv", s.handleDisksShrinkLV)
	mux.HandleFunc("/api/disks/fsck", s.handleDisksFsck)
	mux.HandleFunc("/api/disks/snapshot-create", s.handleDisksSnapshotCreate)
	mux.HandleFunc("/api/disks/snapshot-remove", s.handleDisksSnapshotRemove)
	mux.HandleFunc("/api/disks/lv-create", s.handleDisksLVCreate)
	mux.HandleFunc("/api/disks/resize-fs", s.handleDisksResizeFS)
	mux.HandleFunc("/api/disks/lv-rename", s.handleDisksLVRename)
	mux.HandleFunc("/api/disks/vg-change", s.handleDisksVgchange)
	mux.HandleFunc("/api/disks/fstrim", s.handleDisksFstrim)
	mux.HandleFunc("/api/disks/smart", s.handleDisksSmart)
	mux.HandleFunc("/api/disks/usage", s.handleDisksUsage)
	mux.HandleFunc("/api/services", s.handleServicesList)
	mux.HandleFunc("/api/services/control", s.handleServicesControl)
	mux.HandleFunc("/api/services/logs", s.handleServicesLogs)
	mux.HandleFunc("/api/automations/rules", s.handleAutomationsRules)
	mux.HandleFunc("/api/automations/history", s.handleAutomationsHistory)
	mux.HandleFunc("/api/automations/engine", s.handleAutomationsEngine)
	mux.HandleFunc("/api/admin/users", s.handleAdminUsers)
	mux.HandleFunc("/api/admin/mail", s.handleAdminMail)
	mux.HandleFunc("/api/admin/web-settings", s.handleAdminWebSettings)
	mux.HandleFunc("/api/settings/diagnostics", s.handleSettingsDiagnostics)
	mux.HandleFunc("/api/ssh/terminal/ws", s.handleTerminalWS)
	mux.HandleFunc("/api/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("/api/heartbeat/unload", s.handleHeartbeatUnload)

	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || !strings.Contains(r.URL.Path, ".") {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	}))
	return mux
}

// handleHealth responde com estado básico do serviço.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"product": "ContainerWay Web",
		"version": Version,
	})
}

// handleAuthLogin autentica acesso local e emite cookie de sessão.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	username := strings.TrimSpace(body.Username)
	cfg, _ := webprefs.Load()
	if cfg.Policies.MaxLoginAttempts > 0 {
		if webprefs.LoginFailuresFor(username) >= cfg.Policies.MaxLoginAttempts {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "muitas tentativas falhadas — aguarde ou contacte o administrador",
			})
			return
		}
	}
	user, ok := accessauth.Authenticate(body.Username, body.Password)
	if !ok {
		if cfg.Policies.MaxLoginAttempts > 0 {
			n, _ := webprefs.RecordLoginFailure(username)
			if n >= cfg.Policies.MaxLoginAttempts && cfg.Notifications.LoginFailure {
				// e-mail opcional: ignorar erro de envio
				_ = notifyLoginFailure(cfg, username)
			}
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "usuário ou senha inválidos"})
		return
	}
	_ = webprefs.ResetLoginFailures(username)
	tok, err := s.store.newWebToken(user.Username, user.DisplayName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "falha ao criar sessão"})
		return
	}
	setSessionCookie(w, tok)
	writeJSON(w, http.StatusOK, map[string]any{
		"username":           user.Username,
		"displayName":        user.DisplayName,
		"isAdmin":            isAdminUser(user.Username),
		"permissions":        permissionsPayload(user.Username),
		"mustChangePassword": webprefs.MustChangePassword(user.Username),
	})
}

// handleAuthLogout invalida a sessão web atual.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	if tok, ok := sessionTokenFromRequest(r); ok {
		s.store.deleteWebToken(tok)
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}


// handleSSHConnect abre sessão SSH/SFTP para o utilizador web autenticado.
func (s *Server) handleSSHConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	tok, _, ok := s.requireWebAuth(w, r)
	if !ok {
		return
	}
	var body sshConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	creds, err := body.credentials()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	sess, err := session.Connect(ctx, creds)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.store.setSSH(tok, sess, creds.Host, creds.User, parallelJobsFromRequest(body))
	writeJSON(w, http.StatusOK, map[string]any{
		"connected": true,
		"host":      creds.Host,
		"user":      creds.User,
	})
}

// handleSSHDisconnect fecha a sessão SSH ativa.
func (s *Server) handleSSHDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	tok, _, ok := s.requireWebAuth(w, r)
	if !ok {
		return
	}
	s.store.clearSSH(tok)
	writeJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// handleSSHStatus indica se há sessão SSH ativa.
func (s *Server) handleSSHStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	tok, ws, ok := s.requireWebAuth(w, r)
	if !ok {
		return
	}
	b := s.store.getSSH(tok)
	resp := map[string]any{
		"connected":   b != nil,
		"isAdmin":     isAdminUser(ws.Username),
		"permissions": permissionsPayload(ws.Username),
	}
	if b != nil {
		resp["host"] = b.Host
		resp["user"] = b.User
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSSHClientIP retorna o IP da máquina que estabeleceu a conexão SSH.
func (s *Server) handleSSHClientIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stdout, _, err := b.runSSHCommand(ctx, "echo $SSH_CONNECTION", "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	parts := strings.Fields(stdout)
	ip := ""
	if len(parts) > 0 {
		ip = parts[0]
	}
	writeJSON(w, http.StatusOK, map[string]string{"ip": ip})
}

// handleLocalList lista diretório no computador onde o servidor corre.
func (s *Server) handleLocalList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	if _, _, ok := s.requireWebAuth(w, r); !ok {
		return
	}
	dir := strings.TrimSpace(r.URL.Query().Get("path"))
	if dir == "" {
		if runtime.GOOS == "windows" {
			dir = localfs.WindowsDrivesVirtualPath
		} else {
			home, err := osUserHome()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			dir = home
		}
	}
	entries, err := localfs.List(dir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    dir,
		"entries": serializeEntries(entries),
	})
}

// handleRemoteList lista diretório no host remoto via SFTP.
func (s *Server) handleRemoteList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	tok, _, ok := s.requireWebAuth(w, r)
	if !ok {
		return
	}
	b := s.store.getSSH(tok)
	if b == nil {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "ligue-se ao servidor SSH primeiro"})
		return
	}
	dir := strings.TrimSpace(r.URL.Query().Get("path"))
	if dir == "" {
		dir = "/"
	}
	containerID := strings.TrimSpace(r.URL.Query().Get("containerId"))
	var entries []fsutil.DirEntry
	var err error
	if containerID != "" {
		cfs, cfsErr := containerFS(b, containerID)
		if cfsErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": cfsErr.Error()})
			return
		}
		entries, err = cfs.List(r.Context(), dir)
	} else {
		b.sudoMu.Lock()
		sudoOn := b.sudoEnabled
		b.sudoMu.Unlock()
		if sudoOn {
			entries, err = b.listHostWithSudo(r.Context(), dir)
		} else {
			hfs := &hostfs.FS{Client: b.Sess.SFTP}
			entries, err = hfs.List(r.Context(), dir)
		}
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	resp := map[string]any{
		"path":        dir,
		"containerId": containerID,
		"entries":     serializeEntries(entries),
	}
	b.sudoMu.Lock()
	if b.sudoEnabled {
		resp["sudo"] = map[string]any{"enabled": true, "user": b.sudoUser}
	}
	b.sudoMu.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

// requireWebAuth valida cookie de sessão web.
func (s *Server) requireWebAuth(w http.ResponseWriter, r *http.Request) (string, webSession, bool) {
	tok, ok := sessionTokenFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "não autenticado"})
		return "", webSession{}, false
	}
	ws, ok := s.store.getWebToken(tok)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sessão expirada"})
		return "", webSession{}, false
	}
	if idle, mins := sessionIdleExceeded(ws); idle {
		s.store.deleteWebToken(tok)
		clearSessionCookie(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "sessão encerrada por inatividade (" + strconv.Itoa(mins) + " min)",
		})
		return "", webSession{}, false
	}
	s.store.touchWebToken(tok)
	if need := permissionForRequest(r.Method, r.URL.Path); need != "" {
		if !accessauth.PermissionsForUser(ws.Username).HasAction(need) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "sem permissão para esta ação"})
			return "", webSession{}, false
		}
	}
	return tok, ws, true
}

// serializeEntries converte entradas para JSON da API.
func serializeEntries(entries []fsutil.DirEntry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"name":    e.Name,
			"path":    e.Path,
			"isDir":   e.IsDir,
			"size":    e.Size,
			"modTime": e.ModTime.UTC().Format(time.RFC3339),
		})
	}
	return out
}

// writeJSON escreve resposta JSON com código HTTP.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// handleSSHProxy aceita conexões de rede na VPN Tailscale e as redireciona para a porta SSH local (padrão 22).
func handleSSHProxy(ln net.Listener) {
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Tailscale SSH Proxy: erro ao aceitar conexão: %v", err)
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			localPort := "22"
			if lp := os.Getenv("CONTAINERWAY_LOCAL_SSH_PORT"); lp != "" {
				localPort = lp
			}
			localConn, err := net.DialTimeout("tcp", "127.0.0.1:"+localPort, 10*time.Second)
			if err != nil {
				log.Printf("Tailscale SSH Proxy: erro ao conectar ao SSH local na porta %s: %v", localPort, err)
				return
			}
			defer localConn.Close()

			errChan := make(chan error, 2)
			go func() {
				_, err := io.Copy(localConn, c)
				errChan <- err
			}()
			go func() {
				_, err := io.Copy(c, localConn)
				errChan <- err
			}()
			<-errChan
		}(conn)
	}
}
