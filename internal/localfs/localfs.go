package localfs

import (
	"os"
	"path/filepath"
	"strings"

	"containerway/internal/fsutil"
)

// List lista um diretório local (Windows).
func List(dir string) ([]fsutil.DirEntry, error) {
	d := strings.TrimSpace(dir)
	if d == "" {
		d = "."
	}
	d = filepath.Clean(d)
	infos, err := os.ReadDir(d)
	if err != nil {
		return nil, err
	}
	var out []fsutil.DirEntry
	parent := filepath.Dir(d)
	if parent != d {
		out = append(out, fsutil.DirEntry{Name: "..", Path: parent, IsDir: true})
	}
	for _, de := range infos {
		name := de.Name()
		if name == "." {
			continue
		}
		full := filepath.Join(d, name)
		fi, err := de.Info()
		if err != nil {
			continue
		}
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

func Mkdir(dir string) error {
	d := strings.TrimSpace(dir)
	if d == "" {
		return os.ErrInvalid
	}
	return os.Mkdir(filepath.Clean(d), 0o755)
}

func Rename(oldPath, newPath string) error {
	oldClean := filepath.Clean(strings.TrimSpace(oldPath))
	newClean := filepath.Clean(strings.TrimSpace(newPath))
	return os.Rename(oldClean, newClean)
}

func Remove(p string, recursive bool) error {
	clean := filepath.Clean(strings.TrimSpace(p))
	if recursive {
		return os.RemoveAll(clean)
	}
	return os.Remove(clean)
}
