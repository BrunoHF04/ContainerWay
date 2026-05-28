package webapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"containerway/internal/hostfs"
	"containerway/internal/localfs"
)

const maxEditorFileBytes = 2 * 1024 * 1024   // 2 MB texto
const maxImagePreviewBytes = 8 * 1024 * 1024 // 8 MB imagens

type fileEditBody struct {
	Path        string `json:"path"`
	ContainerID string `json:"containerId"`
	Content     string `json:"content"`
}

// handleLocalFile lê (GET) ou grava (PUT) ficheiro local para edição.
func (s *Server) handleLocalFile(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireWebAuth(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		p := strings.TrimSpace(r.URL.Query().Get("path"))
		if p == "" || localfs.IsWindowsDrivesVirtual(p) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho inválido"})
			return
		}
		if isImageFileName(p) {
			data, mime, err := readLocalImage(p)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, editorImageResponse(p, "", mime, data))
			return
		}
		data, err := readLocalFileForEditor(p)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"path":     p,
			"content":  string(data),
			"encoding": "utf-8",
		})
	case http.MethodPut:
		var body fileEditBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Path) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path e content obrigatórios"})
			return
		}
		if localfs.IsWindowsDrivesVirtual(body.Path) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho inválido"})
			return
		}
		if !utf8.ValidString(body.Content) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "conteúdo deve ser texto UTF-8"})
			return
		}
		if len(body.Content) > maxEditorFileBytes {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ficheiro demasiado grande para editar"})
			return
		}
		if err := os.WriteFile(body.Path, []byte(body.Content), 0o644); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

// handleRemoteFile lê (GET) ou grava (PUT) ficheiro remoto (host SFTP ou contêiner).
func (s *Server) handleRemoteFile(w http.ResponseWriter, r *http.Request) {
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		p := strings.TrimSpace(r.URL.Query().Get("path"))
		cid := strings.TrimSpace(r.URL.Query().Get("containerId"))
		if p == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path obrigatório"})
			return
		}
		if isImageFileName(p) {
			data, mime, err := readRemoteImage(ctx, b, p, cid)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, editorImageResponse(p, cid, mime, data))
			return
		}
		data, err := readRemoteFileForEditor(ctx, b, p, cid)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"path":        p,
			"containerId": cid,
			"content":     string(data),
			"encoding":    "utf-8",
		})
	case http.MethodPut:
		var body fileEditBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Path) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path e content obrigatórios"})
			return
		}
		if !utf8.ValidString(body.Content) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "conteúdo deve ser texto UTF-8"})
			return
		}
		if len(body.Content) > maxEditorFileBytes {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ficheiro demasiado grande para editar"})
			return
		}
		if err := writeRemoteFileForEditor(ctx, b, body.Path, body.ContainerID, []byte(body.Content)); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

func readLocalFileForEditor(p string) ([]byte, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, fmt.Errorf("é uma pasta, não um ficheiro")
	}
	if st.Size() > maxEditorFileBytes {
		return nil, fmt.Errorf("ficheiro demasiado grande (máx. %d MB)", maxEditorFileBytes/(1<<20))
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	if !isTextContent(data) {
		return nil, fmt.Errorf("ficheiro binário — não pode ser editado como texto")
	}
	return data, nil
}

func readRemoteFileForEditor(ctx context.Context, b *sshBundle, p, containerID string) ([]byte, error) {
	if strings.TrimSpace(containerID) != "" {
		cfs, err := containerFS(b, containerID)
		if err != nil {
			return nil, err
		}
		rc, size, err := cfs.OpenFileReader(ctx, p)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		if size > maxEditorFileBytes {
			return nil, fmt.Errorf("ficheiro demasiado grande (máx. %d MB)", maxEditorFileBytes/(1<<20))
		}
		return readLimitedText(rc, maxEditorFileBytes)
	}

	b.sudoMu.Lock()
	sudoOn := b.sudoEnabled
	b.sudoMu.Unlock()
	if sudoOn {
		return b.readHostFileWithSudo(ctx, p)
	}

	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	rf, err := hfs.OpenReader(p)
	if err != nil {
		if isPermissionDenied(err) {
			return nil, fmt.Errorf("permissão negada — active o sudo para ler este ficheiro")
		}
		return nil, err
	}
	defer rf.Close()
	st, err := rf.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > maxEditorFileBytes {
		return nil, fmt.Errorf("ficheiro demasiado grande (máx. %d MB)", maxEditorFileBytes/(1<<20))
	}
	return readLimitedText(rf, maxEditorFileBytes)
}

func writeRemoteFileForEditor(ctx context.Context, b *sshBundle, p, containerID string, data []byte) error {
	if strings.TrimSpace(containerID) != "" {
		cfs, err := containerFS(b, containerID)
		if err != nil {
			return err
		}
		dir := path.Dir(p)
		name := path.Base(p)
		if dir == "." {
			dir = "/"
		}
		return cfs.UploadFile(ctx, dir, name, bytes.NewReader(data), int64(len(data)))
	}

	b.sudoMu.Lock()
	sudoOn := b.sudoEnabled
	b.sudoMu.Unlock()
	if sudoOn {
		return b.writeHostFileWithSudo(ctx, p, data)
	}

	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	w, err := hfs.CreateWriter(p)
	if err != nil {
		if isPermissionDenied(err) {
			return fmt.Errorf("permissão negada — active o sudo para gravar este ficheiro")
		}
		return err
	}
	defer w.Close()
	if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
		return err
	}
	return nil
}

func readLimitedText(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("ficheiro demasiado grande (máx. %d MB)", max/(1<<20))
	}
	if !isTextContent(data) {
		return nil, fmt.Errorf("ficheiro binário — não pode ser editado como texto")
	}
	return data, nil
}

func isTextContent(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	return !bytes.Contains(sample, []byte{0}) && utf8.Valid(sample)
}

func isImageFileName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".ico":
		return true
	default:
		return false
	}
}

func mimeFromFileName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}

func editorImageResponse(path, containerID, mime string, data []byte) map[string]any {
	out := map[string]any{
		"path":     path,
		"encoding": "base64",
		"mimeType": mime,
		"content":  base64.StdEncoding.EncodeToString(data),
	}
	if containerID != "" {
		out["containerId"] = containerID
	}
	return out
}

func readLocalImage(p string) ([]byte, string, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, "", err
	}
	if st.IsDir() {
		return nil, "", fmt.Errorf("é uma pasta, não um ficheiro")
	}
	if st.Size() > maxImagePreviewBytes {
		return nil, "", fmt.Errorf("imagem demasiado grande (máx. %d MB)", maxImagePreviewBytes/(1<<20))
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, "", err
	}
	return data, mimeFromFileName(p), nil
}

func readRemoteImage(ctx context.Context, b *sshBundle, p, containerID string) ([]byte, string, error) {
	data, err := readRemoteBinary(ctx, b, p, containerID, maxImagePreviewBytes)
	if err != nil {
		return nil, "", err
	}
	return data, mimeFromFileName(p), nil
}

func readRemoteBinary(ctx context.Context, b *sshBundle, p, containerID string, max int64) ([]byte, error) {
	if max <= 0 {
		max = maxEditorFileBytes
	}
	if strings.TrimSpace(containerID) != "" {
		cfs, err := containerFS(b, containerID)
		if err != nil {
			return nil, err
		}
		rc, size, err := cfs.OpenFileReader(ctx, p)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		if size > max {
			return nil, fmt.Errorf("ficheiro demasiado grande (máx. %d MB)", max/(1<<20))
		}
		return readLimitedBytes(rc, max)
	}
	b.sudoMu.Lock()
	sudoOn := b.sudoEnabled
	b.sudoMu.Unlock()
	if sudoOn {
		data, err := b.readHostFileWithSudo(ctx, p)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > max {
			return nil, fmt.Errorf("ficheiro demasiado grande (máx. %d MB)", max/(1<<20))
		}
		return data, nil
	}
	hfs := &hostfs.FS{Client: b.Sess.SFTP}
	rf, err := hfs.OpenReader(p)
	if err != nil {
		if isPermissionDenied(err) {
			return nil, fmt.Errorf("permissão negada — active o sudo")
		}
		return nil, err
	}
	defer rf.Close()
	st, err := rf.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > max {
		return nil, fmt.Errorf("ficheiro demasiado grande (máx. %d MB)", max/(1<<20))
	}
	return readLimitedBytes(rf, max)
}

func readLimitedBytes(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("ficheiro demasiado grande (máx. %d MB)", max/(1<<20))
	}
	return data, nil
}
