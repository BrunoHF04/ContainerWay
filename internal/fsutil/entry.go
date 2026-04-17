package fsutil

import "time"

// DirEntry representa um item listado num diretório (local, SFTP ou metadados de tar).
type DirEntry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}
