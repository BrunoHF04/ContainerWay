package webapp

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"containerway/internal/connectcfg"
	"containerway/internal/session"
)

const (
	sessionCookieName = "cw_session"
	sessionTTL        = 12 * time.Hour
)

type webSession struct {
	Username    string
	DisplayName string
	ExpiresAt   time.Time
}

type sessionStore struct {
	mu  sync.RWMutex
	web map[string]webSession
	ssh map[string]*sshBundle
}

// newWebToken cria token de sessão web.
func (st *sessionStore) newWebToken(username, displayName string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.web == nil {
		st.web = map[string]webSession{}
	}
	st.web[tok] = webSession{
		Username:    username,
		DisplayName: displayName,
		ExpiresAt:   time.Now().Add(sessionTTL),
	}
	return tok, nil
}

// getWebToken devolve sessão web se válida.
func (st *sessionStore) getWebToken(tok string) (webSession, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	ws, ok := st.web[tok]
	if !ok || time.Now().After(ws.ExpiresAt) {
		return webSession{}, false
	}
	return ws, true
}

// deleteWebToken remove sessão web e SSH associada.
func (st *sessionStore) deleteWebToken(tok string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.web, tok)
	if b, ok := st.ssh[tok]; ok {
		b.Sess.Close()
		delete(st.ssh, tok)
	}
}

// setSSH associa sessão SSH à sessão web.
func (st *sessionStore) setSSH(webTok string, s *session.Session, host, user string, parallelJobs int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.ssh == nil {
		st.ssh = map[string]*sshBundle{}
	}
	if old, ok := st.ssh[webTok]; ok && old != nil && old.Sess != s {
		old.Sess.Close()
	}
	if parallelJobs < 1 {
		parallelJobs = 2
	}
	if parallelJobs > 8 {
		parallelJobs = 8
	}
	st.ssh[webTok] = &sshBundle{Sess: s, Host: host, User: user, parallelJobs: parallelJobs}
}

// getSSH devolve bundle SSH da sessão web.
func (st *sessionStore) getSSH(webTok string) *sshBundle {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.ssh[webTok]
}

// clearSSH fecha e remove sessão SSH.
func (st *sessionStore) clearSSH(webTok string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if b, ok := st.ssh[webTok]; ok {
		b.stopAutomationEngine()
		b.Sess.Close()
		delete(st.ssh, webTok)
	}
}

// sessionTokenFromRequest lê token do cookie de sessão.
func sessionTokenFromRequest(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(c.Value) == "" {
		return "", false
	}
	return c.Value, true
}

// setSessionCookie define cookie HttpOnly de sessão.
func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// clearSessionCookie remove cookie de sessão.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

type sshConnectRequest struct {
	ProfileName     string `json:"profileName"`
	Host            string `json:"host"`
	User            string `json:"user"`
	Password        string `json:"password"`
	KeyPath         string `json:"keyPath"`
	KeyPass         string `json:"keyPass"`
	KnownHosts      string `json:"knownHosts"`
	InsecureHostKey bool   `json:"insecureHostKey"`
	DockerSocket    string `json:"dockerSocket"`
	ParallelJobs    string `json:"parallelJobs"`
}

// parallelJobsFromRequest devolve paralelismo de transferências (1–8).
func parallelJobsFromRequest(req sshConnectRequest) int {
	if n := parseParallelJobs(req.ParallelJobs); n > 0 {
		return n
	}
	if name := strings.TrimSpace(req.ProfileName); name != "" {
		list, err := connectcfg.LoadAll()
		if err == nil {
			if prof, ok := connectcfg.FindByName(list, name); ok {
				if n := parseParallelJobs(prof.ParallelJobs); n > 0 {
					return n
				}
			}
		}
	}
	return 2
}

func parseParallelJobs(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0
	}
	if n > 8 {
		return 8
	}
	return n
}

// credentials monta credenciais SSH a partir do pedido ou perfil guardado.
func (req sshConnectRequest) credentials() (session.Credentials, error) {
	host := strings.TrimSpace(req.Host)
	user := strings.TrimSpace(req.User)
	password := req.Password
	keyPath := strings.TrimSpace(req.KeyPath)
	keyPass := req.KeyPass
	knownHosts := strings.TrimSpace(req.KnownHosts)
	insecure := req.InsecureHostKey
	dockerSock := strings.TrimSpace(req.DockerSocket)

	if name := strings.TrimSpace(req.ProfileName); name != "" {
		list, err := connectcfg.LoadAll()
		if err != nil {
			return session.Credentials{}, err
		}
		prof, ok := connectcfg.FindByName(list, name)
		if !ok {
			return session.Credentials{}, errProfileNotFound(name)
		}
		if host == "" {
			host = prof.Host
		}
		if user == "" {
			user = prof.User
		}
		if password == "" {
			password = prof.Password
		}
		if keyPath == "" {
			keyPath = prof.KeyPath
		}
		if keyPass == "" {
			keyPass = prof.KeyPass
		}
		if knownHosts == "" {
			knownHosts = prof.KnownHosts
		}
		if !req.InsecureHostKey {
			insecure = prof.InsecureHostKey
		}
		if dockerSock == "" {
			dockerSock = prof.DockerSocket
		}
	}

	var kh []string
	if knownHosts != "" {
		for _, p := range strings.Split(knownHosts, "|") {
			p = strings.TrimSpace(p)
			if p != "" {
				kh = append(kh, p)
			}
		}
	}

	if host == "" || user == "" {
		return session.Credentials{}, errMissingHostUser()
	}

	return session.Credentials{
		Host:              host,
		User:              user,
		Password:          password,
		KeyPath:           keyPath,
		KeyPass:           keyPass,
		KnownHostsFiles:   kh,
		InsecureHostKey:   insecure,
		DockerUnixSocket:  dockerSock,
	}, nil
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func errProfileNotFound(name string) error {
	return simpleError("perfil não encontrado: " + name)
}

func errMissingHostUser() error {
	return simpleError("host e usuário são obrigatórios")
}
