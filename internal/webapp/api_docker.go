package webapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"containerway/internal/dockerutil"

	dcontainer "github.com/docker/docker/api/types/container"
)

func dockerClientFromBundle(b *sshBundle, w http.ResponseWriter) bool {
	if b.Sess.Docker == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Docker/Podman indisponível nesta ligação"})
		return false
	}
	return true
}

// handleDockerContainers lista contêineres no host remoto.
// Query: all=1 (incluir parados), metrics=1 (métricas por linha).
func (s *Server) handleDockerContainers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	metrics := r.URL.Query().Get("metrics") == "1" || r.URL.Query().Get("metrics") == "true"
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	rows, err := listDockerContainers(ctx, b.Sess.Docker, all, metrics)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"containers": rows,
		"count":      len(rows),
		"updatedAt":  time.Now().Format(time.RFC3339),
	})
}

// handleDockerExport exporta snapshot CSV dos contêineres.
func (s *Server) handleDockerExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	all := r.URL.Query().Get("all") == "1"
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	rows, err := listDockerContainers(ctx, b.Sess.Docker, all, true)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="docker-containers.csv"`)
	_ = writeContainersCSV(w, rows)
}

// handleDockerRestart reinicia um contêiner (Compose recreate quando possível).
func (s *Server) handleDockerRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.ID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID obrigatório"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	if err := smartRestartContainer(ctx, b, body.ID); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reiniciado"})
}

// handleDockerRestartBatch reinicia vários contêineres em sequência.
func (s *Server) handleDockerRestartBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ids obrigatório"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	var errs []string
	okN := 0
	for _, id := range body.IDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		if err := smartRestartContainer(ctx, b, id); err != nil {
			errs = append(errs, id+": "+err.Error())
		} else {
			okN++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"restarted": okN,
		"errors":    errs,
	})
}

// handleDockerComposeRestartProject recria todos os serviços do projeto Compose de um contêiner.
func (s *Server) handleDockerComposeRestartProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.ID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id obrigatório"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	if err := restartComposeProject(ctx, b, body.ID); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "projeto reiniciado"})
}

func (s *Server) handleDockerLifecycle(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID obrigatório"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		fullID, err := resolveContainerID(ctx, b.Sess.Docker, body.ID)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		timeout := 30
		var runErr error
		switch action {
		case "stop":
			runErr = b.Sess.Docker.ContainerStop(ctx, fullID, dcontainer.StopOptions{Timeout: &timeout})
		case "start":
			runErr = b.Sess.Docker.ContainerStart(ctx, fullID, dcontainer.StartOptions{})
		case "pause":
			runErr = b.Sess.Docker.ContainerPause(ctx, fullID)
		case "unpause":
			runErr = b.Sess.Docker.ContainerUnpause(ctx, fullID)
		case "remove":
			runErr = b.Sess.Docker.ContainerRemove(ctx, fullID, dcontainer.RemoveOptions{Force: body.Force})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ação inválida"})
			return
		}
		if runErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": runErr.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "action": action})
	}
}

// handleDockerLogs devolve últimas linhas de log de um contêiner.
func (s *Server) handleDockerLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id obrigatório"})
		return
	}
	tail := 200
	if t := strings.TrimSpace(r.URL.Query().Get("tail")); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			tail = n
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	fullID, err := resolveContainerID(ctx, b.Sess.Docker, id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	since := strings.TrimSpace(r.URL.Query().Get("since"))
	text, err := dockerutil.LoadContainerLogs(ctx, b.Sess.Docker, fullID, tail, since)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if text == "" {
		text = "(sem logs)"
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": text})
}

// handleDockerStats devolve estatísticas formatadas de um contêiner.
func (s *Server) handleDockerStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id obrigatório"})
		return
	}
	raw := r.URL.Query().Get("raw") == "1"
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	fullID, err := resolveContainerID(ctx, b.Sess.Docker, id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if raw {
		st, err := b.Sess.Docker.ContainerStats(ctx, fullID, false)
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
		return
	}
	m, err := fetchContainerMetrics(ctx, b.Sess.Docker, fullID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"metrics": m})
}

// handleDockerInspect devolve inspect resumido do contêiner.
func (s *Server) handleDockerInspect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id obrigatório"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	fullID, err := resolveContainerID(ctx, b.Sess.Docker, id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	ins, err := b.Sess.Docker.ContainerInspect(ctx, fullID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	out := map[string]any{
		"id":     dockerutil.ShortID(strings.TrimPrefix(ins.ID, "sha256:")),
		"idFull": ins.ID,
		"name":   strings.TrimPrefix(ins.Name, "/"),
		"image":  ins.Image,
	}
	if ins.Config != nil {
		out["hostname"] = ins.Config.Hostname
		out["labels"] = ins.Config.Labels
	}
	if ins.State != nil {
		out["state"] = ins.State.Status
		out["running"] = ins.State.Running
		out["startedAt"] = ins.State.StartedAt
		out["finishedAt"] = ins.State.FinishedAt
		out["exitCode"] = ins.State.ExitCode
	}
	out["restartCount"] = ins.RestartCount
	if ins.HostConfig != nil {
		out["restartPolicy"] = ins.HostConfig.RestartPolicy.Name
	}
	writeJSON(w, http.StatusOK, map[string]any{"inspect": out})
}
