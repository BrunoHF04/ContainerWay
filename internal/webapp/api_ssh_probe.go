package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"containerway/internal/session"
)

// handleSSHTest testa ligação SSH/SFTP/Docker sem manter sessão.
func (s *Server) handleSSHTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	if _, _, ok := s.requireWebAuth(w, r); !ok {
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
	steps := []string{}
	sess, err := session.Connect(ctx, creds)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"steps": append(steps, "SSH: falhou"),
			"error": err.Error(),
		})
		return
	}
	defer sess.Close()
	steps = append(steps, "SSH: ok")
	if sess.SFTP != nil {
		steps = append(steps, "SFTP: ok")
	} else {
		steps = append(steps, "SFTP: indisponível")
	}
	if sess.Docker != nil {
		steps = append(steps, "Docker: ok")
	} else {
		steps = append(steps, "Docker: indisponível")
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "steps": steps})
}
