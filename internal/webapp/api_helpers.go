package webapp

import (
	"net/http"
	"strings"
)

// requireSSH exige sessão web e SSH ativa.
func (s *Server) requireSSH(w http.ResponseWriter, r *http.Request) (string, *sshBundle, bool) {
	tok, _, ok := s.requireWebAuth(w, r)
	if !ok {
		return "", nil, false
	}
	b := s.store.getSSH(tok)
	if b == nil || b.Sess == nil {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "ligue-se ao servidor SSH primeiro"})
		return "", nil, false
	}
	return tok, b, true
}

// isAdminUser indica se o utilizador web é administrador.
func isAdminUser(username string) bool {
	return strings.EqualFold(strings.TrimSpace(username), "admin")
}
