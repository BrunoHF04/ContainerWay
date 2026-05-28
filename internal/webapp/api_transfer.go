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

// handleTransferPush envia ficheiro local → remoto (ficheiro único).
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
	if err != nil || st.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "selecione um ficheiro local válido"})
		return
	}
	name := fmt.Sprintf("Enviar %s → %s", filepath.Base(local), remote)
	b.TM.Enqueue(transfer.Job{
		Name: name,
		Run: func(ctx context.Context, on transfer.Progress) error {
			return pushOneFile(ctx, b, local, remote, st.Size(), on)
		},
	})
	go s.drainTransfers(tok, b)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado", "name": name})
}

// handleTransferPull recebe ficheiro remoto → local (ficheiro único).
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
	name := fmt.Sprintf("Receber %s → %s", remote, local)
	b.TM.Enqueue(transfer.Job{
		Name: name,
		Run: func(ctx context.Context, on transfer.Progress) error {
			return pullOneFile(ctx, b, local, remote, on)
		},
	})
	go s.drainTransfers(tok, b)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado", "name": name})
}

// drainTransfers processa fila de transferências da sessão.
func (s *Server) drainTransfers(webTok string, b *sshBundle) {
	if b == nil {
		return
	}
	ctx := context.Background()
	b.TM.DrainAsync(ctx, 2,
		func(j transfer.Job) { b.addTransferLog(j.Name, "running", "") },
		func(j transfer.Job, err error) {
			if err != nil {
				b.addTransferLog(j.Name, "error", err.Error())
				return
			}
			b.addTransferLog(j.Name, "ok", "")
		},
		nil,
	)
}

// pushOneFile copia um ficheiro local para o host remoto via SFTP.
func pushOneFile(ctx context.Context, b *sshBundle, local, remote string, total int64, on transfer.Progress) error {
	_ = ctx
	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	if st, err := hfs.Stat(remote); err == nil && st.Size() == total {
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

// pullOneFile copia um ficheiro remoto para o disco local.
func pullOneFile(ctx context.Context, b *sshBundle, local, remote string, on transfer.Progress) error {
	_ = ctx
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

// joinRemotePath junta pasta remota com nome de ficheiro.
func joinRemotePath(dir, name string) string {
	if dir == "" || dir == "/" {
		return path.Join("/", name)
	}
	return path.Join(dir, name)
}
