package webapp

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"containerway/internal/containerfs"
	"containerway/internal/hostfs"
	"containerway/internal/localfs"
)

type fsPathBody struct {
	Path string `json:"path"`
}

type fsRenameBody struct {
	OldPath string `json:"oldPath"`
	NewPath string `json:"newPath"`
}

type fsDeleteBody struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

// handleLocalMkdir cria pasta local.
func (s *Server) handleLocalMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	if _, _, ok := s.requireWebAuth(w, r); !ok {
		return
	}
	var body fsPathBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path obrigatório"})
		return
	}
	if localfs.IsWindowsDrivesVirtual(body.Path) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho inválido"})
		return
	}
	if err := localfs.Mkdir(body.Path); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleLocalRename renomeia ficheiro/pasta local.
func (s *Server) handleLocalRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	if _, _, ok := s.requireWebAuth(w, r); !ok {
		return
	}
	var body fsRenameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	if strings.TrimSpace(body.OldPath) == "" || strings.TrimSpace(body.NewPath) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminhos obrigatórios"})
		return
	}
	if err := localfs.Rename(body.OldPath, body.NewPath); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleLocalDelete apaga ficheiro/pasta local.
func (s *Server) handleLocalDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	if _, _, ok := s.requireWebAuth(w, r); !ok {
		return
	}
	var body fsDeleteBody
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}
	} else {
		body.Path = r.URL.Query().Get("path")
		body.Recursive = r.URL.Query().Get("recursive") == "1" || r.URL.Query().Get("recursive") == "true"
	}
	if strings.TrimSpace(body.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path obrigatório"})
		return
	}
	if err := localfs.Remove(body.Path, body.Recursive); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleRemoteMkdir cria pasta no host SFTP ou contêiner.
func (s *Server) handleRemoteMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var body struct {
		fsPathBody
		ContainerID string `json:"containerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path obrigatório"})
		return
	}
	cid := strings.TrimSpace(body.ContainerID)
	if cid != "" {
		cfs, err := containerFS(b, cid)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := cfs.Mkdir(r.Context(), body.Path); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		b.addOperation("Pasta criada (contêiner): "+body.Path, "info")
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	if err := hfs.Mkdir(body.Path); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	b.addOperation("Pasta criada (remoto): "+body.Path, "info")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleRemoteRename renomeia no host ou contêiner.
func (s *Server) handleRemoteRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var body struct {
		fsRenameBody
		ContainerID string `json:"containerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	if strings.TrimSpace(body.OldPath) == "" || strings.TrimSpace(body.NewPath) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminhos obrigatórios"})
		return
	}
	cid := strings.TrimSpace(body.ContainerID)
	if cid != "" {
		cfs, err := containerFS(b, cid)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := cfs.Rename(r.Context(), body.OldPath, body.NewPath); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		b.addOperation("Renomeado (contêiner): "+filepath.Base(body.NewPath), "info")
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	if err := hfs.Rename(body.OldPath, body.NewPath); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	b.addOperation("Renomeado (remoto): "+filepath.Base(body.NewPath), "info")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleRemoteDelete apaga no host ou contêiner.
func (s *Server) handleRemoteDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var body struct {
		fsDeleteBody
		ContainerID string `json:"containerId"`
	}
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}
	} else {
		body.Path = r.URL.Query().Get("path")
		body.ContainerID = r.URL.Query().Get("containerId")
		body.Recursive = r.URL.Query().Get("recursive") == "1" || r.URL.Query().Get("recursive") == "true"
	}
	if strings.TrimSpace(body.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path obrigatório"})
		return
	}
	cid := strings.TrimSpace(body.ContainerID)
	if cid != "" {
		cfs, err := containerFS(b, cid)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := cfs.Remove(r.Context(), body.Path, body.Recursive); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		b.addOperation("Apagado (contêiner): "+filepath.Base(body.Path), "info")
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	if err := hfs.Remove(body.Path, body.Recursive); err != nil {
		if isPermissionDenied(err) {
			b.sudoMu.Lock()
			sudoOn := b.sudoEnabled
			b.sudoMu.Unlock()
			if sudoOn {
				if errSudo := b.removeHostWithSudo(r.Context(), body.Path, body.Recursive); errSudo == nil {
					b.addOperation("Apagado (remoto/sudo): "+filepath.Base(body.Path), "info")
					writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
					return
				}
			}
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	b.addOperation("Apagado (remoto): "+filepath.Base(body.Path), "info")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleLocalShortcuts devolve atalhos do sistema local.
func (s *Server) handleLocalShortcuts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	if _, _, ok := s.requireWebAuth(w, r); !ok {
		return
	}
	home, _ := os.UserHomeDir()
	shortcuts := []map[string]string{
		{"label": "Home", "path": home},
	}
	if home != "" {
		shortcuts = append(shortcuts,
			map[string]string{"label": "Desktop", "path": filepath.Join(home, "Desktop")},
			map[string]string{"label": "Documentos", "path": filepath.Join(home, "Documents")},
			map[string]string{"label": "Downloads", "path": filepath.Join(home, "Downloads")},
		)
	}
	writeJSON(w, http.StatusOK, map[string]any{"shortcuts": shortcuts})
}

func containerFS(b *sshBundle, containerID string) (*containerfs.FS, error) {
	if b.Sess.Docker == nil {
		return nil, errDockerUnavailable
	}
	id := strings.TrimSpace(containerID)
	if id == "" {
		return nil, errInvalidContainer
	}
	return &containerfs.FS{Docker: b.Sess.Docker, ID: id}, nil
}

func parseRecursiveQuery(r *http.Request) bool {
	v := r.URL.Query().Get("recursive")
	if v == "" {
		return false
	}
	b, _ := strconv.ParseBool(v)
	return b || v == "1"
}
