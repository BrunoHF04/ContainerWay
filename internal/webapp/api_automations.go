package webapp

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"containerway/internal/configdir"
)

type automationRuleJSON struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Trigger     string `json:"trigger"`
	Action      string `json:"action"`
	Target      string `json:"target"`
	CooldownSec int    `json:"cooldownSec"`
	Enabled     bool   `json:"enabled"`
	WebhookURL  string `json:"webhookURL,omitempty"`
}

// handleAutomationsRules devolve regras guardadas para o host da sessão.
func (s *Server) handleAutomationsRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	path, err := automationConfigPath(b.Host)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rules, err := loadAutomationRulesFile(path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

// handleAutomationsHistory devolve histórico persistido do host.
func (s *Server) handleAutomationsHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	path, err := automationHistoryPath(b.Host)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	bb, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]any{"events": []any{}})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var lines []string
	if len(strings.TrimSpace(string(bb))) > 0 {
		_ = json.Unmarshal(bb, &lines)
	}
	if lines == nil {
		lines = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

// automationConfigPath resolve ficheiro de regras por host.
func automationConfigPath(host string) (string, error) {
	root, err := configdir.Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "automations-"+sanitizeHost(host)+".json"), nil
}

// automationHistoryPath resolve ficheiro de histórico por host.
func automationHistoryPath(host string) (string, error) {
	root, err := configdir.Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "automations-history-"+sanitizeHost(host)+".json"), nil
}

// sanitizeHost normaliza host para nome de ficheiro.
func sanitizeHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.ReplaceAll(h, ":", "_")
	h = strings.ReplaceAll(h, "/", "_")
	h = strings.ReplaceAll(h, "\\", "_")
	h = strings.ReplaceAll(h, " ", "_")
	if h == "" {
		return "host-desconhecido"
	}
	return h
}

// loadAutomationRulesFile lê regras do JSON em disco.
func loadAutomationRulesFile(path string) ([]automationRuleJSON, error) {
	bb, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []automationRuleJSON{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(bb))) == 0 {
		return []automationRuleJSON{}, nil
	}
	var out []automationRuleJSON
	if err := json.Unmarshal(bb, &out); err != nil {
		return nil, err
	}
	return out, nil
}
