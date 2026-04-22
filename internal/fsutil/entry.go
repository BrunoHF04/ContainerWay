package fsutil

import (
	"sort"
	"strings"
	"time"
)

// DirEntry representa um item listado num diretório (local, SFTP ou metadados de tar).
type DirEntry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// SortLikeWinSCP ordena entradas com ".." primeiro, depois pastas e por fim arquivos (A-Z).
func SortLikeWinSCP(entries []DirEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Name == ".." && b.Name != ".." {
			return true
		}
		if b.Name == ".." && a.Name != ".." {
			return false
		}
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}
