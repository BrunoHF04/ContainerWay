package localfs

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"containerway/internal/fsutil"
)

// WindowsDrivesVirtualPath é um caminho sentinela usado só no Windows para listar
// todas as letras de unidade disponíveis (C:, D:, …). Não corresponde a pasta real no disco.
const WindowsDrivesVirtualPath = "__CW_WIN32_DRIVES__"

// IsWindowsDrivesVirtual indica se o caminho é a vista sintética de unidades do Windows.
func IsWindowsDrivesVirtual(p string) bool {
	return strings.TrimSpace(p) == WindowsDrivesVirtualPath
}

// List lista um diretório local (Windows).
func List(dir string) ([]fsutil.DirEntry, error) {
	d := strings.TrimSpace(dir)
	if IsWindowsDrivesVirtual(d) {
		return listWindowsDrives()
	}
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

func listWindowsDrives() ([]fsutil.DirEntry, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("lista de unidades só está disponível no Windows")
	}
	var out []fsutil.DirEntry
	for _, letter := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		root := string(letter) + ":\\"
		if _, err := os.Stat(root); err != nil {
			continue
		}
		out = append(out, fsutil.DirEntry{
			Name:    string(letter) + ":",
			Path:    root,
			IsDir:   true,
			ModTime: time.Time{},
		})
	}
	fsutil.SortLikeWinSCP(out)
	return out, nil
}

// Mkdir executa parte da logica deste modulo.
func Mkdir(dir string) error {
	d := strings.TrimSpace(dir)
	if d == "" {
		return os.ErrInvalid
	}
	return os.Mkdir(filepath.Clean(d), 0o755)
}

// Rename executa parte da logica deste modulo.
func Rename(oldPath, newPath string) error {
	oldClean := filepath.Clean(strings.TrimSpace(oldPath))
	newClean := filepath.Clean(strings.TrimSpace(newPath))
	return os.Rename(oldClean, newClean)
}

// Remove executa parte da logica deste modulo.
func Remove(p string, recursive bool) error {
	clean := filepath.Clean(strings.TrimSpace(p))
	if recursive {
		return os.RemoveAll(clean)
	}
	return os.Remove(clean)
}
