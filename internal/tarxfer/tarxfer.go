package tarxfer

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/pkg/sftp"
	docker "github.com/docker/docker/client"
)

// WriteLocalDirToTar escreve uma árvore local num tar (nomes com barras POSIX relativos a absDir).
func WriteLocalDirToTar(ctx context.Context, absDir string, out io.Writer) error {
	absDir = filepath.Clean(absDir)
	tw := tar.NewWriter(out)
	defer tw.Close()

	return filepath.WalkDir(absDir, func(full string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, err := filepath.Rel(absDir, full)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if d.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		f, err := os.Open(full)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, f); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	})
}

func posixSafeJoin(root, rel string) (string, error) {
	root = path.Clean(root)
	if !strings.HasPrefix(root, "/") {
		root = "/" + root
	}
	candidate := path.Clean(path.Join(root, filepath.ToSlash(rel)))
	if candidate != root && !strings.HasPrefix(candidate, root+"/") {
		return "", fmt.Errorf("caminho inválido no tar: %q", rel)
	}
	return candidate, nil
}

func localSafeJoin(root, rel string) (string, error) {
	root = filepath.Clean(root)
	candidate := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	if !strings.HasPrefix(candidate, root+string(os.PathSeparator)) && candidate != root {
		return "", fmt.Errorf("caminho inválido no tar: %q", rel)
	}
	return candidate, nil
}

func posixRel(root, full string) (string, error) {
	root = path.Clean(root)
	full = path.Clean(full)
	if root == full {
		return ".", nil
	}
	if root == "/" {
		return strings.TrimPrefix(full, "/"), nil
	}
	prefix := root + "/"
	if !strings.HasPrefix(full, prefix) {
		return "", fmt.Errorf("caminho fora da pasta raiz no servidor")
	}
	return strings.TrimPrefix(full, prefix), nil
}

// ExtractTarToLocalDir extrai um tar (ex.: de CopyFromContainer) para destRoot no Windows.
func ExtractTarToLocalDir(r io.Reader, destRoot string) (int64, error) {
	destRoot = filepath.Clean(destRoot)
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return 0, err
	}
	tr := tar.NewReader(r)
	var written int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return written, err
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if name == "" {
			continue
		}
		dst, err := localSafeJoin(destRoot, name)
		if err != nil {
			return written, err
		}
		isDir := hdr.Typeflag == tar.TypeDir || strings.HasSuffix(hdr.Name, "/")
		if isDir {
			if err := os.MkdirAll(dst, os.FileMode(hdr.Mode&0o1777)); err != nil {
				return written, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return written, err
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode&0o1777))
		if err != nil {
			return written, err
		}
		n, err := io.Copy(f, tr)
		_ = f.Close()
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

// ExtractTarToSFTP extrai um tar para caminhos POSIX no host remoto.
func ExtractTarToSFTP(c *sftp.Client, r io.Reader, destRootPosix string) (int64, error) {
	destRootPosix = path.Clean(destRootPosix)
	if !strings.HasPrefix(destRootPosix, "/") {
		destRootPosix = "/" + destRootPosix
	}
	if err := c.MkdirAll(destRootPosix); err != nil {
		return 0, err
	}
	tr := tar.NewReader(r)
	var written int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return written, err
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if name == "" {
			continue
		}
		dst, err := posixSafeJoin(destRootPosix, name)
		if err != nil {
			return written, err
		}
		if hdr.Typeflag == tar.TypeDir || strings.HasSuffix(hdr.Name, "/") {
			if err := c.MkdirAll(dst); err != nil {
				return written, err
			}
			continue
		}
		if err := c.MkdirAll(path.Dir(dst)); err != nil {
			return written, err
		}
		f, err := c.Create(dst)
		if err != nil {
			return written, err
		}
		n, err := io.Copy(f, tr)
		_ = f.Close()
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

// SFTPDownloadTree copia uma árvore remota (walker SFTP) para destLocalDir.
func SFTPDownloadTree(ctx context.Context, c *sftp.Client, remoteRoot, destLocalDir string) (int64, error) {
	destLocalDir = filepath.Clean(destLocalDir)
	remoteRoot = path.Clean(remoteRoot)
	var written int64
	w := c.Walk(remoteRoot)
	for w.Step() {
		if ctx.Err() != nil {
			return written, ctx.Err()
		}
		if w.Err() != nil {
			return written, w.Err()
		}
		rpath := w.Path()
		rel, err := posixRel(remoteRoot, rpath)
		if err != nil {
			return written, err
		}
		if rel == "." {
			if err := os.MkdirAll(destLocalDir, 0o755); err != nil {
				return written, err
			}
			continue
		}
		dst := filepath.Join(destLocalDir, filepath.FromSlash(rel))
		fi := w.Stat()
		if fi.IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return written, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return written, err
		}
		rf, err := c.Open(rpath)
		if err != nil {
			return written, err
		}
		out, err := os.Create(dst)
		if err != nil {
			_ = rf.Close()
			return written, err
		}
		n, err := io.Copy(out, rf)
		_ = rf.Close()
		_ = out.Close()
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

// SFTPUploadLocalTree envia uma pasta local para remoteDestDir (POSIX).
func SFTPUploadLocalTree(ctx context.Context, localRoot, remoteDestDir string, c *sftp.Client) (int64, error) {
	localRoot = filepath.Clean(localRoot)
	remoteDestDir = path.Clean(remoteDestDir)
	if !strings.HasPrefix(remoteDestDir, "/") {
		remoteDestDir = "/" + remoteDestDir
	}
	if err := c.MkdirAll(remoteDestDir); err != nil {
		return 0, err
	}
	var written int64
	err := filepath.WalkDir(localRoot, func(full string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, err := filepath.Rel(localRoot, full)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rp := path.Join(remoteDestDir, filepath.ToSlash(rel))
		if d.IsDir() {
			return c.MkdirAll(rp)
		}
		if err := c.MkdirAll(path.Dir(rp)); err != nil {
			return err
		}
		f, err := os.Open(full)
		if err != nil {
			return err
		}
		wf, err := c.Create(rp)
		if err != nil {
			_ = f.Close()
			return err
		}
		n, err := io.Copy(wf, f)
		_ = f.Close()
		_ = wf.Close()
		written += n
		return err
	})
	return written, err
}

// ExtractContainerDirToLocal obtém o tar de um diretório no contentor e extrai para disco local.
func ExtractContainerDirToLocal(ctx context.Context, cli *docker.Client, containerID, containerDir, localDest string) (int64, error) {
	rc, _, err := cli.CopyFromContainer(ctx, containerID, containerDir)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	return ExtractTarToLocalDir(rc, localDest)
}

// ExtractContainerDirToSFTP obtém o tar de um diretório no contentor e extrai via SFTP.
func ExtractContainerDirToSFTP(ctx context.Context, cli *docker.Client, containerID, containerDir, remoteDestPosix string, c *sftp.Client) (int64, error) {
	rc, _, err := cli.CopyFromContainer(ctx, containerID, containerDir)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	return ExtractTarToSFTP(c, rc, remoteDestPosix)
}

// UploadLocalDirToContainer envia uma pasta local como tar para dstDir no contentor.
func UploadLocalDirToContainer(ctx context.Context, cli *docker.Client, containerID, localAbs, dstContainerDir string) error {
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		err := WriteLocalDirToTar(ctx, localAbs, pw)
		_ = pw.CloseWithError(err)
		errCh <- err
	}()
	opts := container.CopyToContainerOptions{AllowOverwriteDirWithFile: true}
	err := cli.CopyToContainer(ctx, containerID, dstContainerDir, pr, opts)
	_ = pr.Close()
	werr := <-errCh
	if err != nil {
		return err
	}
	return werr
}
