package diskutil

import "strings"

// UsageRootsFromRows sugere atalhos de análise a partir dos pontos de montagem do lsblk.
func UsageRootsFromRows(rows []Row) []map[string]string {
	seen := map[string]struct{}{"/": {}}
	out := []map[string]string{{"label": "Raiz /", "path": "/"}}
	for _, r := range rows {
		m := strings.TrimSpace(r.Mount)
		if m == "" || strings.EqualFold(m, "[SWAP]") {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, map[string]string{"label": m, "path": m})
	}
	for _, d := range defaultUsageRootsFallback() {
		if _, ok := seen[d["path"]]; ok {
			continue
		}
		seen[d["path"]] = struct{}{}
		out = append(out, d)
	}
	return out
}

func defaultUsageRootsFallback() []map[string]string {
	return []map[string]string{
		{"label": "/home", "path": "/home"},
		{"label": "/var", "path": "/var"},
		{"label": "/opt", "path": "/opt"},
		{"label": "/usr", "path": "/usr"},
		{"label": "/tmp", "path": "/tmp"},
	}
}
