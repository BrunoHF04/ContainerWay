package webapp

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"containerway/internal/automation"
	"containerway/internal/configdir"
)

var errDockerUnavailable = errors.New("Docker/Podman indisponível nesta conexão")

// handleAutomationsRules GET lista regras; PUT grava conjunto completo.
func (s *Server) handleAutomationsRules(w http.ResponseWriter, r *http.Request) {
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	path, err := automationConfigPath(b.Host)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		rules, err := automation.LoadRules(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
	case http.MethodPut:
		var body struct {
			Rules []automation.Rule `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}
		if err := validateAutomationRules(body.Rules); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := automation.SaveRules(path, body.Rules); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rules": body.Rules})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

// handleAutomationsHistory GET histórico; DELETE limpa.
func (s *Server) handleAutomationsHistory(w http.ResponseWriter, r *http.Request) {
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	path, err := automationHistoryPath(b.Host)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		lines, err := automation.LoadHistory(path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
	case http.MethodDelete:
		if err := automation.ClearHistory(path); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

// handleAutomationsEngine GET estado; POST inicia ou para o motor.
func (s *Server) handleAutomationsEngine(w http.ResponseWriter, r *http.Request) {
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"running": b.automationEngineRunning(),
		})
	case http.MethodPost:
		var body struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}
		switch strings.ToLower(strings.TrimSpace(body.Action)) {
		case "start":
			if b.Sess == nil || b.Sess.Docker == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": errDockerUnavailable.Error()})
				return
			}
			if err := b.startAutomationEngine(b.Host); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			_ = automation.AppendHistory(mustHistoryPath(b.Host), "Motor iniciado pela interface web.")
			writeJSON(w, http.StatusOK, map[string]any{"running": true})
		case "stop":
			b.stopAutomationEngine()
			writeJSON(w, http.StatusOK, map[string]any{"running": false})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action deve ser start ou stop"})
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

func mustHistoryPath(host string) string {
	p, _ := automationHistoryPath(host)
	return p
}

// validateAutomationRules valida IDs únicos e campos mínimos.
func validateAutomationRules(rules []automation.Rule) error {
	seen := map[string]bool{}
	for i, r := range rules {
		id := strings.TrimSpace(r.ID)
		if id == "" {
			return errors.New("cada regra precisa de id")
		}
		if seen[id] {
			return errors.New("ids de regras duplicados: " + id)
		}
		seen[id] = true
		if strings.TrimSpace(r.Name) == "" {
			return errors.New("regra sem nome na posição " + ruleIndexLabel(i))
		}
		if r.CooldownSec < 0 {
			return errors.New("cooldownSec inválido em " + id)
		}
	}
	return nil
}

func ruleIndexLabel(i int) string {
	return strconv.Itoa(i + 1)
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
