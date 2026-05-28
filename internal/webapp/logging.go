package webapp

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// InitLogging redireciona log para ficheiro quando não há consola (ex.: Windows GUI).
func InitLogging() {
	if os.Getenv("CONTAINERWAY_WEB_LOG_STDOUT") == "1" {
		return
	}
	if isTerminal(os.Stdout) {
		return
	}
	path, err := logFilePath()
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f))
	log.Printf("log em %s", path)
}

// logFilePath devolve o caminho do log da versão web.
func logFilePath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "ContainerWay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "web.log"), nil
}

// isTerminal indica se o stdout parece ser uma consola interativa.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
