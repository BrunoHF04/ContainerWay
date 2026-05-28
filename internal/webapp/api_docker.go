package webapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	dcontainer "github.com/docker/docker/api/types/container"
)

// handleDockerContainers lista contêineres em execução no host remoto.
func (s *Server) handleDockerContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	if b.Sess.Docker == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Docker/Podman indisponível nesta ligação"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	list, err := b.Sess.Docker.ContainerList(ctx, dcontainer.ListOptions{All: false})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		out = append(out, map[string]any{
			"id":      c.ID[:12],
			"idFull":  c.ID,
			"name":    name,
			"image":   c.Image,
			"state":   c.State,
			"status":  c.Status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"containers": out})
}

// handleDockerRestart reinicia um contêiner pelo ID.
func (s *Server) handleDockerRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	if b.Sess.Docker == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Docker indisponível"})
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.ID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID obrigatório"})
		return
	}
	id := strings.TrimSpace(body.ID)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	timeout := 30
	if err := b.Sess.Docker.ContainerRestart(ctx, id, dcontainer.StopOptions{Timeout: &timeout}); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reiniciado"})
}

// handleDockerLogs devolve últimas linhas de log de um contêiner.
func (s *Server) handleDockerLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	if b.Sess.Docker == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Docker indisponível"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id obrigatório"})
		return
	}
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "200"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	rc, err := b.Sess.Docker.ContainerLogs(ctx, id, dcontainer.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Timestamps: true,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer rc.Close()
	bb, err := io.ReadAll(rc)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": string(bb)})
}

// handleDockerStats devolve estatísticas atuais de um contêiner.
func (s *Server) handleDockerStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	if b.Sess.Docker == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Docker indisponível"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id obrigatório"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	st, err := b.Sess.Docker.ContainerStats(ctx, id, false)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer st.Body.Close()
	bb, err := io.ReadAll(st.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": json.RawMessage(bb)})
}
