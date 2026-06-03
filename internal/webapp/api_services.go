package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleServicesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	cmd := "sh -lc " + shellQuote(strings.TrimSpace(servicesListScript))
	out, stderr, err := b.runSSHCommand(ctx, cmd, "")
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg != "" {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": msg})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	rows, err := parseServicesList(out)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"services":  rows,
		"count":     len(rows),
		"updatedAt": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleServicesControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req desktopServiceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	unit, err := normalizeServiceUnit(req.Unit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	action := strings.TrimSpace(strings.ToLower(req.Action))
	script := scriptServiceAction(unit, action)
	if script == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ação inválida (start, stop, restart, enable ou disable)"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	out, stderr, err := b.runSudoScript(ctx, script)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(out)
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("%v — %s", err, msg)})
		return
	}
	b.addOperation(fmt.Sprintf("Serviço %s: %s", unit, action), "info")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": strings.TrimSpace(out)})
}

func (s *Server) handleServicesLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	unit, err := normalizeServiceUnit(r.URL.Query().Get("unit"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	n := 80
	if v := r.URL.Query().Get("lines"); v != "" {
		if i, e := strconv.Atoi(v); e == nil && i > 0 && i <= 500 {
			n = i
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	inner := fmt.Sprintf(
		"unit=%s; n=%d; if command -v journalctl >/dev/null 2>&1; then journalctl -u \"$unit\" -n \"$n\" --no-pager 2>/dev/null; fi; systemctl status \"$unit\" --no-pager -l 2>/dev/null | tail -n 40",
		unit, n,
	)
	cmd := "sh -lc " + shellQuote(inner)
	out, stderr, err := b.runSSHCommand(ctx, cmd, "")
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg != "" {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": msg})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	text := strings.TrimSpace(out)
	if text == "" {
		text = "(sem saída)"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"unit":  unit,
		"lines": n,
		"log":   text,
	})
}
