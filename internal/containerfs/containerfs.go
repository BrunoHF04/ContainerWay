package containerfs

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	docker "github.com/docker/docker/client"
	"containerway/internal/fsutil"
)

// FS opera sobre ficheiros dentro de um contentor via API Docker (arquivos tar).
type FS struct {
	Docker *docker.Client
	ID     string
}

func (f *FS) join(containerPath string, names ...string) string {
	base := strings.TrimSuffix(containerPath, "/")
	if base == "" {
		base = "/"
	}
	joined := path.Join(append([]string{base}, names...)...)
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	return path.Clean(joined)
}

// List lista o conteúdo de um diretório ou devolve um único ficheiro como entrada.
func (f *FS) List(ctx context.Context, containerPath string) ([]fsutil.DirEntry, error) {
	p := path.Clean(containerPath)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	stat, err := f.Docker.ContainerStatPath(ctx, f.ID, p)
	if err != nil {
		return nil, err
	}

	if !stat.Mode.IsDir() {
		return []fsutil.DirEntry{{
			Name:    stat.Name,
			Path:    p,
			IsDir:   false,
			Size:    stat.Size,
			ModTime: stat.Mtime,
		}}, nil
	}

	rc, _, err := f.Docker.CopyFromContainer(ctx, f.ID, p)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	seen := map[string]struct{}{}
	var out []fsutil.DirEntry

	if p != "/" {
		parent := path.Dir(p)
		if parent == "" || parent == "." {
			parent = "/"
		}
		out = append(out, fsutil.DirEntry{Name: "..", Path: parent, IsDir: true})
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		name := strings.TrimSuffix(hdr.Name, "/")
		name = strings.TrimPrefix(name, "./")
		if name == "" || name == "." {
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return nil, err
			}
			continue
		}
		base := path.Base(name)
		if base == "." || base == "" {
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return nil, err
			}
			continue
		}
		if _, ok := seen[base]; ok {
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return nil, err
			}
			continue
		}
		seen[base] = struct{}{}

		isDir := hdr.Typeflag == tar.TypeDir || strings.HasSuffix(hdr.Name, "/")
		mt := hdr.ModTime
		if mt.IsZero() {
			mt = time.Unix(hdr.ModTime.Unix(), 0)
		}
		out = append(out, fsutil.DirEntry{
			Name:    base,
			Path:    f.join(p, base),
			IsDir:   isDir,
			Size:    hdr.Size,
			ModTime: mt,
		})

		if _, err := io.Copy(io.Discard, tr); err != nil {
			return nil, err
		}
	}

	return out, nil
}

// OpenFileReader lê um ficheiro regular do contentor (primeiro membro do tar).
func (f *FS) OpenFileReader(ctx context.Context, filePath string) (io.ReadCloser, int64, error) {
	p := path.Clean(filePath)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	stat, err := f.Docker.ContainerStatPath(ctx, f.ID, p)
	if err != nil {
		return nil, 0, err
	}
	if stat.Mode.IsDir() {
		return nil, 0, fmt.Errorf("é um diretório: %s", p)
	}
	rc, _, err := f.Docker.CopyFromContainer(ctx, f.ID, p)
	if err != nil {
		return nil, 0, err
	}
	tr := tar.NewReader(rc)
	hdr, err := tr.Next()
	if err != nil {
		_ = rc.Close()
		return nil, 0, err
	}
	if hdr.FileInfo().IsDir() {
		_ = rc.Close()
		return nil, 0, fmt.Errorf("esperado ficheiro, obtido diretório")
	}
	return &tarFileReader{rc: rc, tr: tr}, hdr.Size, nil
}

type tarFileReader struct {
	rc io.ReadCloser
	tr *tar.Reader
}

func (r *tarFileReader) Read(b []byte) (int, error) {
	return r.tr.Read(b)
}

func (r *tarFileReader) Close() error {
	return r.rc.Close()
}

// UploadFile envia um ficheiro (stream) para o caminho destino no contentor.
func (f *FS) UploadFile(ctx context.Context, dstDir string, fileName string, content io.Reader, size int64) error {
	dstDir = path.Clean(dstDir)
	if !strings.HasPrefix(dstDir, "/") {
		dstDir = "/" + dstDir
	}
	pr, pw := io.Pipe()
	go func() {
		tw := tar.NewWriter(pw)
		hdr := &tar.Header{
			Name: fileName,
			Mode: 0o644,
			Size: size,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(tw, content); err != nil {
			_ = tw.Close()
			_ = pw.CloseWithError(err)
			return
		}
		if err := tw.Close(); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_ = pw.Close()
	}()

	opts := container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
	}
	return f.Docker.CopyToContainer(ctx, f.ID, dstDir, pr, opts)
}
