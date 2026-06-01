package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"containerway/internal/foldercompare"
	"containerway/internal/fsutil"
	"containerway/internal/hostfs"
	"containerway/internal/localfs"
	"containerway/internal/tarxfer"
	"containerway/internal/transfer"
)

// handleExplorerOperations devolve ou limpa histórico de operações da sessão.
func (s *Server) handleExplorerOperations(w http.ResponseWriter, r *http.Request) {
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"lines": b.operationHistory()})
	case http.MethodDelete:
		b.clearOperationHistory()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

type syncMirrorBody struct {
	LocalPath     string `json:"localPath"`
	RemotePath    string `json:"remotePath"`
	ContainerID   string `json:"containerId,omitempty"`
	Direction     string `json:"direction"` // both | push | pull
}

// handleExplorerSyncMirror sincroniza pastas com base na comparação (enviar/receber diferenças).
func (s *Server) handleExplorerSyncMirror(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	tok, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var body syncMirrorBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	localPath := strings.TrimSpace(body.LocalPath)
	remotePath := strings.TrimSpace(body.RemotePath)
	if localPath == "" || remotePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "localPath e remotePath obrigatórios"})
		return
	}
	dir := strings.ToLower(strings.TrimSpace(body.Direction))
	if dir == "" {
		dir = "both"
	}
	leftRows, err := localfs.List(localPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cid := strings.TrimSpace(body.ContainerID)
	var rightRows []fsutil.DirEntry
	if cid != "" {
		cfs, err := containerFS(b, cid)
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
	rep := foldercompare.Build("Local", "Remoto", localPath, remotePath, leftRows, rightRows)
	enqueued := 0
	if dir == "both" || dir == "push" {
		for _, it := range rep.OnlyLeftItems {
			if enqueueMirrorPush(tok, s, b, localPath, remotePath, cid, it.Name, it.IsDir) {
				enqueued++
			}
		}
		for _, it := range rep.MismatchItems {
			if enqueueMirrorPush(tok, s, b, localPath, remotePath, cid, it.Name, it.IsDir) {
				enqueued++
			}
		}
	}
	if dir == "both" || dir == "pull" {
		for _, it := range rep.OnlyRightItems {
			if enqueueMirrorPull(tok, s, b, localPath, remotePath, cid, it.Name, it.IsDir) {
				enqueued++
			}
		}
	}
	b.addOperation("Sincronização espelhada: "+itoa(enqueued)+" tarefa(s) enfileirada(s)", "info")
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "enfileirado", "count": enqueued})
}

func enqueueMirrorPush(tok string, s *Server, b *sshBundle, localDir, remoteDir, cid, name string, isDir bool) bool {
	localPath := filepath.Join(localDir, name)
	if _, err := os.Stat(localPath); err != nil {
		return false
	}
	if cid != "" {
		s.enqueueTransfer(tok, b, "Sync → "+name, func(ctx context.Context, on transfer.Progress) error {
			if isDir {
				return tarxfer.UploadLocalDirToContainer(ctx, b.Sess.Docker, cid, localPath, joinRemotePath(remoteDir, name))
			}
			return pasteLocalFileToContainer(ctx, b, cid, localPath, remoteDir)
		})
	} else {
		remote := joinRemotePath(remoteDir, name)
		if isDir {
			s.enqueueTransfer(tok, b, "Sync → "+name, func(ctx context.Context, on transfer.Progress) error {
				return pushDir(ctx, b, localPath, remote, on)
			})
		} else {
			st, _ := os.Stat(localPath)
			size := int64(0)
			if st != nil {
				size = st.Size()
			}
			s.enqueueTransfer(tok, b, "Sync → "+name, func(ctx context.Context, on transfer.Progress) error {
				return pushOneFile(ctx, b, localPath, remote, size, on)
			})
		}
	}
	return true
}

func enqueueMirrorPull(tok string, s *Server, b *sshBundle, localDir, remoteDir, cid, name string, isDir bool) bool {
	localPath := filepath.Join(localDir, name)
	if cid != "" {
		remotePath := path.Join(remoteDir, name)
		s.enqueueTransfer(tok, b, "Sync ← "+name, func(ctx context.Context, on transfer.Progress) error {
			if isDir {
				_, err := tarxfer.ExtractContainerDirToLocal(ctx, b.Sess.Docker, cid, remotePath, localPath)
				return err
			}
			return pullContainerFileToLocal(ctx, b, cid, remotePath, localPath)
		})
	} else {
		remotePath := joinRemotePath(remoteDir, name)
		hfs := &hostfs.FS{Client: b.Sess.SFTP}
		rst, err := hfs.Stat(remotePath)
		if err != nil {
			return false
		}
		if rst.IsDir() {
			s.enqueueTransfer(tok, b, "Sync ← "+name, func(ctx context.Context, on transfer.Progress) error {
				return pullDir(ctx, b, localPath, remotePath, on)
			})
		} else {
			s.enqueueTransfer(tok, b, "Sync ← "+name, func(ctx context.Context, on transfer.Progress) error {
				return pullOneFile(ctx, b, localPath, remotePath, on)
			})
		}
	}
	return true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
