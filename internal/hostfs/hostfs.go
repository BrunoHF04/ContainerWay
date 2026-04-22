package hostfs

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/pkg/sftp"

	"containerway/internal/fsutil"
)

// FS acessa o sistema de ficheiros do host remoto via SFTP.
type FS struct {
	Client *sftp.Client
}

func (f *FS) List(ctx context.Context, dir string) ([]fsutil.DirEntry, error) {
	_ = ctx
	p := normalize(dir)
	infos, err := f.Client.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := make([]fsutil.DirEntry, 0, len(infos)+1)
	if p != "/" {
		parent := path.Dir(p)
		if parent == "" || parent == "." {
			parent = "/"
		}
		out = append(out, fsutil.DirEntry{Name: "..", Path: parent, IsDir: true})
	}
	for _, fi := range infos {
		name := fi.Name()
		if name == "." || name == ".." {
			continue
		}
		full := path.Join(p, name)
		out = append(out, fsutil.DirEntry{
			Name:    name,
			Path:    full,
			IsDir:   fi.IsDir(),
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
		})
	}
	fsutil.SortLikeWinSCP(out)
	return out, nil
}

func normalize(dir string) string {
	d := strings.TrimSpace(dir)
	if d == "" || d == "." {
		return "/"
	}
	if !strings.HasPrefix(d, "/") {
		d = "/" + d
	}
	return path.Clean(d)
}

// OpenReader abre um ficheiro remoto para leitura.
func (f *FS) OpenReader(path string) (*sftp.File, error) {
	return f.Client.Open(normalize(path))
}

// CreateWriter cria ou trunca um ficheiro remoto.
func (f *FS) CreateWriter(p string) (*sftp.File, error) {
	p = normalize(p)
	dir := path.Dir(p)
	if dir != "" && dir != "/" && dir != "." {
		_ = f.Client.MkdirAll(dir)
	}
	return f.Client.Create(p)
}

// Mkdir cria um diretório no host.
func (f *FS) Mkdir(p string) error {
	return f.Client.Mkdir(normalize(p))
}

func (f *FS) Rename(oldPath, newPath string) error {
	return f.Client.Rename(normalize(oldPath), normalize(newPath))
}

func (f *FS) Remove(p string, recursive bool) error {
	target := normalize(p)
	if !recursive {
		return f.Client.Remove(target)
	}
	return f.removeRecursive(target)
}

func (f *FS) removeRecursive(p string) error {
	fi, err := f.Client.Stat(p)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return f.Client.Remove(p)
	}
	entries, err := f.Client.ReadDir(p)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}
		child := path.Join(p, name)
		if e.IsDir() {
			if err := f.removeRecursive(child); err != nil {
				return err
			}
			continue
		}
		if err := f.Client.Remove(child); err != nil {
			return err
		}
	}
	if err := f.Client.RemoveDirectory(p); err != nil {
		return fmt.Errorf("não foi possível remover pasta %s: %w", p, err)
	}
	return nil
}

// Stat devolve metadados SFTP.
func (f *FS) Stat(p string) (os.FileInfo, error) {
	return f.Client.Stat(normalize(p))
}
