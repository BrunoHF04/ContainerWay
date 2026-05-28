package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"

	"containerway/internal/hostfs"
	"containerway/internal/tarxfer"
	"containerway/internal/transfer"
)

// handleTransferStatus devolve estado da fila de transferências.
func (s *Server) handleTransferStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, b.transferStatus())
}

type transferPathsBody struct {
	LocalPath  string `json:"localPath"`
	RemotePath string `json:"remotePath"`
}

// handleTransferPush envia ficheiro ou pasta local → remoto.
func (s *Server) handleTransferPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	tok, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var body transferPathsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	local := strings.TrimSpace(body.LocalPath)
	remote := strings.TrimSpace(body.RemotePath)
	if local == "" || remote == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminhos obrigatórios"})
		return
	}
	st, err := os.Stat(local)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho local inválido"})
		return
	}
	if st.IsDir() {
		remote = joinRemotePath(remote, filepath.Base(local))
		name := fmt.Sprintf("Enviar pasta %s → %s", local, remote)
		s.enqueueTransfer(tok, b, name, func(ctx context.Context, on transfer.Progress) error {
			return pushDir(ctx, b, local, remote, on)
		})
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado", "name": name, "kind": "dir"})
		return
	}
	remoteFile := joinRemotePath(remote, filepath.Base(local))
	name := fmt.Sprintf("Enviar %s → %s", filepath.Base(local), remoteFile)
	s.enqueueTransfer(tok, b, name, func(ctx context.Context, on transfer.Progress) error {
		return pushOneFile(ctx, b, local, remoteFile, st.Size(), on)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado", "name": name, "kind": "file"})
}

// handleTransferPull recebe ficheiro ou pasta remota → local.
func (s *Server) handleTransferPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	tok, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var body transferPathsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	local := strings.TrimSpace(body.LocalPath)
	remote := strings.TrimSpace(body.RemotePath)
	if local == "" || remote == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminhos obrigatórios"})
		return
	}
	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	rst, err := hfs.Stat(remote)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho remoto inválido"})
		return
	}
	if rst.IsDir() {
		local = joinLocalPath(local, path.Base(remote))
		name := fmt.Sprintf("Receber pasta %s → %s", remote, local)
		s.enqueueTransfer(tok, b, name, func(ctx context.Context, on transfer.Progress) error {
			return pullDir(ctx, b, local, remote, on)
		})
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado", "name": name, "kind": "dir"})
		return
	}
	localFile := joinLocalPath(local, path.Base(remote))
	name := fmt.Sprintf("Receber %s → %s", path.Base(remote), localFile)
	s.enqueueTransfer(tok, b, name, func(ctx context.Context, on transfer.Progress) error {
		return pullOneFile(ctx, b, localFile, remote, on)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado", "name": name, "kind": "file"})
}

// enqueueTransfer adiciona job à fila e inicia drenagem.
func (s *Server) enqueueTransfer(webTok string, b *sshBundle, name string, run func(context.Context, transfer.Progress) error) {
	b.TM.Enqueue(transfer.Job{Name: name, Run: run})
	go s.drainTransfers(webTok, b)
}

// drainTransfers processa fila de transferências da sessão.
func (s *Server) drainTransfers(webTok string, b *sshBundle) {
	if b == nil {
		return
	}
	ctx := context.Background()
	b.TM.DrainAsync(ctx, 2,
		func(j transfer.Job) {
			b.addTransferLog(j.Name, "running", "")
			b.setTransferProgress(j.Name, 0, 0)
		},
		func(j transfer.Job, err error) {
			b.clearTransferProgress()
			if err != nil {
				b.addTransferLog(j.Name, "error", err.Error())
				return
			}
			b.addTransferLog(j.Name, "ok", "")
		},
		func(done, total int64) {
			b.updateTransferProgress(done, total)
		},
	)
}

// pushOneFile copia um ficheiro local para o host remoto via SFTP.
func pushOneFile(ctx context.Context, b *sshBundle, local, remote string, total int64, on transfer.Progress) error {
	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	if st, err := hfs.Stat(remote); err == nil && !st.IsDir() && st.Size() == total {
		if on != nil {
			on(total, total)
		}
		return nil
	}
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	wf, err := hfs.CreateWriter(remote)
	if err != nil {
		return err
	}
	defer wf.Close()
	var done atomic.Int64
	pr := &transfer.CountingReader{R: f, N: &done, Total: total}
	if on != nil {
		on(0, total)
	}
	if _, err := io.Copy(wf, pr); err != nil {
		return err
	}
	if on != nil {
		on(total, total)
	}
	return nil
}

// pushDir envia pasta local completa para o host remoto.
func pushDir(ctx context.Context, b *sshBundle, local, remote string, on transfer.Progress) error {
	if on != nil {
		on(0, -1)
	}
	n, err := tarxfer.SFTPUploadLocalTree(ctx, local, remote, b.Sess.SFTP)
	if on != nil {
		total := n
		if total < 1 {
			total = 1
		}
		on(total, total)
	}
	return err
}

// pullOneFile copia um ficheiro remoto para o disco local.
func pullOneFile(ctx context.Context, b *sshBundle, local, remote string, on transfer.Progress) error {
	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	rf, err := hfs.OpenReader(remote)
	if err != nil {
		return err
	}
	defer rf.Close()
	st, err := rf.Stat()
	if err != nil {
		return err
	}
	total := st.Size()
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		return err
	}
	out, err := os.Create(local)
	if err != nil {
		return err
	}
	defer out.Close()
	var done atomic.Int64
	pr := &transfer.CountingReader{R: rf, N: &done, Total: total}
	if on != nil {
		on(0, total)
	}
	if _, err := io.Copy(out, pr); err != nil {
		return err
	}
	if on != nil {
		on(total, total)
	}
	return nil
}

// pullDir recebe pasta remota completa para disco local.
func pullDir(ctx context.Context, b *sshBundle, local, remote string, on transfer.Progress) error {
	if on != nil {
		on(0, -1)
	}
	n, err := tarxfer.SFTPDownloadTree(ctx, b.Sess.SFTP, remote, local)
	if on != nil {
		total := n
		if total < 1 {
			total = 1
		}
		on(total, total)
	}
	return err
}

// joinRemotePath junta pasta remota com nome relativo.
func joinRemotePath(dir, name string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" || dir == "/" {
		return path.Join("/", name)
	}
	return path.Join(dir, name)
}

// joinLocalPath junta pasta local com nome relativo.
func joinLocalPath(dir, name string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return name
	}
	return filepath.Join(dir, name)
}
