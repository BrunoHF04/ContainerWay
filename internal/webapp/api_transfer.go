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
	LocalPath   string `json:"localPath"`
	RemotePath  string `json:"remotePath"`
	ContainerID string `json:"containerId,omitempty"`
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
	cid := strings.TrimSpace(body.ContainerID)
	if cid != "" {
		s.enqueueContainerPushHandler(w, r, tok, b, local, remote, cid)
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
	cid := strings.TrimSpace(body.ContainerID)
	if cid != "" {
		s.enqueueContainerPullHandler(w, r, tok, b, local, remote, cid)
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
	par := b.parallelJobs
	if par < 1 {
		par = 2
	}
	if par > 8 {
		par = 8
	}
	b.TM.DrainAsync(ctx, par,
		func(j transfer.Job) {
			b.addTransferLog(j.Name, "running", "")
			b.setTransferProgress(j.Name, 0, 0)
		},
		func(j transfer.Job, err error) {
			b.clearTransferProgress()
			if err != nil {
				b.finishTransferLog(j.Name, "error", err.Error())
				return
			}
			b.finishTransferLog(j.Name, "ok", "")
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

// pasteLocalFileToContainer envia um ficheiro local para pasta no contêiner.
func pasteLocalFileToContainer(ctx context.Context, b *sshBundle, containerID, localPath, destDir string) error {
	cfs, err := containerFS(b, containerID)
	if err != nil {
		return err
	}
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	return cfs.UploadFile(ctx, destDir, filepath.Base(localPath), f, st.Size())
}

// pullContainerFileToLocal obtém um ficheiro do contêiner para disco local.
func pullContainerFileToLocal(ctx context.Context, b *sshBundle, containerID, remotePath, localPath string) error {
	cfs, err := containerFS(b, containerID)
	if err != nil {
		return err
	}
	rc, _, err := cfs.OpenFileReader(ctx, remotePath)
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func (s *Server) enqueueContainerPushHandler(w http.ResponseWriter, r *http.Request, webTok string, b *sshBundle, local, remote, containerID string) {
	_ = r
	st, err := os.Stat(local)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho local inválido"})
		return
	}
	if st.IsDir() {
		remote = joinRemotePath(remote, filepath.Base(local))
		name := fmt.Sprintf("Enviar pasta → contêiner %s", path.Base(local))
		s.enqueueTransfer(webTok, b, name, func(ctx context.Context, on transfer.Progress) error {
			if on != nil {
				on(0, -1)
			}
			err := tarxfer.UploadLocalDirToContainer(ctx, b.Sess.Docker, containerID, local, remote)
			if on != nil {
				on(1, 1)
			}
			return err
		})
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado", "name": name, "kind": "dir"})
		return
	}
	name := fmt.Sprintf("Enviar → contêiner %s", filepath.Base(local))
	s.enqueueTransfer(webTok, b, name, func(ctx context.Context, on transfer.Progress) error {
		return pasteLocalFileToContainer(ctx, b, containerID, local, remote)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado", "name": name, "kind": "file"})
}

func (s *Server) enqueueContainerPullHandler(w http.ResponseWriter, r *http.Request, webTok string, b *sshBundle, local, remote, containerID string) {
	if b.Sess.Docker == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": errDockerUnavailable.Error()})
		return
	}
	stat, err := b.Sess.Docker.ContainerStatPath(r.Context(), containerID, remote)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if stat.Mode.IsDir() {
		localDir := joinLocalPath(local, path.Base(remote))
		name := fmt.Sprintf("Receber pasta do contêiner %s", path.Base(remote))
		s.enqueueTransfer(webTok, b, name, func(ctx context.Context, on transfer.Progress) error {
			if on != nil {
				on(0, -1)
			}
			_, err := tarxfer.ExtractContainerDirToLocal(ctx, b.Sess.Docker, containerID, remote, localDir)
			if on != nil {
				on(1, 1)
			}
			return err
		})
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado", "name": name, "kind": "dir"})
		return
	}
	localFile := joinLocalPath(local, path.Base(remote))
	name := fmt.Sprintf("Receber do contêiner %s", path.Base(remote))
	s.enqueueTransfer(webTok, b, name, func(ctx context.Context, on transfer.Progress) error {
		return pullContainerFileToLocal(ctx, b, containerID, remote, localFile)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado", "name": name, "kind": "file"})
}

// handleTransferBatch envia ou recebe vários itens visíveis/selecionados.
func (s *Server) handleTransferBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	tok, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var body struct {
		Direction   string `json:"direction"` // push | pull
		LocalDir    string `json:"localDir"`
		RemoteDir   string `json:"remoteDir"`
		ContainerID string `json:"containerId,omitempty"`
		Items       []struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			IsDir bool   `json:"isDir"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "items obrigatório"})
		return
	}
	for _, it := range body.Items {
		if it.Name == ".." {
			continue
		}
		if body.Direction == "push" {
			if it.IsDir {
				s.enqueueTransfer(tok, b, "Lote: "+it.Name, func(ctx context.Context, on transfer.Progress) error {
					if body.ContainerID != "" {
						return tarxfer.UploadLocalDirToContainer(ctx, b.Sess.Docker, body.ContainerID, it.Path, joinRemotePath(body.RemoteDir, it.Name))
					}
					return pushDir(ctx, b, it.Path, joinRemotePath(body.RemoteDir, it.Name), on)
				})
			} else {
				st, _ := os.Stat(it.Path)
				size := int64(0)
				if st != nil {
					size = st.Size()
				}
				remote := joinRemotePath(body.RemoteDir, it.Name)
				s.enqueueTransfer(tok, b, "Lote: "+it.Name, func(ctx context.Context, on transfer.Progress) error {
					if body.ContainerID != "" {
						return pasteLocalFileToContainer(ctx, b, body.ContainerID, it.Path, body.RemoteDir)
					}
					return pushOneFile(ctx, b, it.Path, remote, size, on)
				})
			}
		} else {
			local := joinLocalPath(body.LocalDir, it.Name)
			s.enqueueTransfer(tok, b, "Lote: "+it.Name, func(ctx context.Context, on transfer.Progress) error {
				if body.ContainerID != "" {
					if it.IsDir {
						_, err := tarxfer.ExtractContainerDirToLocal(ctx, b.Sess.Docker, body.ContainerID, it.Path, local)
						return err
					}
					return pullContainerFileToLocal(ctx, b, body.ContainerID, it.Path, local)
				}
				if it.IsDir {
					return pullDir(ctx, b, local, it.Path, on)
				}
				return pullOneFile(ctx, b, local, it.Path, on)
			})
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "enfileirado", "count": fmt.Sprintf("%d", len(body.Items))})
}

// handleTransferUpload recebe ficheiros do browser (multipart) e enfileira envio para o remoto.
func (s *Server) handleTransferUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	tok, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	const maxForm = 512 << 20 // 512 MiB
	if err := r.ParseMultipartForm(maxForm); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "upload inválido: " + err.Error()})
		return
	}
	remoteDir := strings.TrimSpace(r.FormValue("remoteDir"))
	if remoteDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "remoteDir obrigatório"})
		return
	}
	cid := strings.TrimSpace(r.FormValue("containerId"))
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nenhum ficheiro enviado"})
		return
	}
	uploadRoot := filepath.Join(os.TempDir(), "containerway-upload", tok)
	if err := os.MkdirAll(uploadRoot, 0o700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	enqueued := 0
	for _, fh := range files {
		rel := strings.TrimPrefix(strings.ReplaceAll(fh.Filename, "\\", "/"), "/")
		if rel == "" || rel == "." {
			continue
		}
		localPath := filepath.Join(uploadRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(localPath), 0o700); err != nil {
			continue
		}
		src, err := fh.Open()
		if err != nil {
			continue
		}
		dst, err := os.Create(localPath)
		if err != nil {
			src.Close()
			continue
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			src.Close()
			os.Remove(localPath)
			continue
		}
		dst.Close()
		src.Close()
		st, err := os.Stat(localPath)
		if err != nil {
			continue
		}
		name := fmt.Sprintf("Upload %s", rel)
		lp := localPath
		if cid != "" {
			dest := joinRemotePath(remoteDir, filepath.ToSlash(rel))
			s.enqueueTransfer(tok, b, name, func(ctx context.Context, on transfer.Progress) error {
				if st.IsDir() {
					return tarxfer.UploadLocalDirToContainer(ctx, b.Sess.Docker, cid, lp, dest)
				}
				return pasteLocalFileToContainer(ctx, b, cid, lp, path.Dir(dest))
			})
		} else {
			remoteFile := joinRemotePath(remoteDir, filepath.ToSlash(rel))
			size := st.Size()
			s.enqueueTransfer(tok, b, name, func(ctx context.Context, on transfer.Progress) error {
				return pushOneFile(ctx, b, lp, remoteFile, size, on)
			})
		}
		enqueued++
	}
	if enqueued == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "não foi possível gravar os ficheiros"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "enfileirado", "count": enqueued})
}

// handleTransferCancel remove jobs pendentes da fila (não cancela transferência em curso).
func (s *Server) handleTransferCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	n := b.TM.ClearQueue()
	b.addOperation("Fila de transferências cancelada ("+fmt.Sprintf("%d", n)+" pendente(s))", "info")
	writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
}

func copyHostFileToContainer(ctx context.Context, b *sshBundle, hostPath, containerID, destDir string) error {
	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	if _, err := hfs.Stat(hostPath); err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "cw-paste-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if err := pullOneFile(ctx, b, tmpPath, hostPath, nil); err != nil {
		return err
	}
	return pasteLocalFileToContainer(ctx, b, containerID, tmpPath, destDir)
}

func copyHostDirToContainer(ctx context.Context, b *sshBundle, hostPath, containerID, destDir string) error {
	tmpRoot, err := os.MkdirTemp("", "cw-paste-dir-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpRoot)
	localTree := filepath.Join(tmpRoot, filepath.Base(hostPath))
	if err := pullDir(ctx, b, localTree, hostPath, nil); err != nil {
		return err
	}
	return tarxfer.UploadLocalDirToContainer(ctx, b.Sess.Docker, containerID, localTree, joinRemotePath(destDir, filepath.Base(hostPath)))
}

func copyContainerFileToHost(ctx context.Context, b *sshBundle, containerID, remotePath, destHostPath string) error {
	tmp, err := os.CreateTemp("", "cw-paste-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if err := pullContainerFileToLocal(ctx, b, containerID, remotePath, tmpPath); err != nil {
		return err
	}
	st, err := os.Stat(tmpPath)
	if err != nil {
		return err
	}
	return pushOneFile(ctx, b, tmpPath, destHostPath, st.Size(), nil)
}
