package appui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type savedConnection struct {
	Name            string `json:"name"`
	Host            string `json:"host"`
	User            string `json:"user"`
	Password        string `json:"password,omitempty"`
	KeyPath         string `json:"keyPath,omitempty"`
	KeyPass         string `json:"keyPass,omitempty"`
	KnownHosts      string `json:"knownHosts,omitempty"`
	InsecureHostKey bool   `json:"insecureHostKey"`
	ParallelJobs    string `json:"parallelJobs,omitempty"`
}

func connectionsFilePath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("não foi possível localizar diretório de configuração: %w", err)
	}
	dir := filepath.Join(cfgDir, "ContainerWay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("não foi possível criar diretório de configuração: %w", err)
	}
	return filepath.Join(dir, "connections.json"), nil
}

func loadSavedConnections() ([]savedConnection, error) {
	p, err := connectionsFilePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []savedConnection{}, nil
		}
		return nil, fmt.Errorf("não foi possível ler conexões salvas: %w", err)
	}
	var out []savedConnection
	if len(strings.TrimSpace(string(b))) == 0 {
		return []savedConnection{}, nil
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("arquivo de conexões inválido: %w", err)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func saveConnections(list []savedConnection) error {
	p, err := connectionsFilePath()
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

func upsertConnection(list []savedConnection, c savedConnection) []savedConnection {
	for i := range list {
		if strings.EqualFold(strings.TrimSpace(list[i].Name), strings.TrimSpace(c.Name)) {
			list[i] = c
			return list
		}
	}
	return append(list, c)
}

func removeConnectionByName(list []savedConnection, name string) []savedConnection {
	target := strings.TrimSpace(name)
	out := make([]savedConnection, 0, len(list))
	for _, c := range list {
		if !strings.EqualFold(strings.TrimSpace(c.Name), target) {
			out = append(out, c)
		}
	}
	return out
}

func findConnectionByName(list []savedConnection, name string) (savedConnection, bool) {
	target := strings.TrimSpace(name)
	for _, c := range list {
		if strings.EqualFold(strings.TrimSpace(c.Name), target) {
			return c, true
		}
	}
	return savedConnection{}, false
}
