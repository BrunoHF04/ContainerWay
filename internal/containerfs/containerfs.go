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

// List lista o conteúdo de um diretório no contêiner (via exec — compatível com Podman/Docker antigo).
func (f *FS) List(ctx context.Context, containerPath string) ([]fsutil.DirEntry, error) {
	p := path.Clean(containerPath)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	rows, err := f.listDirDirect(ctx, p)
	if err != nil {
		return nil, err
	}
	fsutil.SortLikeWinSCP(rows)
	return rows, nil
}

func shellQuotePath(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// listDirDirect lista pasta com ls dentro do contêiner.
func (f *FS) listDirDirect(ctx context.Context, p string) ([]fsutil.DirEntry, error) {
	quoted := shellQuotePath(p)
	stdout, stderr, code, err := f.execShell(ctx, "ls -1Ap -- "+quoted+" 2>/dev/null || ls -1Ap "+quoted)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = fmt.Sprintf("ls no contêiner falhou (código %d)", code)
		}
		// Pode ser um ficheiro único em vez de pasta
		if fi, errFile := f.statPathExec(ctx, p); errFile == nil && !fi.IsDir {
			return []fsutil.DirEntry{fi}, nil
		}
		return nil, fmt.Errorf("%s", msg)
	}

	var out []fsutil.DirEntry
	if p != "/" {
		parent := path.Dir(p)
		if parent == "" || parent == "." {
			parent = "/"
		}
		out = append(out, fsutil.DirEntry{Name: "..", Path: parent, IsDir: true})
	}

	sc := bufio.NewScanner(strings.NewReader(stdout))
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
	return out, nil
}

func (f *FS) statPathExec(ctx context.Context, p string) (fsutil.DirEntry, error) {
	quoted := shellQuotePath(p)
	stdout, stderr, code, err := f.execShell(ctx, "if [ -d "+quoted+" ]; then echo DIR; elif [ -f "+quoted+" ]; then echo FILE; else echo MISSING; fi")
	if err != nil {
		return fsutil.DirEntry{}, err
	}
	if code != 0 {
		return fsutil.DirEntry{}, fmt.Errorf("%s", strings.TrimSpace(stderr))
	}
	kind := strings.TrimSpace(stdout)
	if kind == "MISSING" {
		return fsutil.DirEntry{}, fmt.Errorf("caminho não encontrado no contêiner")
	}
	name := path.Base(p)
	if kind == "DIR" {
		return fsutil.DirEntry{Name: name, Path: p, IsDir: true, ModTime: time.Now()}, nil
	}
	return fsutil.DirEntry{Name: name, Path: p, IsDir: false, ModTime: time.Now()}, nil
}

// execShell corre comando shell no contêiner e devolve stdout, stderr e código de saída.
func (f *FS) execShell(ctx context.Context, shellCmd string) (stdout, stderr string, exitCode int, err error) {
	execCfg := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/sh", "-c", shellCmd},
	}
	created, err := f.Docker.ContainerExecCreate(ctx, f.ID, execCfg)
	if err != nil {
		return "", "", -1, err
	}
	resp, err := f.Docker.ContainerExecAttach(ctx, created.ID, container.ExecStartOptions{})
	if err != nil {
		return "", "", -1, err
	}
	defer resp.Close()
	var outBuf, errBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&outBuf, &errBuf, resp.Reader); err != nil {
		return "", "", -1, err
	}
	inspected, err := f.Docker.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return outBuf.String(), errBuf.String(), -1, err
	}
	return outBuf.String(), errBuf.String(), inspected.ExitCode, nil
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

const maxContainerCatBytes = 32 * 1024 * 1024

// OpenFileReader lê um ficheiro regular do contentor (exec cat; fallback tar se disponível).
func (f *FS) OpenFileReader(ctx context.Context, filePath string) (io.ReadCloser, int64, error) {
	p := path.Clean(filePath)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if fi, err := f.statPathExec(ctx, p); err == nil && fi.IsDir {
		return nil, 0, fmt.Errorf("é uma pasta: %s", p)
	}
	quoted := shellQuotePath(p)
	stdout, stderr, code, err := f.execShell(ctx, "cat -- "+quoted)
	if err == nil && code == 0 {
		data := []byte(stdout)
		if len(data) > maxContainerCatBytes {
			return nil, 0, fmt.Errorf("ficheiro demasiado grande no contêiner")
		}
		return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
	}
	if err == nil && code != 0 && strings.TrimSpace(stderr) != "" {
		return nil, 0, fmt.Errorf("%s", strings.TrimSpace(stderr))
	}
	// Fallback: API archive (Docker recente)
	rc, _, err := f.Docker.CopyFromContainer(ctx, f.ID, p)
	if err != nil {
		if code != 0 {
			msg := strings.TrimSpace(stderr)
			if msg == "" {
				msg = err.Error()
			}
			return nil, 0, fmt.Errorf("cat no contêiner: %s", msg)
		}
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
		return nil, 0, fmt.Errorf("esperava-se um ficheiro, mas veio uma pasta")
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
