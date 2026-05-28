package connectcfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"containerway/internal/configdir"
)

// SavedConnection perfil de ligação SSH/SFTP persistido em disco.
type SavedConnection struct {
	Name            string `json:"name"`
	Host            string `json:"host"`
	User            string `json:"user"`
	Password        string `json:"password,omitempty"`
	KeyPath         string `json:"keyPath,omitempty"`
	KeyPass         string `json:"keyPass,omitempty"`
	KnownHosts      string `json:"knownHosts,omitempty"`
	InsecureHostKey bool   `json:"insecureHostKey"`
	ParallelJobs    string `json:"parallelJobs,omitempty"`
	DockerSocket    string `json:"dockerSocket,omitempty"`
}

// FilePath devolve o caminho de connections.json.
func FilePath() (string, error) {
	root, err := configdir.Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "connections.json"), nil
}

// LoadAll lê todos os perfis guardados.
func LoadAll() ([]SavedConnection, error) {
	p, err := FilePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []SavedConnection{}, nil
		}
		return nil, fmt.Errorf("não foi possível ler conexões salvas: %w", err)
	}
	var out []SavedConnection
	if len(strings.TrimSpace(string(b))) == 0 {
		return []SavedConnection{}, nil
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("arquivo de conexões inválido: %w", err)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// Save grava a lista completa de perfis.
func Save(list []SavedConnection) error {
	p, err := FilePath()
	if err != nil {
		return err
	}
	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("não foi possível serializar conexões: %w", err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return fmt.Errorf("não foi possível salvar conexões: %w", err)
	}
	return nil
}

// Upsert insere ou atualiza um perfil pelo nome.
func Upsert(list []SavedConnection, c SavedConnection) []SavedConnection {
	for i := range list {
		if strings.EqualFold(strings.TrimSpace(list[i].Name), strings.TrimSpace(c.Name)) {
			list[i] = c
			return list
		}
	}
	return append(list, c)
}

// RemoveByName remove um perfil pelo nome.
func RemoveByName(list []SavedConnection, name string) []SavedConnection {
	target := strings.TrimSpace(name)
	out := make([]SavedConnection, 0, len(list))
	for _, c := range list {
		if !strings.EqualFold(strings.TrimSpace(c.Name), target) {
			out = append(out, c)
		}
	}
	return out
}

// FindByName procura um perfil pelo nome.
func FindByName(list []SavedConnection, name string) (SavedConnection, bool) {
	target := strings.TrimSpace(name)
	for _, c := range list {
		if strings.EqualFold(strings.TrimSpace(c.Name), target) {
			return c, true
		}
	}
	return SavedConnection{}, false
}
