package webapp

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const desktopOverviewScript = `
hostname=$(hostname -s 2>/dev/null || hostname 2>/dev/null || echo server)
kernel=$(uname -sr 2>/dev/null || echo Linux)
if uptime -p >/dev/null 2>&1; then
  uptime=$(uptime -p 2>/dev/null | sed 's/^up //')
else
  uptime=$(uptime 2>/dev/null | sed -n 's/.*up \([^,]*\).*/\1/p')
fi
cpus=$(nproc 2>/dev/null || echo 1)
mt=0; mu=0; mf=0
if command -v free >/dev/null 2>&1; then
  read mt mu mf <<<"$(free -b 2>/dev/null | awk '/^Mem:/{print $2,$3,$7;exit}')"
fi
dt=0; du=0
if command -v df >/dev/null 2>&1; then
  read dt du <<<"$(df -B1 / 2>/dev/null | awk 'NR==2{print $2,$3;exit}')"
fi
load="0 0 0"
if [ -r /proc/loadavg ]; then
  load=$(awk '{print $1,$2,$3}' /proc/loadavg)
fi
printf '%s\n' "$hostname" "$kernel" "$uptime" "$cpus" "${mt:-0}" "${mu:-0}" "${mf:-0}" "${dt:-0}" "${du:-0}" "$load"
`

type desktopOverview struct {
	Hostname string  `json:"hostname"`
	OS       string  `json:"os"`
	Uptime   string  `json:"uptime"`
	CPUs     int     `json:"cpus"`
	MemTotal int64   `json:"memTotal"`
	MemUsed  int64   `json:"memUsed"`
	MemFree  int64   `json:"memFree"`
	MemPct   float64 `json:"memPct"`
	DiskTotal int64  `json:"diskTotal"`
	DiskUsed  int64  `json:"diskUsed"`
	DiskPct   float64 `json:"diskPct"`
	Load1     string  `json:"load1"`
	Load5     string  `json:"load5"`
	Load15    string  `json:"load15"`
	UpdatedAt string  `json:"updatedAt"`
}

func parseDesktopOverview(raw string) (desktopOverview, error) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 10 {
		return desktopOverview{}, fmt.Errorf("resposta incompleta do host")
	}
	out := desktopOverview{
		Hostname:  strings.TrimSpace(lines[0]),
		OS:        strings.TrimSpace(lines[1]),
		Uptime:    strings.TrimSpace(lines[2]),
		UpdatedAt: time.Now().Format("15:04:05"),
	}
	out.CPUs, _ = strconv.Atoi(strings.TrimSpace(lines[3]))
	out.MemTotal, _ = strconv.ParseInt(strings.TrimSpace(lines[4]), 10, 64)
	out.MemUsed, _ = strconv.ParseInt(strings.TrimSpace(lines[5]), 10, 64)
	out.MemFree, _ = strconv.ParseInt(strings.TrimSpace(lines[6]), 10, 64)
	out.DiskTotal, _ = strconv.ParseInt(strings.TrimSpace(lines[7]), 10, 64)
	out.DiskUsed, _ = strconv.ParseInt(strings.TrimSpace(lines[8]), 10, 64)
	if out.MemTotal > 0 {
		used := out.MemTotal - out.MemFree
		if used < 0 {
			used = out.MemUsed
		}
		out.MemPct = float64(used) / float64(out.MemTotal) * 100
	}
	if out.DiskTotal > 0 {
		out.DiskPct = float64(out.DiskUsed) / float64(out.DiskTotal) * 100
	}
	parts := strings.Fields(strings.TrimSpace(lines[9]))
	if len(parts) > 0 {
		out.Load1 = parts[0]
	}
	if len(parts) > 1 {
		out.Load5 = parts[1]
	}
	if len(parts) > 2 {
		out.Load15 = parts[2]
	}
	return out, nil
}

func (b *sshBundle) runDesktopOverview(ctx context.Context) (desktopOverview, error) {
	if b == nil || b.Sess == nil || b.Sess.SSH == nil {
		return desktopOverview{}, fmt.Errorf("sessão SSH indisponível")
	}
	cmd := "sh -lc " + shellQuote(strings.TrimSpace(desktopOverviewScript))
	out, stderr, err := b.runSSHCommand(ctx, cmd, "")
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg != "" {
			return desktopOverview{}, fmt.Errorf("%s", msg)
		}
		return desktopOverview{}, err
	}
	return parseDesktopOverview(out)
}

// handleDesktopOverview devolve métricas leves do host para o ambiente gráfico simplificado.
func (s *Server) handleDesktopOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	ov, err := b.runDesktopOverview(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ov)
}
