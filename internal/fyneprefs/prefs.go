// Package fyneprefs lê e grava preferências Fyne (preferences.json) partilhadas com a app desktop.
package fyneprefs

import (
	"encoding/json"
	"os"
	"sync"

	"containerway/internal/configdir"
)

var prefsMu sync.Mutex

// loadMap lê o ficheiro de preferências como mapa chave → valor JSON.
func loadMap() (map[string]json.RawMessage, error) {
	path, err := configdir.FynePreferencesPath()
	if err != nil {
		return nil, err
	}
	bb, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]json.RawMessage{}, nil
		}
		return nil, err
	}
	if len(bb) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(bb, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	return m, nil
}

// saveMap grava o mapa completo de preferências.
func saveMap(m map[string]json.RawMessage) error {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	path, err := configdir.FynePreferencesPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	bb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bb, 0o644)
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}

func getString(m map[string]json.RawMessage, key, fallback string) string {
	raw, ok := m[key]
	if !ok || len(raw) == 0 {
		return fallback
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return fallback
}

func setString(m map[string]json.RawMessage, key, val string) {
	b, _ := json.Marshal(val)
	m[key] = b
}

func getBool(m map[string]json.RawMessage, key string) bool {
	return getString(m, key, "") == "true"
}

func setBool(m map[string]json.RawMessage, key string, v bool) {
	if v {
		setString(m, key, "true")
	} else {
		setString(m, key, "false")
	}
}
