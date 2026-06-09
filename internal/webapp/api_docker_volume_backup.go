package webapp

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"containerway/internal/dockerutil"

	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

// selectBackupRestoreImage busca no host uma imagem pequena/offline para usar no contêiner auxiliar.
func selectBackupRestoreImage(ctx context.Context, cli client.APIClient) string {
	imgs, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil || len(imgs) == 0 {
		return "alpine:latest"
	}

	preferred := []string{"busybox", "alpine", "ubuntu", "debian", "nginx", "postgres", "redis", "node"}
	for _, img := range imgs {
		for _, tag := range img.RepoTags {
			tagLower := strings.ToLower(tag)
			for _, pref := range preferred {
				if strings.Contains(tagLower, pref) {
					return tag
				}
			}
		}
	}

	for _, img := range imgs {
		if len(img.RepoTags) > 0 && img.RepoTags[0] != "<none>:<none>" {
			return img.RepoTags[0]
		}
	}

	return "alpine:latest"
}

// copyTarAndStripPrefix remove o prefixo /volume da estrutura de tar gerada pelo docker.
func copyTarAndStripPrefix(src io.Reader, dst io.Writer, prefix string) (int64, error) {
	tr := tar.NewReader(src)
	tw := tar.NewWriter(dst)
	defer tw.Close()

	var bytesWritten int64
	prefix = strings.TrimPrefix(prefix, "/")
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return bytesWritten, err
		}

		name := strings.TrimPrefix(hdr.Name, "./")
		name = strings.TrimPrefix(name, "/")
		if name == strings.TrimSuffix(prefix, "/") || name == prefix {
			continue // Pula o diretório raiz (/volume)
		}

		if strings.HasPrefix(name, prefix) {
			hdr.Name = strings.TrimPrefix(name, prefix)
		} else {
			hdr.Name = name
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return bytesWritten, err
		}

		if hdr.Typeflag != tar.TypeDir {
			n, err := io.Copy(tw, tr)
			bytesWritten += n
			if err != nil {
				return bytesWritten, err
			}
		}
	}
	return bytesWritten, nil
}

// handleDockerVolumeBackup cria um backup tarball do volume completo.
func (s *Server) handleDockerVolumeBackup(w http.ResponseWriter, r *http.Request) {
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}

	var body struct {
		VolumeName string `json:"volumeName"`
		DestType   string `json:"destType"` // "local" | "ssh" | "download"
		DestPath   string `json:"destPath"`
	}

	if r.Method == http.MethodGet {
		body.VolumeName = r.URL.Query().Get("volumeName")
		body.DestType = r.URL.Query().Get("destType")
		body.DestPath = r.URL.Query().Get("destPath")
	} else if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dados da requisição inválidos"})
			return
		}
	} else {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}

	if strings.TrimSpace(body.VolumeName) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nome do volume é obrigatório"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()

	cli := b.Sess.Docker
	imageID := selectBackupRestoreImage(ctx, cli)

	// Cria o contêiner auxiliar montando o volume como RO (apenas leitura)
	resp, err := cli.ContainerCreate(ctx,
		&dcontainer.Config{
			Image: imageID,
			Cmd:   []string{"sleep", "3600"},
		},
		&dcontainer.HostConfig{
			Binds: []string{body.VolumeName + ":/volume:ro"},
		},
		nil, nil, "")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao criar contêiner auxiliar: %v", err)})
		return
	}

	defer func() {
		timeout := 0
		_ = cli.ContainerStop(ctx, resp.ID, dcontainer.StopOptions{Timeout: &timeout})
		_ = cli.ContainerRemove(ctx, resp.ID, dcontainer.RemoveOptions{Force: true})
	}()

	if err := cli.ContainerStart(ctx, resp.ID, dcontainer.StartOptions{}); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao iniciar contêiner auxiliar: %v", err)})
		return
	}

	// Obtém o stream do volume do contêiner
	reader, _, err := cli.CopyFromContainer(ctx, resp.ID, "/volume")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao copiar arquivos do volume: %v", err)})
		return
	}
	defer reader.Close()

	var outWriter io.Writer
	var closeFn func() error

	switch body.DestType {
	case "download":
		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="volume-%s.tar"`, body.VolumeName))
		w.WriteHeader(http.StatusOK)
		outWriter = w
	case "local":
		if strings.TrimSpace(body.DestPath) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho de destino local obrigatório"})
			return
		}
		if err := os.MkdirAll(filepath.Dir(body.DestPath), 0755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("falha ao criar diretório local: %v", err)})
			return
		}
		f, err := os.Create(body.DestPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("falha ao criar arquivo local: %v", err)})
			return
		}
		outWriter = f
		closeFn = f.Close
	case "ssh":
		if strings.TrimSpace(body.DestPath) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho de destino SSH obrigatório"})
			return
		}
		if b.Sess.SFTP == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SFTP não disponível nesta conexão SSH"})
			return
		}
		if err := b.Sess.SFTP.MkdirAll(path.Dir(body.DestPath)); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao criar diretório remoto via SFTP: %v", err)})
			return
		}
		f, err := b.Sess.SFTP.Create(body.DestPath)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao criar arquivo remoto via SFTP: %v", err)})
			return
		}
		outWriter = f
		closeFn = f.Close
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tipo de destino inválido"})
		return
	}

	if closeFn != nil {
		defer closeFn()
	}

	bytesWritten, err := copyTarAndStripPrefix(reader, outWriter, "volume")
	if err != nil {
		// Se foi download direto, não podemos alterar o cabeçalho HTTP de status pois já foi enviado,
		// mas para arquivos gravados em disco retornamos JSON de erro.
		if body.DestType != "download" {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao gravar tarball: %v", err)})
		}
		return
	}

	if body.DestType != "download" {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":       "success",
			"bytesWritten": bytesWritten,
			"sizeHuman":    dockerutil.FormatBytes(uint64(bytesWritten)),
		})
	}
}

// handleDockerVolumeRestore restaura os dados de um backup para o volume.
func (s *Server) handleDockerVolumeRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok || !dockerClientFromBundle(b, w) {
		return
	}

	var volumeName string
	var sourceType string
	var sourcePath string
	var tarReader io.Reader
	var closeFn func() error

	// Verifica se a requisição é multipart (upload do navegador) ou JSON (arquivos locais/SSH)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(50 << 20); err != nil { // limite de 50 MB em memória
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "falha ao ler form data"})
			return
		}
		volumeName = r.FormValue("volumeName")
		sourceType = "upload"
		file, _, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "arquivo não fornecido"})
			return
		}
		tarReader = file
		closeFn = file.Close
	} else {
		var body struct {
			VolumeName string `json:"volumeName"`
			SourceType string `json:"sourceType"` // "local" | "ssh"
			SourcePath string `json:"sourcePath"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.VolumeName) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dados da requisição inválidos"})
			return
		}
		volumeName = body.VolumeName
		sourceType = body.SourceType
		sourcePath = body.SourcePath

		switch sourceType {
		case "local":
			if strings.TrimSpace(sourcePath) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho de origem local obrigatório"})
				return
			}
			f, err := os.Open(sourcePath)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("arquivo local não encontrado: %v", err)})
				return
			}
			tarReader = f
			closeFn = f.Close
		case "ssh":
			if strings.TrimSpace(sourcePath) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho de origem SSH obrigatório"})
				return
			}
			if b.Sess.SFTP == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SFTP não disponível nesta conexão SSH"})
				return
			}
			f, err := b.Sess.SFTP.Open(sourcePath)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("arquivo remoto não encontrado via SFTP: %v", err)})
				return
			}
			tarReader = f
			closeFn = f.Close
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tipo de origem inválido"})
			return
		}
	}

	if closeFn != nil {
		defer closeFn()
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()

	cli := b.Sess.Docker
	imageID := selectBackupRestoreImage(ctx, cli)

	// Garante que o volume de destino existe
	_, err := cli.VolumeCreate(ctx, volume.CreateOptions{Name: volumeName})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao garantir/criar o volume: %v", err)})
		return
	}

	// Cria o contêiner auxiliar montando o volume como RW
	resp, err := cli.ContainerCreate(ctx,
		&dcontainer.Config{
			Image: imageID,
			Cmd:   []string{"sleep", "3600"},
		},
		&dcontainer.HostConfig{
			Binds: []string{volumeName + ":/volume"},
		},
		nil, nil, "")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao criar contêiner auxiliar para restauração: %v", err)})
		return
	}

	defer func() {
		timeout := 0
		_ = cli.ContainerStop(ctx, resp.ID, dcontainer.StopOptions{Timeout: &timeout})
		_ = cli.ContainerRemove(ctx, resp.ID, dcontainer.RemoveOptions{Force: true})
	}()

	if err := cli.ContainerStart(ctx, resp.ID, dcontainer.StartOptions{}); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao iniciar contêiner auxiliar para restauração: %v", err)})
		return
	}

	// Envia o tarball para o contêiner (dentro de /volume)
	err = cli.CopyToContainer(ctx, resp.ID, "/volume", tarReader, dcontainer.CopyToContainerOptions{})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao extrair backup no volume: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}
