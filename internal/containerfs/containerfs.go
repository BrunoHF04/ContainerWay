package containerfs

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"containerway/internal/fsutil"
	"github.com/docker/docker/api/types/container"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// FS opera sobre ficheiros dentro de um contentor via API Docker (arquivos tar).
type FS struct {
	Docker *docker.Client
	ID     string
}

// join executa parte da logica deste modulo.
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

	rows, err := f.listDirDirect(ctx, p)
	if err == nil {
		fsutil.SortLikeWinSCP(rows)
		return rows, nil
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

	totalHdr := 0
	accepted := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		totalHdr++

		name := strings.TrimPrefix(strings.TrimSpace(hdr.Name), "./")
		name = strings.TrimPrefix(name, "/")
		if name == "" || name == "." {
			continue
		}

		// CopyFromContainer devolve tar recursivo; aqui mostramos apenas os filhos diretos de p.
		rel := name
		if p != "/" {
			fullPrefix := strings.TrimPrefix(path.Clean(p), "/")
			basePrefix := path.Base(p)
			switch {
			case rel == fullPrefix, rel == basePrefix:
				rel = ""
			case strings.HasPrefix(rel, fullPrefix+"/"):
				rel = strings.TrimPrefix(rel, fullPrefix+"/")
			case strings.HasPrefix(rel, basePrefix+"/"):
				rel = strings.TrimPrefix(rel, basePrefix+"/")
			default:
				// Qualquer entrada fora da pasta pedida é descartada.
				continue
			}
		}
		rel = strings.TrimPrefix(rel, "./")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || rel == "." {
			continue
		}
		base := strings.Split(rel, "/")[0]
		if _, ok := seen[base]; ok {
			continue
		}
		seen[base] = struct{}{}

		isDir := strings.Contains(rel, "/") || hdr.Typeflag == tar.TypeDir || strings.HasSuffix(hdr.Name, "/")
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
		accepted++
	}
	fsutil.SortLikeWinSCP(out)
	return out, nil
}

// listDirDirect executa parte da logica deste modulo.
func (f *FS) listDirDirect(ctx context.Context, p string) ([]fsutil.DirEntry, error) {
	execCfg := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"ls", "-1Ap", p},
	}
	created, err := f.Docker.ContainerExecCreate(ctx, f.ID, execCfg)
	if err != nil {
		return nil, err
	}
	resp, err := f.Docker.ContainerExecAttach(ctx, created.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, err
	}
	defer resp.Close()

	var out []fsutil.DirEntry
	if p != "/" {
		parent := path.Dir(p)
		if parent == "" || parent == "." {
			parent = "/"
		}
		out = append(out, fsutil.DirEntry{Name: "..", Path: parent, IsDir: true})
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, resp.Reader); err != nil {
		return nil, err
	}
	if strings.TrimSpace(stderrBuf.String()) != "" {
		return nil, fmt.Errorf("%s", strings.TrimSpace(stderrBuf.String()))
	}

	sc := bufio.NewScanner(strings.NewReader(stdoutBuf.String()))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		isDir := strings.HasSuffix(line, "/")
		name := strings.TrimSuffix(line, "/")
		if name == "." || name == ".." || name == "" {
			continue
		}
		out = append(out, fsutil.DirEntry{
			Name:    name,
			Path:    f.join(p, name),
			IsDir:   isDir,
			Size:    0,
			ModTime: time.Now(),
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	inspected, err := f.Docker.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return nil, err
	}
	if inspected.ExitCode != 0 {
		return nil, fmt.Errorf("ls retornou código %d", inspected.ExitCode)
	}
	return out, nil
}

// runExec executa parte da logica deste modulo.
func (f *FS) runExec(ctx context.Context, cmd []string) error {
	execCfg := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}
	created, err := f.Docker.ContainerExecCreate(ctx, f.ID, execCfg)
	if err != nil {
		return err
	}
	resp, err := f.Docker.ContainerExecAttach(ctx, created.ID, container.ExecStartOptions{})
	if err != nil {
		return err
	}
	defer resp.Close()

	// Consome o stream para evitar bloqueios.
	_, _ = io.Copy(io.Discard, resp.Reader)
	inspected, err := f.Docker.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return err
	}
	if inspected.ExitCode != 0 {
		return fmt.Errorf("comando no contêiner falhou (exit %d): %s", inspected.ExitCode, strings.Join(cmd, " "))
	}
	return nil
}

// Mkdir executa parte da logica deste modulo.
func (f *FS) Mkdir(ctx context.Context, p string) error {
	target := path.Clean(p)
	if !strings.HasPrefix(target, "/") {
		target = "/" + target
	}
	return f.runExec(ctx, []string{"mkdir", target})
}

// Rename executa parte da logica deste modulo.
func (f *FS) Rename(ctx context.Context, oldPath, newPath string) error {
	oldTarget := path.Clean(oldPath)
	newTarget := path.Clean(newPath)
	if !strings.HasPrefix(oldTarget, "/") {
		oldTarget = "/" + oldTarget
	}
	if !strings.HasPrefix(newTarget, "/") {
		newTarget = "/" + newTarget
	}
	return f.runExec(ctx, []string{"mv", oldTarget, newTarget})
}

// Remove executa parte da logica deste modulo.
func (f *FS) Remove(ctx context.Context, p string, recursive bool) error {
	target := path.Clean(p)
	if !strings.HasPrefix(target, "/") {
		target = "/" + target
	}
	if recursive {
		return f.runExec(ctx, []string{"rm", "-rf", target})
	}
	return f.runExec(ctx, []string{"rm", "-f", target})
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
		return nil, 0, fmt.Errorf("é uma pasta: %s", p)
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
		return nil, 0, fmt.Errorf("esperava-se um arquivo, mas veio uma pasta")
	}
	return &tarFileReader{rc: rc, tr: tr}, hdr.Size, nil
}

type tarFileReader struct {
	rc io.ReadCloser
	tr *tar.Reader
}

// Read executa parte da logica deste modulo.
func (r *tarFileReader) Read(b []byte) (int, error) {
	return r.tr.Read(b)
}

// Close executa parte da logica deste modulo.
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
