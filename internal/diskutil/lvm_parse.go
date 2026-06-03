package diskutil

import (
	"strconv"
	"strings"
)

// VGStats resumo de um volume group (saída vgs).
type VGStats struct {
	Name      string  `json:"name"`
	SizeGiB   float64 `json:"sizeGiB"`
	FreeGiB   float64 `json:"freeGiB"`
	SizeHuman string  `json:"sizeHuman"`
	FreeHuman string  `json:"freeHuman"`
}

// LVRecord entrada lvs para assistente e snapshots.
type LVRecord struct {
	Path       string  `json:"path"`
	VG         string  `json:"vg"`
	LVName     string  `json:"lvName"`
	SizeGiB    float64 `json:"sizeGiB"`
	SizeHuman  string  `json:"sizeHuman"`
	Attr       string  `json:"attr"`
	Origin     string  `json:"origin"`
	IsSnapshot bool    `json:"isSnapshot"`
}

// ParseVGSBlock interpreta vgs (separador ; ou colunas).
func ParseVGSBlock(vgsBlock string) []VGStats {
	var out []VGStats
	for _, line := range strings.Split(vgsBlock, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isLVMSkipLine(line) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "vg_name") {
			continue
		}
		var parts []string
		if strings.Contains(line, ";") {
			parts = strings.Split(line, ";")
		} else {
			parts = strings.Fields(line)
		}
		if len(parts) < 3 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		sizeG := parseFloatField(parts[1])
		freeG := parseFloatField(parts[2])
		out = append(out, VGStats{
			Name:      name,
			SizeGiB:   sizeG,
			FreeGiB:   freeG,
			SizeHuman: formatGiBHuman(sizeG),
			FreeHuman: formatGiBHuman(freeG),
		})
	}
	return out
}

// ParseLVSBlock interpreta lvs (separador ; ou colunas).
func ParseLVSBlock(lvsBlock string) []LVRecord {
	var out []LVRecord
	for _, line := range strings.Split(lvsBlock, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isLVMSkipLine(line) {
			continue
		}
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "lv path") || strings.HasPrefix(low, "path") || strings.HasPrefix(low, "lv_name") {
			continue
		}
		var parts []string
		if strings.Contains(line, ";") {
			parts = strings.Split(line, ";")
		} else {
			parts = strings.Fields(line)
		}
		if len(parts) < 2 {
			continue
		}
		path := strings.TrimSpace(parts[0])
		if !strings.HasPrefix(path, "/dev/") {
			continue
		}
		rec := LVRecord{
			Path: strings.TrimSpace(parts[0]),
			VG:   strings.TrimSpace(parts[1]),
		}
		if len(parts) >= 3 {
			rec.SizeGiB = parseFloatField(parts[2])
			rec.SizeHuman = formatGiBHuman(rec.SizeGiB)
		}
		if len(parts) >= 4 {
			rec.Attr = strings.TrimSpace(parts[3])
		}
		if len(parts) >= 5 {
			rec.Origin = strings.TrimSpace(parts[4])
		}
		if len(parts) >= 6 {
			rec.LVName = strings.TrimSpace(parts[5])
		}
		if rec.LVName == "" {
			rec.LVName = lvNameFromPath(rec.Path, rec.VG)
		}
		rec.IsSnapshot = rec.Origin != "" || (len(rec.Attr) > 0 && rec.Attr[0] == 's')
		out = append(out, rec)
	}
	return out
}

// SnapshotsForOrigin devolve snapshots cujo origin é o LV indicado.
func SnapshotsForOrigin(records []LVRecord, originPath string) []LVRecord {
	originPath = strings.TrimSpace(originPath)
	if originPath == "" {
		return nil
	}
	var out []LVRecord
	for _, r := range records {
		if !r.IsSnapshot {
			continue
		}
		if strings.TrimSpace(r.Origin) == originPath {
			out = append(out, r)
		}
	}
	return out
}

// VGStatsByName devolve estatísticas do VG ou nil.
func VGStatsByName(all []VGStats, name string) *VGStats {
	name = strings.TrimSpace(name)
	for i := range all {
		if all[i].Name == name {
			return &all[i]
		}
	}
	return nil
}

// ParseLVPathToVG mapeia caminho LV → nome do VG a partir da saída lvs.
func ParseLVPathToVG(lvsBlock string) map[string]string {
	m := make(map[string]string)
	for _, r := range ParseLVSBlock(lvsBlock) {
		if r.Path != "" && r.VG != "" {
			m[r.Path] = r.VG
		}
	}
	return m
}

func isLVMSkipLine(line string) bool {
	low := strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(low, "warning") || strings.HasPrefix(low, "file descriptor") || strings.HasPrefix(low, "can't open")
}

func parseFloatField(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "g"))
	s = strings.ReplaceAll(s, ",", ".")
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func formatGiBHuman(g float64) string {
	if g <= 0 {
		return "—"
	}
	return formatGiB(g) + " GiB"
}

func lvNameFromPath(path, vg string) string {
	path = strings.TrimSpace(path)
	vg = strings.TrimSpace(vg)
	if vg != "" {
		prefix := "/dev/" + vg + "/"
		if strings.HasPrefix(path, prefix) {
			return strings.TrimPrefix(path, prefix)
		}
	}
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return ""
}
