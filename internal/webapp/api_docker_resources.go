package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/image"
)

func (s *Server) handleDockerImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	rows, err := listDockerImages(ctx, b.Sess.Docker)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	var total int64
	for _, row := range rows {
		total += row.Size
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"images":    rows,
		"count":     len(rows),
		"totalSize": total,
		"totalHuman": formatBytesHuman(total),
		"updatedAt": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleDockerVolumes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	rows, err := listDockerVolumes(ctx, b.Sess.Docker)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"volumes":   rows,
		"count":     len(rows),
		"updatedAt": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleDockerImageRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	var body struct {
		ID    string `json:"id"`
		Force bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.ID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id obrigatório"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	_, err := b.Sess.Docker.ImageRemove(ctx, strings.TrimSpace(body.ID), image.RemoveOptions{
		Force:         body.Force,
		PruneChildren: true,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removida"})
}

func (s *Server) handleDockerVolumeRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	var body struct {
		Name  string `json:"name"`
		Force bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name obrigatório"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := b.Sess.Docker.VolumeRemove(ctx, strings.TrimSpace(body.Name), body.Force); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removido"})
}
