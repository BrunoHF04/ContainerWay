package webapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"containerway/internal/localfs"
)

type openExternalBody struct {
	Path        string `json:"path"`
	ContainerID string `json:"containerId"`
	Editor      string `json:"editor"` // default | notepad++
}

type syncExternalBody struct {
	SessionID string `json:"sessionId"`
}

// handleLocalOpenExternal abre ficheiro local com programa do SO (Notepad++, etc.).
func (s *Server) handleLocalOpenExternal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	if _, _, ok := s.requireWebAuth(w, r); !ok {
		return
	}
	var body openExternalBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	p := strings.TrimSpace(body.Path)
	if p == "" || localfs.IsWindowsDrivesVirtual(p) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho inválido"})
		return
	}
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ficheiro não encontrado"})
		return
	}
	if err := openWithEditor(p, body.Editor); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Ficheiro aberto no programa externo (alterações gravam no ficheiro local).",
	})
}

// handleRemoteOpenExternal descarrega ficheiro remoto para temp e abre no SO local.
func (s *Server) handleRemoteOpenExternal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var body openExternalBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	p := strings.TrimSpace(body.Path)
	if p == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path obrigatório"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	data, err := readRemoteBinary(ctx, b, p, strings.TrimSpace(body.ContainerID), maxEditorFileBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ext := filepath.Ext(p)
	tmp, err := os.CreateTemp("", "containerway-ext-*"+ext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := openWithEditor(tmpPath, body.Editor); err != nil {
		_ = os.Remove(tmpPath)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	sid := b.registerExternalEdit(tmpPath, p, strings.TrimSpace(body.ContainerID))
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"sessionId": sid,
		"message":   "Aberto localmente. Após editar, use «Sincronizar» no explorador para enviar de volta ao servidor.",
	})
}

// handleRemoteSyncExternal envia ficheiro temp editado de volta ao remoto.
func (s *Server) handleRemoteSyncExternal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var body syncExternalBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	sess, err := b.takeExternalEdit(strings.TrimSpace(body.SessionID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	data, err := os.ReadFile(sess.TempPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := writeRemoteFileForEditor(ctx, b, sess.RemotePath, sess.ContainerID, data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_ = os.Remove(sess.TempPath)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "Ficheiro sincronizado com o servidor."})
}

func (b *sshBundle) registerExternalEdit(tempPath, remotePath, containerID string) string {
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	b.externalMu.Lock()
	if b.externalEdits == nil {
		b.externalEdits = map[string]*externalEditSession{}
	}
	b.externalEdits[id] = &externalEditSession{
		TempPath:    tempPath,
		RemotePath:  remotePath,
		ContainerID: containerID,
	}
	b.externalMu.Unlock()
	return id
}

func (b *sshBundle) takeExternalEdit(id string) (*externalEditSession, error) {
	b.externalMu.Lock()
	defer b.externalMu.Unlock()
	sess, ok := b.externalEdits[id]
	if !ok {
		return nil, fmt.Errorf("sessão de edição externa expirada ou inválida")
	}
	delete(b.externalEdits, id)
	return sess, nil
}
