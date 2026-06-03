package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	reHostLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	reIfaceName = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,32}$`)
	reSvcUnit   = regexp.MustCompile(`^[a-zA-Z0-9@._-]+\.service$`)
)

type desktopNetworkApplyReq struct {
	Hostname string   `json:"hostname"`
	DNS      []string `json:"dns"`
}

type desktopIfaceReq struct {
	Iface  string `json:"iface"`
	Action string `json:"action"`
}

type desktopServiceReq struct {
	Unit   string `json:"unit"`
	Action string `json:"action"`
}

func normalizeServiceUnit(u string) (string, error) {
	u = strings.TrimSpace(u)
	if u == "" {
		return "", fmt.Errorf("indique a unidade systemd")
	}
	if !strings.HasSuffix(u, ".service") {
		u += ".service"
	}
	if !reSvcUnit.MatchString(u) {
		return "", fmt.Errorf("unidade inválida")
	}
	return u, nil
}

func validateDNS(servers []string) ([]string, error) {
	if len(servers) == 0 {
		return nil, fmt.Errorf("indique pelo menos um servidor DNS")
	}
	out := make([]string, 0, len(servers))
	for _, s := range servers {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if ip := net.ParseIP(s); ip == nil {
			return nil, fmt.Errorf("DNS inválido: %s", s)
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("indique pelo menos um servidor DNS")
	}
	return out, nil
}

func scriptSetHostname(name string) string {
	q := shellQuote(name)
	return "set -e\nif command -v hostnamectl >/dev/null 2>&1; then\n  hostnamectl set-hostname " + q + "\nelse\n  hostname " + q + "\nfi\n"
}

func scriptSetDNS(servers []string) string {
	var b strings.Builder
	b.WriteString("set -e\n")
	b.WriteString("cp -a /etc/resolv.conf /etc/resolv.conf.cw-bak 2>/dev/null || true\n")
	b.WriteString("{\n")
	b.WriteString("  echo '# ContainerWay'\n")
	for _, s := range servers {
		b.WriteString(fmt.Sprintf("  echo 'nameserver %s'\n", s))
	}
	b.WriteString("} > /tmp/resolv.conf.cw && mv /tmp/resolv.conf.cw /etc/resolv.conf\n")
	return b.String()
}

func scriptIfaceLink(iface, action string) string {
	q := shellQuote(iface)
	if action == "down" {
		return "set -e\nip link set " + q + " down\n"
	}
	return "set -e\nip link set " + q + " up\n"
}

func scriptSystemctl(unit, action string) string {
	q := shellQuote(unit)
	switch action {
	case "start":
		return "set -e\nsystemctl start " + q + "\n"
	case "stop":
		return "set -e\nsystemctl stop " + q + "\n"
	case "restart":
		return "set -e\nsystemctl restart " + q + "\n"
	default:
		return ""
	}
}

func (s *Server) handleDesktopNetworkApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req desktopNetworkApplyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	host := strings.TrimSpace(req.Hostname)
	hasHost := host != ""
	var dns []string
	hasDNS := false
	if len(req.DNS) > 0 {
		var err error
		dns, err = validateDNS(req.DNS)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		hasDNS = true
	}
	if !hasHost && !hasDNS {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique hostname ou servidores DNS"})
		return
	}
	if hasHost && !reHostLabel.MatchString(host) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hostname inválido"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	var parts []string
	if hasHost {
		out, stderr, err := b.runSudoScript(ctx, scriptSetHostname(host))
		if err != nil {
			msg := strings.TrimSpace(stderr)
			if msg == "" {
				msg = strings.TrimSpace(out)
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("hostname: %v — %s", err, msg)})
			return
		}
		parts = append(parts, strings.TrimSpace(out))
		b.addOperation("Hostname alterado: "+host, "info")
	}
	if hasDNS {
		out, stderr, err := b.runSudoScript(ctx, scriptSetDNS(dns))
		if err != nil {
			msg := strings.TrimSpace(stderr)
			if msg == "" {
				msg = strings.TrimSpace(out)
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("DNS: %v — %s", err, msg)})
			return
		}
		if parts != nil {
			parts = append(parts, strings.TrimSpace(out))
		} else {
			parts = []string{strings.TrimSpace(out)}
		}
		b.addOperation("DNS atualizado no servidor", "info")
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": strings.Join(parts, "\n")})
}

func (s *Server) handleDesktopNetworkIface(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req desktopIfaceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	iface := strings.TrimSpace(req.Iface)
	action := strings.TrimSpace(strings.ToLower(req.Action))
	if iface == "" || iface == "lo" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "interface inválida"})
		return
	}
	if !reIfaceName.MatchString(iface) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "interface inválida"})
		return
	}
	if action != "up" && action != "down" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ação inválida (up ou down)"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	out, stderr, err := b.runSudoScript(ctx, scriptIfaceLink(iface, action))
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(out)
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("%v — %s", err, msg)})
		return
	}
	b.addOperation(fmt.Sprintf("Interface %s: %s", iface, action), "info")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": strings.TrimSpace(out)})
}

func (s *Server) handleDesktopServiceControl(w http.ResponseWriter, r *http.Request) {
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
	script := scriptSystemctl(unit, action)
	if script == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ação inválida (start, stop ou restart)"})
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
