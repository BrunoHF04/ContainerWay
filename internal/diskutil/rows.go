package diskutil

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"containerway/internal/transfer"
)

type lsblkNode struct {
	Name          string          `json:"name"`
	Path          string          `json:"path"`
	Type          string          `json:"type"`
	Size          json.RawMessage `json:"size"`
	Fstype        string          `json:"fstype"`
	Mountpoint    string          `json:"mountpoint"`
	Mountpoints   json.RawMessage `json:"mountpoints"`
	Model         string          `json:"model"`
	Children      []lsblkNode     `json:"children"`
}

type lsblkJSONRoot struct {
	Blockdevices []lsblkNode `json:"blockdevices"`
}

// Row é uma linha da tabela «Armazenamento no host».
type Row struct {
	Depth       int     `json:"depth"`
	DisplayName string  `json:"displayName"`
	DevPath     string  `json:"devPath"`
	TypeLabel   string  `json:"typeLabel"`
	Mount       string  `json:"mount"`
	SizeBytes   uint64  `json:"sizeBytes"`
	BlockBytes  uint64  `json:"blockBytes"`
	UsedBytes   uint64  `json:"usedBytes"`
	AvailBytes  uint64  `json:"availBytes"`
	TotalDF     uint64  `json:"totalDf"`
	UsePct      float64 `json:"usePct"`
	HasDF       bool    `json:"hasDf"`
	Fstype      string  `json:"fstype"`
	LsblkType   string  `json:"lsblkType"`
	IsLoop      bool    `json:"isLoop"`
	UsageLine   string  `json:"usageLine"`
}

// SortMode define a ordenação da tabela.
type SortMode string

const (
	SortLsblkOrder SortMode = "lsblk"
	SortBlockDesc  SortMode = "block"
	SortUsePctDesc SortMode = "use"
)

type dfEntry struct {
	Device string
	Fstype string
	Total  uint64
	Used   uint64
	Avail  uint64
	UsePct float64
	Mount  string
}

func parseJSONUint64(raw json.RawMessage) uint64 {
	if len(raw) == 0 {
		return 0
	}
	var n uint64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		if f < 0 {
			return 0
		}
		return uint64(math.Round(f))
	}
	s := strings.Trim(string(raw), `"`)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func devicePathForNode(n lsblkNode, parentDevPath string) string {
	if strings.TrimSpace(n.Path) != "" {
		return strings.TrimSpace(n.Path)
	}
	name := strings.TrimSpace(n.Name)
	if name == "" {
		return parentDevPath
	}
	if strings.HasPrefix(name, "/dev/") {
		return name
	}
	return "/dev/" + name
}

func parseMountpointsJSON(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var strs []string
	if err := json.Unmarshal(raw, &strs); err == nil {
		return strs
	}
	var iface []interface{}
	if err := json.Unmarshal(raw, &iface); err != nil {
		return nil
	}
	out := make([]string, 0, len(iface))
	for _, v := range iface {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func effectiveMountpoint(n lsblkNode) string {
	if m := strings.TrimSpace(n.Mountpoint); m != "" {
		return m
	}
	for _, m := range parseMountpointsJSON(n.Mountpoints) {
		m = strings.TrimSpace(m)
		if m != "" && !strings.EqualFold(m, "null") {
			return m
		}
	}
	return ""
}

func diskTypeLabel(n lsblkNode) string {
	t := strings.ToLower(strings.TrimSpace(n.Type))
	fs := strings.TrimSpace(n.Fstype)
	switch t {
	case "rom":
		return "Unidade óptica / removível"
	case "disk":
		if strings.TrimSpace(n.Model) != "" {
			return fmt.Sprintf("Disco (%s)", strings.TrimSpace(n.Model))
		}
		return "Disco"
	case "loop":
		if fs != "" {
			return fmt.Sprintf("%s (loop)", fs)
		}
		return "loop"
	case "crypt":
		if fs != "" {
			return fmt.Sprintf("%s (criptografia)", fs)
		}
		return "Dispositivo criptografado"
	case "lvm":
		if fs != "" {
			return fmt.Sprintf("%s (sistema de arquivos)", fs)
		}
		return "LVM"
	case "part":
		if fs != "" {
			return fmt.Sprintf("%s (partição)", fs)
		}
		return "partição"
	default:
		if fs != "" {
			return fmt.Sprintf("%s (%s)", fs, t)
		}
		if t != "" {
			return t
		}
		return "—"
	}
}

func parseDFDevMap(block string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(strings.ReplaceAll(block, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dev, canon, ok := strings.Cut(line, "\t")
		if !ok {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				dev, canon = fields[0], fields[1]
			} else {
				continue
			}
		}
		dev, canon = strings.TrimSpace(dev), strings.TrimSpace(canon)
		if dev != "" && canon != "" {
			out[dev] = canon
		}
	}
	return out
}

func mergeDFCanonicalPaths(df map[string]dfEntry, devToCanon map[string]string) {
	for dev, canon := range devToCanon {
		if dev == "" || canon == "" {
			continue
		}
		if e, ok := df[dev]; ok {
			if _, ok2 := df[canon]; !ok2 {
				df[canon] = e
			}
		}
		if e, ok := df[canon]; ok {
			if _, ok2 := df[dev]; !ok2 {
				df[dev] = e
			}
		}
	}
}

func parseDFTable(dfBlock string) map[string]dfEntry {
	out := make(map[string]dfEntry)
	lines := strings.Split(strings.ReplaceAll(dfBlock, "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.EqualFold(fields[0], "filesystem") {
			continue
		}
		if len(fields) < 7 {
			continue
		}
		fsDev := fields[0]
		fstype := fields[1]
		total, _ := strconv.ParseUint(fields[2], 10, 64)
		used, _ := strconv.ParseUint(fields[3], 10, 64)
		avail, _ := strconv.ParseUint(fields[4], 10, 64)
		useStr := strings.TrimSuffix(fields[5], "%")
		usePct, _ := strconv.ParseFloat(useStr, 64)
		mount := strings.TrimSpace(strings.Join(fields[6:], " "))
		ent := dfEntry{Device: fsDev, Fstype: fstype, Total: total, Used: used, Avail: avail, UsePct: usePct, Mount: mount}
		out[fsDev] = ent
		if mount != "" {
			out[mount] = ent
		}
	}
	return out
}

func lookupDF(df map[string]dfEntry, devPath, mount string) (dfEntry, bool) {
	if mount != "" {
		m := strings.TrimSpace(mount)
		if e, ok := df[m]; ok {
			return e, true
		}
		if m != "/" && strings.HasSuffix(m, "/") {
			if e, ok := df[strings.TrimSuffix(m, "/")]; ok {
				return e, true
			}
		}
	}
	d := strings.TrimSpace(devPath)
	if e, ok := df[d]; ok {
		return e, true
	}
	for k, e := range df {
		if strings.HasPrefix(k, "/dev/") && (k == d || strings.HasSuffix(d, k) || strings.HasSuffix(k, d)) {
			return e, true
		}
	}
	return dfEntry{}, false
}

func flattenLsblk(nodes []lsblkNode, parentModel string, depth int, showLoop bool, dfs *int, out *[]Row) {
	for _, n := range nodes {
		t := strings.ToLower(strings.TrimSpace(n.Type))
		if !showLoop && t == "loop" {
			continue
		}
		dev := devicePathForNode(n, "")
		model := strings.TrimSpace(n.Model)
		if model == "" {
			model = parentModel
		}
		name := strings.TrimSpace(n.Name)
		display := name
		if model != "" && (t == "disk" || t == "nvme" || name != "") {
			if t == "disk" || strings.HasPrefix(name, "nvme") || strings.HasPrefix(name, "sd") {
				display = name + " — " + model
			}
		}
		sz := parseJSONUint64(n.Size)
		mount := effectiveMountpoint(n)
		row := Row{
			Depth:       depth,
			DisplayName: display,
			DevPath:     dev,
			TypeLabel:   diskTypeLabel(n),
			Mount:       mount,
			SizeBytes:   sz,
			BlockBytes:  sz,
			Fstype:      strings.TrimSpace(n.Fstype),
			LsblkType:   strings.TrimSpace(n.Type),
			IsLoop:      t == "loop",
		}
		*dfs++
		*out = append(*out, row)
		if len(n.Children) > 0 {
			flattenLsblk(n.Children, model, depth+1, showLoop, dfs, out)
		}
	}
}

// BuildRows constrói linhas da tabela a partir da sondagem.
func BuildRows(lsblkJSON, dfBlock, dfDevMapBlock string, sortMode SortMode, filter string, showLoop bool) []Row {
	var root lsblkJSONRoot
	_ = json.Unmarshal([]byte(strings.TrimSpace(lsblkJSON)), &root)
	df := parseDFTable(dfBlock)
	mergeDFCanonicalPaths(df, parseDFDevMap(dfDevMapBlock))
	dfs := 0
	var raw []Row
	flattenLsblk(root.Blockdevices, "", 0, showLoop, &dfs, &raw)
	for i := range raw {
		if ent, ok := lookupDF(df, raw[i].DevPath, raw[i].Mount); ok {
			raw[i].HasDF = true
			raw[i].UsedBytes = ent.Used
			raw[i].AvailBytes = ent.Avail
			raw[i].TotalDF = ent.Total
			raw[i].UsePct = ent.UsePct
			if raw[i].Fstype == "" {
				raw[i].Fstype = ent.Fstype
			}
		}
		raw[i].UsageLine = formatUsageLine(raw[i])
	}
	q := strings.ToLower(strings.TrimSpace(filter))
	if q != "" {
		filtered := raw[:0]
		for _, r := range raw {
			blob := strings.ToLower(r.DisplayName + " " + r.TypeLabel + " " + r.Mount + " " + r.DevPath)
			if strings.Contains(blob, q) {
				filtered = append(filtered, r)
			}
		}
		raw = filtered
	}
	switch sortMode {
	case SortBlockDesc:
		sort.SliceStable(raw, func(i, j int) bool { return raw[i].BlockBytes > raw[j].BlockBytes })
	case SortUsePctDesc:
		sort.SliceStable(raw, func(i, j int) bool {
			if raw[i].HasDF != raw[j].HasDF {
				return raw[i].HasDF && !raw[j].HasDF
			}
			return raw[i].UsePct > raw[j].UsePct
		})
	}
	return raw
}

func formatUsageLine(r Row) string {
	if !r.HasDF || r.TotalDF == 0 {
		if r.BlockBytes > 0 {
			s := transfer.FormatBytes(int64(r.BlockBytes))
			fs := strings.ToLower(strings.TrimSpace(r.Fstype))
			if strings.Contains(fs, "lvm") || strings.Contains(fs, "lvm2") {
				return s + " — PV LVM (ver LV)"
			}
			return s
		}
		return "—"
	}
	u := transfer.FormatBytes(int64(r.UsedBytes))
	t := transfer.FormatBytes(int64(r.TotalDF))
	b := transfer.FormatBytes(int64(r.BlockBytes))
	return fmt.Sprintf("%s / %s · bloco %s", u, t, b)
}

// LVOption é uma opção no assistente LVM.
type LVOption struct {
	Path  string `json:"path"`
	Label string `json:"label"`
}

const manualPathOption = "Caminho manual (edite o campo abaixo)"

// AssistantOptions devolve LVs detectados para o assistente.
func AssistantOptions(rows []Row) []LVOption {
	opts := []LVOption{{Path: "", Label: manualPathOption}}
	seen := map[string]struct{}{}
	for _, r := range rows {
		if r.DevPath == "" {
			continue
		}
		lt := strings.ToLower(r.LsblkType)
		if lt != "lvm" && !strings.Contains(r.DevPath, "/mapper/") {
			continue
		}
		if _, ok := seen[r.DevPath]; ok {
			continue
		}
		seen[r.DevPath] = struct{}{}
		label := r.DevPath
		if r.Mount != "" {
			label = fmt.Sprintf("%s — %s", r.DevPath, r.Mount)
		}
		opts = append(opts, LVOption{Path: r.DevPath, Label: label})
	}
	return opts
}

// ParseLVPathToVG mapeia caminho LV → nome do VG a partir da saída lvs.
func ParseLVPathToVG(lvsBlock string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(lvsBlock, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "warning") || strings.HasPrefix(strings.ToLower(line), "file descriptor") {
			continue
		}
		if strings.HasPrefix(line, "LV Path") || strings.HasPrefix(line, "Path") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		lvPath := strings.TrimSpace(fields[0])
		vg := strings.TrimSpace(fields[1])
		if strings.HasPrefix(lvPath, "/dev/") {
			m[lvPath] = vg
		}
	}
	return m
}

// FindRowByDev devolve a linha pelo caminho do dispositivo.
func FindRowByDev(rows []Row, dev string) (Row, bool) {
	d := strings.TrimSpace(dev)
	for _, r := range rows {
		if r.DevPath == d {
			return r, true
		}
	}
	return Row{}, false
}

// VGForLV devolve o VG associado a um LV, se existir.
func VGForLV(lvsBlock, dev string) string {
	m := ParseLVPathToVG(lvsBlock)
	if vg, ok := m[strings.TrimSpace(dev)]; ok && vg != "" {
		return vg
	}
	return ""
}
