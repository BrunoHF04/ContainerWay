package webapp

import (
	"context"
	"net/http"
	"time"

	"containerway/internal/diskutil"
)

// handleDisksSummary mantém compatibilidade com clientes antigos (tabela simples).
func (s *Server) handleDisksSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	raw, err := b.runDiskProbeScript(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	lsblkJ, dfB, dfMapB, lvsB, vgsB, pvsB := diskutil.SplitProbe(raw)
	rows := diskutil.BuildRows(lsblkJ, dfB, dfMapB, diskutil.SortLsblkOrder, "", false)
	devices := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		devices = append(devices, map[string]any{
			"name":  row.DisplayName,
			"path":  row.DevPath,
			"type":  row.TypeLabel,
			"size":  row.UsageLine,
			"mount": row.Mount,
			"depth": row.Depth,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"devices": devices,
		"dfRaw":   diskutil.BuildTechnicalText(dfB, lvsB, vgsB, pvsB),
	})
}
