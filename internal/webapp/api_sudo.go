package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// handleSSHSudo gere activação/desactivação do modo sudo na sessão SSH.
func (s *Server) handleSSHSudo(w http.ResponseWriter, r *http.Request) {
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, b.sudoStatus())
	case http.MethodPost:
		var body struct {
			Action   string `json:"action"`
			User     string `json:"user"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}
		switch strings.TrimSpace(body.Action) {
		case "disable":
			b.clearSudo()
			writeJSON(w, http.StatusOK, map[string]string{"status": "sudo desactivado"})
		case "enable":
			user := strings.TrimSpace(body.User)
			if user == "" {
				user = "root"
			}
			pass := body.Password
			if pass == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "senha sudo obrigatória"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
			defer cancel()
			resolved, err := b.testSudoAccess(ctx, user, pass)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			b.setSudo(resolved, pass)
			writeJSON(w, http.StatusOK, map[string]any{
				"status":  "sudo activo",
				"enabled": true,
				"user":    resolved,
			})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action inválida (enable|disable)"})
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}
