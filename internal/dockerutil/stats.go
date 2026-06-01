package dockerutil

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	dcontainertypes "github.com/docker/docker/api/types/container"
)

// MetricsSummary métricas formatadas para a UI web.
type MetricsSummary struct {
	CPUPercent   float64 `json:"cpuPercent"`
	MemUsage     uint64  `json:"memUsage"`
	MemLimit     uint64  `json:"memLimit"`
	MemPercent   float64 `json:"memPercent"`
	NetRx        uint64  `json:"netRx"`
	NetTx        uint64  `json:"netTx"`
	DiskRead     uint64  `json:"diskRead"`
	DiskWrite    uint64  `json:"diskWrite"`
	Pids         uint64  `json:"pids"`
	RestartCount int     `json:"restartCount"`
	Uptime       string  `json:"uptime"`
	AlertLevel   int     `json:"alertLevel"`
	StatsLine    string  `json:"statsLine,omitempty"`
}

// ParseStatsResponse calcula métricas a partir da resposta de stats + inspect opcional.
func ParseStatsResponse(stats *dcontainertypes.StatsResponse, startedAt string, restartCount int) MetricsSummary {
	cpuPct := computeCPUPercent(*stats)
	memUsage := stats.MemoryStats.Usage
	memLimit := stats.MemoryStats.Limit
	memPct := 0.0
	if memLimit > 0 {
		memPct = (float64(memUsage) / float64(memLimit)) * 100
	}
	rx, tx := collectNetIO(*stats)
	rd, wr := collectBlockIO(*stats)
	pids := stats.PidsStats.Current
	uptime := CalcUptimeLabel(startedAt)
	alert := AlertLevel(cpuPct, memPct)
	line := fmt.Sprintf(
		"CPU %.1f%% · RAM %s / %s (%.0f%%) · rede ↓%s ↑%s · disco ↓%s ↑%s",
		math.Max(cpuPct, 0),
		FormatBytes(memUsage),
		FormatBytes(memLimit),
		math.Max(memPct, 0),
		FormatBytes(rx),
		FormatBytes(tx),
		FormatBytes(rd),
		FormatBytes(wr),
	)
	return MetricsSummary{
		CPUPercent:   cpuPct,
		MemUsage:     memUsage,
		MemLimit:     memLimit,
		MemPercent:   memPct,
		NetRx:        rx,
		NetTx:        tx,
		DiskRead:     rd,
		DiskWrite:    wr,
		Pids:         pids,
		RestartCount: restartCount,
		Uptime:       uptime,
		AlertLevel:   alert,
		StatsLine:    line,
	}
}

// DecodeStatsJSON lê o corpo JSON de ContainerStats.
func DecodeStatsJSON(body []byte) (*dcontainertypes.StatsResponse, error) {
	var stats dcontainertypes.StatsResponse
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

func computeCPUPercent(stats dcontainertypes.StatsResponse) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage) - float64(stats.PreCPUStats.SystemUsage)
	if cpuDelta <= 0 || systemDelta <= 0 {
		return 0
	}
	cpus := float64(stats.CPUStats.OnlineCPUs)
	if cpus < 1 {
		cpus = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
		if cpus < 1 {
			cpus = 1
		}
	}
	return (cpuDelta / systemDelta) * cpus * 100.0
}

func collectNetIO(stats dcontainertypes.StatsResponse) (rx, tx uint64) {
	for _, nw := range stats.Networks {
		rx += nw.RxBytes
		tx += nw.TxBytes
	}
	return rx, tx
}

func collectBlockIO(stats dcontainertypes.StatsResponse) (read, write uint64) {
	for _, e := range stats.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(strings.TrimSpace(e.Op)) {
		case "read":
			read += e.Value
		case "write":
			write += e.Value
		}
	}
	return read, write
}

// CalcUptimeLabel formata tempo desde startedAt (RFC3339).
func CalcUptimeLabel(startedAt string) string {
	t := strings.TrimSpace(startedAt)
	if t == "" {
		return "n/d"
	}
	start, err := time.Parse(time.RFC3339Nano, t)
	if err != nil {
		return "n/d"
	}
	d := time.Since(start)
	if d < 0 {
		return "n/d"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
}

// AlertLevel 0 normal, 1 aviso, 2 crítico.
func AlertLevel(cpuPct, memPct float64) int {
	if cpuPct >= 90 || memPct >= 90 {
		return 2
	}
	if cpuPct >= 75 || memPct >= 75 {
		return 1
	}
	return 0
}

// FormatBytes formata bytes em KiB/MiB/…
func FormatBytes(v uint64) string {
	if v < 1024 {
		return fmt.Sprintf("%d B", v)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	f := float64(v)
	idx := 0
	for f >= 1024 && idx < len(units)-1 {
		f /= 1024
		idx++
	}
	return fmt.Sprintf("%.1f %s", f, units[idx])
}

// StateLabelPT rótulo legível do estado.
func StateLabelPT(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running":
		return "Em execução"
	case "exited":
		return "Parado"
	case "paused":
		return "Pausado"
	case "restarting":
		return "A reiniciar"
	case "dead":
		return "Morto"
	case "created":
		return "Criado"
	case "removing":
		return "A remover"
	default:
		if state == "" {
			return "Desconhecido"
		}
		return state
	}
}
