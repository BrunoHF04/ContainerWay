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
	return out, nil
}
