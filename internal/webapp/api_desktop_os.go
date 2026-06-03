package webapp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const desktopNetworkScript = `
hostname=$(hostname -f 2>/dev/null || hostname -s 2>/dev/null || echo localhost)
gw=$(ip route show default 2>/dev/null | awk '{print $3; exit}')
dns=""
if [ -r /etc/resolv.conf ]; then
  dns=$(grep -E '^nameserver' /etc/resolv.conf 2>/dev/null | awk '{print $2}' | paste -sd, -)
fi
echo "HOST $hostname"
echo "GW ${gw:-}"
echo "DNS ${dns:-}"
if command -v ip >/dev/null 2>&1; then
  for dev in $(ls /sys/class/net 2>/dev/null); do
    [ "$dev" = "lo" ] && continue
    state=$(cat "/sys/class/net/$dev/operstate" 2>/dev/null | tr 'A-Z' 'a-z')
    ipv4=$(ip -4 -o addr show dev "$dev" 2>/dev/null | awk '{print $4}' | paste -sd, -)
    mac=$(ip link show dev "$dev" 2>/dev/null | awk '/link\/ether/{print $2; exit}')
    echo "IF ${dev}|${state:-unknown}|${ipv4:-}|${mac:-}"
  done
fi
`

const desktopServicesScript = `
if command -v systemctl >/dev/null 2>&1; then
  systemctl list-units --type=service --state=running,failed --no-legend --no-pager 2>/dev/null | head -100 | while read -r unit _ active sub desc; do
    echo "SVC ${unit}|${active}|${sub}|${desc}"
  done
else
  echo "SVC —|—|—|systemctl não disponível neste sistema"
fi
`

type desktopNetIface struct {
	Name  string   `json:"name"`
	State string   `json:"state"`
	IPv4  []string `json:"ipv4"`
	MAC   string   `json:"mac"`
}

type desktopNetwork struct {
	Hostname     string            `json:"hostname"`
	DefaultRoute string            `json:"defaultRoute"`
	DNS          []string          `json:"dns"`
	Interfaces   []desktopNetIface `json:"interfaces"`
	UpdatedAt    string            `json:"updatedAt"`
}

type desktopService struct {
	Unit        string `json:"unit"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Description string `json:"description"`
}

func parseDesktopNetwork(raw string) (desktopNetwork, error) {
	out := desktopNetwork{UpdatedAt: time.Now().Format("15:04:05")}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "HOST ") {
			out.Hostname = strings.TrimSpace(line[5:])
			continue
		}
		if strings.HasPrefix(line, "GW ") {
			out.DefaultRoute = strings.TrimSpace(line[3:])
			continue
		}
		if strings.HasPrefix(line, "DNS ") {
			d := strings.TrimSpace(line[4:])
			if d != "" {
				out.DNS = strings.Split(d, ",")
			}
			continue
		}
		if strings.HasPrefix(line, "IF ") {
			parts := strings.SplitN(line[3:], "|", 4)
			if len(parts) < 2 {
				continue
			}
			iface := desktopNetIface{Name: parts[0], State: parts[1]}
			if len(parts) > 2 && parts[2] != "" {
				iface.IPv4 = strings.Split(parts[2], ",")
			}
			if len(parts) > 3 {
				iface.MAC = parts[3]
			}
			if iface.Name != "lo" || len(iface.IPv4) > 0 {
				out.Interfaces = append(out.Interfaces, iface)
			}
		}
	}
	if out.Hostname == "" {
		return out, fmt.Errorf("resposta de rede incompleta")
	}
	return out, nil
}

func parseDesktopServices(raw string) []desktopService {
	var out []desktopService
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "SVC ") {
			continue
		}
		parts := strings.SplitN(line[4:], "|", 4)
		if len(parts) < 2 {
			continue
		}
		s := desktopService{Unit: parts[0], Active: parts[1]}
		if len(parts) > 2 {
			s.Sub = parts[2]
		}
		if len(parts) > 3 {
			s.Description = parts[3]
		}
		out = append(out, s)
	}
	return out
}

func (s *Server) handleDesktopNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cmd := "sh -lc " + shellQuote(strings.TrimSpace(desktopNetworkScript))
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
	net, err := parseDesktopNetwork(out)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, net)
}

func (s *Server) handleDesktopServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	cmd := "sh -lc " + shellQuote(strings.TrimSpace(desktopServicesScript))
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
	services := parseDesktopServices(out)
	writeJSON(w, http.StatusOK, map[string]any{
		"services":  services,
		"count":     len(services),
		"updatedAt": time.Now().Format("15:04:05"),
	})
}
