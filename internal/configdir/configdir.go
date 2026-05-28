package configdir

import (
	"fmt"
	"os"
	"path/filepath"
)

// Root devolve o diretório de configuração do ContainerWay (%AppData%/ContainerWay ou equivalente).
func Root() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("não foi possível localizar diretório de configuração: %w", err)
	}
	dir := filepath.Join(cfg, "ContainerWay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("não foi possível criar diretório de configuração: %w", err)
	}
	return dir, nil
}

// FynePreferencesPath devolve o ficheiro de preferências da app desktop Fyne (io.containerway.app).
func FynePreferencesPath() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "io.containerway.app", "preferences.json"), nil
}
