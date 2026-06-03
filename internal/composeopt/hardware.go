package composeopt

// HostSpec descreve recursos do host SSH para recomendações de Compose.
type HostSpec struct {
	Hostname  string  `json:"hostname"`
	CPUs      int     `json:"cpus"`
	MemTotal  int64   `json:"memTotal"`
	MemFree   int64   `json:"memFree"`
	MemUsed   int64   `json:"memUsed"`
	MemPct    float64 `json:"memPct"`
	Load1     string  `json:"load1"`
	UpdatedAt string  `json:"updatedAt,omitempty"`
}

// FromOverview converte métricas do desktop overview para HostSpec.
func FromOverview(hostname string, cpus int, memTotal, memUsed, memFree int64, memPct float64, load1, updatedAt string) HostSpec {
	return HostSpec{
		Hostname:  hostname,
		CPUs:      cpus,
		MemTotal:  memTotal,
		MemFree:   memFree,
		MemUsed:   memUsed,
		MemPct:    memPct,
		Load1:     load1,
		UpdatedAt: updatedAt,
	}
}
