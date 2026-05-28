package webapp

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"containerway/internal/accessauth"
	"containerway/internal/connectcfg"
	"containerway/internal/fsutil"
	"containerway/internal/hostfs"
	"containerway/internal/localfs"
	"containerway/internal/session"
)

//go:embed static/*
var staticEmbed embed.FS

// Options parâmetros de arranque do servidor web.
type Options struct {
	Addr        string
	OpenBrowser bool
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
	}
	srv.httpServer.Handler = srv.routes()

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

	return srv.httpServer.Serve(ln)
}

// Server agrupa estado HTTP e sessões SSH.
type Server struct {
	httpServer *http.Server
	store      sessionStore
}

// routes regista handlers da API e ficheiros estáticos.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
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
	mux.HandleFunc("/api/ssh/disconnect", s.handleSSHDisconnect)
	mux.HandleFunc("/api/ssh/status", s.handleSSHStatus)
	mux.HandleFunc("/api/local/list", s.handleLocalList)
	mux.HandleFunc("/api/remote/list", s.handleRemoteList)
	mux.HandleFunc("/api/transfer/status", s.handleTransferStatus)
	mux.HandleFunc("/api/transfer/push", s.handleTransferPush)
	mux.HandleFunc("/api/transfer/pull", s.handleTransferPull)
	mux.HandleFunc("/api/docker/containers", s.handleDockerContainers)
	mux.HandleFunc("/api/docker/restart", s.handleDockerRestart)
	mux.HandleFunc("/api/disks/summary", s.handleDisksSummary)
	mux.HandleFunc("/api/automations/rules", s.handleAutomationsRules)
	mux.HandleFunc("/api/automations/history", s.handleAutomationsHistory)
	mux.HandleFunc("/api/ssh/terminal/ws", s.handleTerminalWS)

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
	user, ok := accessauth.Authenticate(body.Username, body.Password)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "utilizador ou senha inválidos"})
		return
	}
	tok, err := s.store.newWebToken(user.Username, user.DisplayName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "falha ao criar sessão"})
		return
	}
	setSessionCookie(w, tok)
	writeJSON(w, http.StatusOK, map[string]any{
		"username":    user.Username,
		"displayName": user.DisplayName,
		"isAdmin":     isAdminUser(user.Username),
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

// handleAuthMe devolve o utilizador autenticado na sessão web.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, ws, ok := s.requireWebAuth(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username":    ws.Username,
		"displayName": ws.DisplayName,
		"isAdmin":     isAdminUser(ws.Username),
	})
}

// handleConnections lista perfis SSH guardados (sem segredos).
func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	if _, _, ok := s.requireWebAuth(w, r); !ok {
		return
	}
	list, err := connectcfg.LoadAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, map[string]any{
			"name":            c.Name,
			"host":            c.Host,
			"user":            c.User,
			"hasPassword":     strings.TrimSpace(c.Password) != "",
			"hasKey":          strings.TrimSpace(c.KeyPath) != "",
			"insecureHostKey": c.InsecureHostKey,
			"dockerSocket":    c.DockerSocket,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
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
	s.store.setSSH(tok, sess, creds.Host, creds.User)
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
		"connected": b != nil,
		"isAdmin":   isAdminUser(ws.Username),
	}
	if b != nil {
		resp["host"] = b.Host
		resp["user"] = b.User
	}
	writeJSON(w, http.StatusOK, resp)
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
	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	entries, err := hfs.List(r.Context(), dir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    dir,
		"entries": serializeEntries(entries),
	})
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
