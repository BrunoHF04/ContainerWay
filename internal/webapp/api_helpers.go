package webapp

import (
	"errors"
	"net/http"
	"strings"
)

// errDockerUnavailable está definido em api_automations.go (partilhado no pacote webapp).

var (
	errInvalidContainer = errors.New("ID de contêiner obrigatório")
	errInvalidPath      = errors.New("caminho inválido")
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
