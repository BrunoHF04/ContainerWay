package webapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"containerway/internal/foldercompare"
	"containerway/internal/tarxfer"
	"containerway/internal/transfer"
	"containerway/internal/fyneprefs"
	"containerway/internal/fsutil"
	"containerway/internal/hostfs"
	"containerway/internal/localfs"
)

type clipboardEntry struct {
	Source      string `json:"source"` // local, host, container
	Path        string `json:"path"`
	IsDir       bool   `json:"isDir"`
	ContainerID string `json:"containerId,omitempty"`
}

func (b *sshBundle) setClipboard(e *clipboardEntry) {
	b.clipboardMu.Lock()
	defer b.clipboardMu.Unlock()
	if e == nil {
		b.clipboard = nil
		return
	}
	c := *e
	b.clipboard = &c
}

func (b *sshBundle) getClipboard() *clipboardEntry {
	b.clipboardMu.Lock()
	defer b.clipboardMu.Unlock()
	if b.clipboard == nil {
		return nil
	}
	c := *b.clipboard
	return &c
}

// handleExplorerCompare compara conteúdo das pastas abertas nos dois painéis.
func (s *Server) handleExplorerCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	localPath := strings.TrimSpace(r.URL.Query().Get("localPath"))
	remotePath := strings.TrimSpace(r.URL.Query().Get("remotePath"))
	containerID := strings.TrimSpace(r.URL.Query().Get("containerId"))
	if localPath == "" || remotePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "localPath e remotePath obrigatórios"})
		return
	}
	leftRows, err := localfs.List(localPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var rightRows []fsutil.DirEntry
	if containerID != "" {
		cfs, err := containerFS(b, containerID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		rightRows, err = cfs.List(r.Context(), remotePath)
	} else {
		hfs := &hostfs.FS{Client: b.Sess.SFTP}
		rightRows, err = hfs.List(r.Context(), remotePath)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	rightTitle := "Host"
	if containerID != "" {
		rightTitle = "Contêiner"
	}
	rep := foldercompare.Build("Local", rightTitle, localPath, remotePath, leftRows, rightRows)
	writeJSON(w, http.StatusOK, rep)
}

// handleExplorerFavorites lista ou grava favoritos do explorador (partilhado com desktop).
func (s *Server) handleExplorerFavorites(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireWebAuth(w, r); !ok {
		return
	}
	side := strings.TrimSpace(r.URL.Query().Get("side"))
	key := fyneprefs.LeftFavoritesKey
	if side == "right" {
		key = fyneprefs.RightFavoritesKey
	}
	switch r.Method {
	case http.MethodGet:
		paths, err := fyneprefs.GetStringSlice(key)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"paths": paths})
	case http.MethodPut:
		var body struct {
			Paths []string `json:"paths"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}
		if err := fyneprefs.SetStringSlice(key, body.Paths); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

// handleExplorerClipboard copia metadados do item selecionado para colar noutro painel.
func (s *Server) handleExplorerClipboard(w http.ResponseWriter, r *http.Request) {
	tok, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	_ = tok
	switch r.Method {
	case http.MethodGet:
		cb := b.getClipboard()
		if cb == nil {
			writeJSON(w, http.StatusOK, map[string]any{"clipboard": nil})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"clipboard": cb})
	case http.MethodPost:
		var body clipboardEntry
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Path) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path obrigatório"})
			return
		}
		b.setClipboard(&body)
		writeJSON(w, http.StatusOK, map[string]string{"status": "copiado"})
	case http.MethodDelete:
		b.setClipboard(nil)
		writeJSON(w, http.StatusOK, map[string]string{"status": "limpo"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

type explorerPasteBody struct {
	TargetSource      string `json:"targetSource"` // local, host, container
	TargetPath        string `json:"targetPath"`
	TargetContainerID string `json:"targetContainerId,omitempty"`
}

// handleExplorerPaste cola item copiado no destino (transferência enfileirada).
func (s *Server) handleExplorerPaste(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	tok, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	cb := b.getClipboard()
	if cb == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nada copiado"})
		return
	}
	var body explorerPasteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	destDir := strings.TrimSpace(body.TargetPath)
	if destDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "targetPath obrigatório"})
		return
	}
	name := filepath.Base(cb.Path)
	if name == "" || name == "." {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "origem inválida"})
		return
	}

	src := cb.Source
	dst := body.TargetSource
	if src == dst && strings.TrimSpace(cb.ContainerID) == strings.TrimSpace(body.TargetContainerID) {
		// rename/move no mesmo sistema
		if err := s.pasteSameFS(r, b, cb, destDir, body.TargetContainerID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "movido"})
		return
	}

	// cross-panel: use transfer queue
	if err := s.enqueuePasteTransfer(tok, b, cb, body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado"})
}

func (s *Server) pasteSameFS(r *http.Request, b *sshBundle, cb *clipboardEntry, destDir, destContainerID string) error {
	base := filepath.Base(cb.Path)
	newPath := joinPathForSource(cb.Source, destDir, base, destContainerID)
	switch cb.Source {
	case "local":
		return localfs.Rename(cb.Path, newPath)
	case "host":
		hfs := &hostfs.FS{Client: b.Sess.SFTP}
		return hfs.Rename(cb.Path, newPath)
	case "container":
		cfs, err := containerFS(b, cb.ContainerID)
		if err != nil {
			return err
		}
		return cfs.Rename(r.Context(), cb.Path, newPath)
	default:
		return errInvalidPath
	}
}

func joinPathForSource(source, dir, name, containerID string) string {
	_ = containerID
	if source == "local" {
		return filepath.Join(dir, name)
	}
	return path.Join(dir, name)
}

func (s *Server) enqueuePasteTransfer(webTok string, b *sshBundle, cb *clipboardEntry, body explorerPasteBody) error {
	src := cb.Source
	dst := body.TargetSource
	destDir := strings.TrimSpace(body.TargetPath)
	destCID := strings.TrimSpace(body.TargetContainerID)
	srcCID := strings.TrimSpace(cb.ContainerID)

	switch {
	case src == "local" && dst == "host":
		remote := joinRemotePath(destDir, filepath.Base(cb.Path))
		s.enqueueTransfer(webTok, b, "Colar → host", func(ctx context.Context, on transfer.Progress) error {
			if cb.IsDir {
				return pushDir(ctx, b, cb.Path, remote, on)
			}
			st, err := os.Stat(cb.Path)
			if err != nil {
				return err
			}
			return pushOneFile(ctx, b, cb.Path, remote, st.Size(), on)
		})
	case src == "host" && dst == "local":
		local := joinLocalPath(destDir, path.Base(cb.Path))
		s.enqueueTransfer(webTok, b, "Colar → local", func(ctx context.Context, on transfer.Progress) error {
			if cb.IsDir {
				return pullDir(ctx, b, local, cb.Path, on)
			}
			return pullOneFile(ctx, b, local, cb.Path, on)
		})
	case src == "local" && dst == "container":
		if destCID == "" {
			return errInvalidContainer
		}
		s.enqueueTransfer(webTok, b, "Colar → contêiner", func(ctx context.Context, on transfer.Progress) error {
			if cb.IsDir {
				return tarxfer.UploadLocalDirToContainer(ctx, b.Sess.Docker, destCID, cb.Path, destDir)
			}
			return pasteLocalFileToContainer(ctx, b, destCID, cb.Path, destDir)
		})
	case src == "container" && dst == "local":
		if srcCID == "" {
			return errInvalidContainer
		}
		local := joinLocalPath(destDir, path.Base(cb.Path))
		s.enqueueTransfer(webTok, b, "Colar → local", func(ctx context.Context, on transfer.Progress) error {
			if cb.IsDir {
				_, err := tarxfer.ExtractContainerDirToLocal(ctx, b.Sess.Docker, srcCID, cb.Path, local)
				return err
			}
			return pullContainerFileToLocal(ctx, b, srcCID, cb.Path, local)
		})
	default:
		return errors.New("colar entre estes destinos ainda não suportado na web")
	}
	return nil
}
