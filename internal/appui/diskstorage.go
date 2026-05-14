package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	fynecontainer "fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"containerway/internal/transfer"
)

const diskProbeScript = `set +e
printf '%s\n' '===LSBLK_JSON==='
lsblk -J -b 2>/dev/null || printf '%s\n' '{"blockdevices":[]}'
printf '\n%s\n' '===DF==='
df -B1 -T 2>/dev/null || true
printf '\n%s\n' '===DF_DEVMAP==='
df -B1 -T 2>/dev/null | awk 'NR>1 && $1 ~ /^\/dev\// {print $1}' | sort -u | while IFS= read -r dev; do
  [ -z "$dev" ] && continue
  c=$(readlink -f "$dev" 2>/dev/null || printf '%s\n' "$dev")
  printf '%s\t%s\n' "$dev" "$c"
done
printf '\n%s\n' '===LVS==='
lvs -a -o lv_path,vg_name,lv_size 2>&1 || true
printf '\n%s\n' '===VGS==='
vgs -a -o vg_name,vg_free,vg_size 2>&1 || true
printf '\n%s\n' '===PVS==='
pvs 2>&1 || true
printf '\n%s\n' '===FIM==='
`

type lsblkNode struct {
	Name          string          `json:"name"`
	Path          string          `json:"path"`
	Type          string          `json:"type"`
	Size          json.RawMessage `json:"size"`
	Fstype        string          `json:"fstype"`
	Mountpoint  string          `json:"mountpoint"`
	Mountpoints json.RawMessage `json:"mountpoints"`
	Model         string          `json:"model"`
	Rm            json.RawMessage `json:"rm"`
	Children      []lsblkNode     `json:"children"`
}

type lsblkJSONRoot struct {
	Blockdevices []lsblkNode `json:"blockdevices"`
}

type diskTableRow struct {
	Depth       int
	DFSOrder    int
	DisplayName string
	DevPath     string
	TypeLabel   string
	Mount       string
	SizeBytes   uint64
	BlockBytes  uint64
	UsedBytes   uint64
	AvailBytes  uint64
	TotalDF     uint64
	UsePct      float64
	HasDF       bool
	Fstype      string
	LsblkType   string
	IsLoop      bool
}

type diskSortMode int

const (
	diskSortLsblkOrder diskSortMode = iota
	diskSortBlockDesc
	diskSortUsePctDesc
)

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
		return "Unidade ótica / removível"
	case "disk":
		if strings.TrimSpace(n.Model) != "" {
			return "Disco (" + strings.TrimSpace(n.Model) + ")"
		}
		return "Disco"
	case "loop":
		if fs != "" {
			return fs + " (loop)"
		}
		return "loop"
	case "crypt":
		if fs != "" {
			return fs + " (criptografia)"
		}
		return "Dispositivo cifrado"
	case "lvm":
		if fs != "" {
			return fs + " filesystem"
		}
		return "LVM"
	case "part":
		if fs != "" {
			return fs + " (partição)"
		}
		return "partição"
	default:
		if fs != "" {
			return fs + " (" + t + ")"
		}
		if t != "" {
			return t
		}
		return "—"
	}
}

type dfEntry struct {
	Device    string
	Fstype    string
	Total     uint64
	Used      uint64
	Avail     uint64
	UsePct    float64
	Mount     string
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

// mergeDFCanonicalPaths replica entradas de df entre caminho de df e o destino de readlink -f
// (ex.: /dev/mapper/vg-lv e /dev/dm-0), para o emparelhamento com lsblk.
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
		// Filesystem Type 1B-blocks Used Available Use% Mounted
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
		// df pode reportar o mount com barra final em casos raros
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
	// Caminhos equivalentes (symlinks /dev)
	for k, e := range df {
		if strings.HasPrefix(k, "/dev/") && (k == d || strings.HasSuffix(d, k) || strings.HasSuffix(k, d)) {
			return e, true
		}
	}
	return dfEntry{}, false
}

func parseLVPathToVG(lvsBlock string) map[string]string {
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

func flattenLsblk(nodes []lsblkNode, parentModel string, depth int, showLoop bool, dfs *int, out *[]diskTableRow) {
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
		row := diskTableRow{
			Depth:       depth,
			DFSOrder:    *dfs,
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

func buildDiskRowsFromProbe(lsblkJSON string, dfBlock string, dfDevMapBlock string, sortMode diskSortMode, filter string, showLoop bool) []diskTableRow {
	var root lsblkJSONRoot
	_ = json.Unmarshal([]byte(strings.TrimSpace(lsblkJSON)), &root)
	df := parseDFTable(dfBlock)
	mergeDFCanonicalPaths(df, parseDFDevMap(dfDevMapBlock))
	dfs := 0
	var raw []diskTableRow
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
	case diskSortLsblkOrder:
		return raw
	case diskSortBlockDesc:
		sort.SliceStable(raw, func(i, j int) bool { return raw[i].BlockBytes > raw[j].BlockBytes })
		return raw
	case diskSortUsePctDesc:
		sort.SliceStable(raw, func(i, j int) bool {
			pi, pj := raw[i].UsePct, raw[j].UsePct
			if raw[i].HasDF != raw[j].HasDF {
				return raw[i].HasDF && !raw[j].HasDF
			}
			return pi > pj
		})
		return raw
	default:
		return raw
	}
}

func formatDiskUsageLine(r diskTableRow) string {
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
	// Uma linha ao lado da barra; a percentagem fica só no widget ProgressBar.
	return fmt.Sprintf("%s / %s · bloco %s", u, t, b)
}

const (
	diskListRowH     float32 = 60
	diskListColDev   float32 = 260
	diskListColType  float32 = 220
	diskListColMount float32 = 120
	diskListBarW     float32 = 112
	diskListBarH     float32 = 28
)

func newDiskHostListRow() fyne.CanvasObject {
	dev := widget.NewLabel("")
	dev.Wrapping = fyne.TextWrapOff
	dev.Truncation = fyne.TextTruncateEllipsis
	devWrap := fynecontainer.NewGridWrap(fyne.NewSize(diskListColDev, diskListRowH), dev)

	typ := widget.NewLabel("")
	typ.Wrapping = fyne.TextWrapOff
	typ.Truncation = fyne.TextTruncateEllipsis
	typWrap := fynecontainer.NewGridWrap(fyne.NewSize(diskListColType, diskListRowH), typ)

	mnt := widget.NewLabel("")
	mnt.Wrapping = fyne.TextWrapOff
	mnt.Truncation = fyne.TextTruncateEllipsis
	mntWrap := fynecontainer.NewGridWrap(fyne.NewSize(diskListColMount, diskListRowH), mnt)

	bar := widget.NewProgressBar()
	barBox := fynecontainer.NewGridWrap(fyne.NewSize(diskListBarW, diskListBarH), bar)
	useLbl := widget.NewLabel("")
	useLbl.Wrapping = fyne.TextWrapWord
	useLbl.Truncation = fyne.TextTruncateOff
	useLbl.Alignment = fyne.TextAlignLeading
	// HBox nunca estica filhos (só layout.Spacer recebe o «extra»). Border dá ao centro
	// toda a largura restante; barra fixa à esquerda, texto preenche o espaço à direita.
	usageCell := fynecontainer.NewBorder(nil, nil, barBox, nil, useLbl)

	colsLeft := fynecontainer.NewHBox(devWrap, typWrap, mntWrap)
	inner := fynecontainer.NewBorder(nil, nil, colsLeft, nil, usageCell)
	return fynecontainer.NewMax(inner)
}

func updateDiskHostListRow(o fyne.CanvasObject, r *diskTableRow, empty bool) {
	outer, ok := o.(*fyne.Container)
	if !ok || len(outer.Objects) < 1 {
		return
	}
	row, ok := outer.Objects[0].(*fyne.Container)
	if !ok || len(row.Objects) < 2 {
		return
	}
	// NewBorder(nil,nil,left,nil,center) → Objects [center, left]
	usageBorder := row.Objects[0].(*fyne.Container)
	leftHBox := row.Objects[1].(*fyne.Container)
	if len(leftHBox.Objects) < 3 || len(usageBorder.Objects) < 2 {
		return
	}
	dev := leftHBox.Objects[0].(*fyne.Container).Objects[0].(*widget.Label)
	typ := leftHBox.Objects[1].(*fyne.Container).Objects[0].(*widget.Label)
	mnt := leftHBox.Objects[2].(*fyne.Container).Objects[0].(*widget.Label)
	// NewBorder(nil,nil,barBox,nil,useLbl) → Objects [useLbl, barBox]
	useLbl := usageBorder.Objects[0].(*widget.Label)
	barBox := usageBorder.Objects[1].(*fyne.Container)
	bar := barBox.Objects[0].(*widget.ProgressBar)

	if empty || r == nil {
		dev.SetText("Sem dados — use «Atualizar».")
		typ.SetText("")
		mnt.SetText("")
		barBox.Hide()
		bar.Hide()
		bar.SetValue(0)
		useLbl.SetText("")
		return
	}

	prefix := strings.Repeat("  ", r.Depth)
	dev.SetText(prefix + r.DisplayName)
	typ.SetText(r.TypeLabel)
	mnt.SetText(r.Mount)

	line := formatDiskUsageLine(*r)
	if r.HasDF && r.TotalDF > 0 && !math.IsNaN(r.UsePct) {
		barBox.Show()
		bar.Show()
		v := r.UsePct / 100.0
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		bar.SetValue(v)
		useLbl.SetText(line)
		return
	}
	barBox.Hide()
	bar.Hide()
	bar.SetValue(0)
	useLbl.SetText(line)
}

func (ui *explorer) runDiskProbeScript(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	script := diskProbeScript
	if ui.sudoEnabled && strings.TrimSpace(ui.sudoUser) != "" && strings.TrimSpace(ui.sudoPass) != "" {
		if err := ui.ensureSudoSession(ctx); err != nil {
			return "", err
		}
		cmd := fmt.Sprintf("sudo -S -p '' -u %s sh -lc %s", shellQuote(ui.sudoUser), shellQuote(script))
		stdout, _, err := ui.runSSHCommandWithInput(ctx, cmd, ui.sudoPass)
		return stdout, err
	}
	stdout, _, err := ui.runSSHCommandWithInput(ctx, "sh -lc "+shellQuote(script), "")
	return stdout, err
}

func splitDiskProbe(raw string) (lsblkJSON, dfBlock, dfDevMapBlock, lvsBlock, vgsBlock, pvsBlock string) {
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	between := func(startTag, endTag string) string {
		i := strings.Index(s, startTag)
		if i < 0 {
			return ""
		}
		rest := s[i+len(startTag):]
		rest = strings.TrimLeft(rest, "\n")
		if endTag == "" {
			return strings.TrimSpace(rest)
		}
		j := strings.Index(rest, endTag)
		if j < 0 {
			return strings.TrimSpace(rest)
		}
		return strings.TrimSpace(rest[:j])
	}
	lsblkJSON = between("===LSBLK_JSON===", "===DF===")
	if strings.Contains(s, "===DF_DEVMAP===") {
		dfBlock = between("===DF===", "===DF_DEVMAP===")
		dfDevMapBlock = between("===DF_DEVMAP===", "===LVS===")
	} else {
		// Compatível com probes antigos sem mapa readlink
		dfBlock = between("===DF===", "===LVS===")
		dfDevMapBlock = ""
	}
	lvsBlock = between("===LVS===", "===VGS===")
	vgsBlock = between("===VGS===", "===PVS===")
	pvsBlock = between("===PVS===", "===FIM===")
	return lsblkJSON, dfBlock, dfDevMapBlock, lvsBlock, vgsBlock, pvsBlock
}

func buildTechnicalDiskText(dfBlock, lvsBlock, vgsBlock, pvsBlock string) string {
	var b strings.Builder
	b.WriteString("===DF===\n")
	b.WriteString(strings.TrimSpace(dfBlock))
	b.WriteString("\n\n===LVS===\n")
	b.WriteString(strings.TrimSpace(lvsBlock))
	b.WriteString("\n\n===VGS===\n")
	b.WriteString(strings.TrimSpace(vgsBlock))
	if strings.TrimSpace(pvsBlock) != "" {
		b.WriteString("\n\n===PVS===\n")
		b.WriteString(strings.TrimSpace(pvsBlock))
	}
	b.WriteString("\n\n===FIM===\n")
	return b.String()
}

func diskAssistantOptions(rows []diskTableRow) []string {
	opts := []string{"Caminho manual (editar o campo abaixo)"}
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
		opts = append(opts, label)
	}
	return opts
}

func findDiskRowByDev(rows []diskTableRow, dev string) (diskTableRow, bool) {
	d := strings.TrimSpace(dev)
	for _, r := range rows {
		if r.DevPath == d {
			return r, true
		}
	}
	return diskTableRow{}, false
}

func parseAssistantSelection(sel string) string {
	s := strings.TrimSpace(sel)
	if s == "" || strings.HasPrefix(s, "Caminho manual") {
		return ""
	}
	if idx := strings.Index(s, " — "); idx > 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

// showDiskStorageManager abre o módulo «Discos e armazenamento» (lsblk, df/LVM, assistente).
func (ui *explorer) showDiskStorageManager() {
	appendAuditLog("disco", "Módulo Discos e armazenamento aberto")

	stopAuto := make(chan struct{})
	var stopOnce sync.Once
	autoOn := atomic.Bool{}

	sudoStatus := widget.NewLabel("")
	sudoStatus.Wrapping = fyne.TextWrapWord

	btnSudoOn := widget.NewButtonWithIcon("Ativar superusuário (sudo)", theme.LoginIcon(), func() {
		ui.showSudoCredentialsDialog("Superusuário (sudo)")
	})
	btnSudoOn.Importance = widget.HighImportance
	btnSudoOff := widget.NewButtonWithIcon("Desactivar sudo", theme.CancelIcon(), func() {
		ui.disableSudoMode()
	})
	btnSudoOff.Hide()

	updateSudoBanner := func() {
		if ui.sudoEnabled && strings.TrimSpace(ui.sudoUser) != "" {
			sudoStatus.SetText(fmt.Sprintf("Superusuário (sudo): activo (%s) — pode usar «Ampliar LV (LVM)» no servidor.", ui.sudoUser))
			btnSudoOn.Disable()
			btnSudoOff.Show()
			return
		}
		sudoStatus.SetText("Superusuário (sudo): inactivo — necessário para «Ampliar LV (LVM)» no servidor.")
		btnSudoOn.Enable()
		btnSudoOff.Hide()
	}

	hint := widget.NewLabel("Resumo a partir de lsblk (árvore). Por defeito ocultamos dispositivos loop usados pelo Snap. Use as abas para alternar entre o assistente LVM, a tabela de armazenamento e a saída técnica (df/LVM).")
	hint.Wrapping = fyne.TextWrapWord

	updatedAt := widget.NewLabel("Dados ainda não actualizados.")
	updatedAt.Alignment = fyne.TextAlignTrailing
	updatedAt.Wrapping = fyne.TextWrapOff

	btnRefresh := widget.NewButtonWithIcon("Atualizar", theme.ViewRefreshIcon(), nil)
	btnRefresh.Importance = widget.MediumImportance

	autoCheck := widget.NewCheck("Actualização automática a cada 90 s (só com a aba «Armazenamento no host» visível)", func(on bool) {
		autoOn.Store(on)
	})

	// --- Tab: Armazenamento no host
	showLoop := atomic.Bool{}
	loopCheck := widget.NewCheck("Mostrar dispositivos loop (Snap)", func(on bool) { showLoop.Store(on) })
	filterEntry := widget.NewEntry()
	filterEntry.SetPlaceHolder("Filtrar tabela (nome, tipo, montagem, caminho)…")
	sortSelect := widget.NewSelect([]string{
		"Ordem lsblk (árvore)",
		"Tamanho do bloco (maior primeiro)",
		"Uso % (maior primeiro)",
	}, func(string) {})
	sortSelect.Selected = "Ordem lsblk (árvore)"

	var tableRows []diskTableRow
	var tableRowsMu sync.Mutex
	hostList := widget.NewList(
		func() int {
			tableRowsMu.Lock()
			n := len(tableRows)
			tableRowsMu.Unlock()
			if n == 0 {
				return 1
			}
			return n
		},
		newDiskHostListRow,
		func(id widget.ListItemID, o fyne.CanvasObject) {
			tableRowsMu.Lock()
			defer tableRowsMu.Unlock()
			if len(tableRows) == 0 {
				updateDiskHostListRow(o, nil, true)
				return
			}
			if int(id) < 0 || int(id) >= len(tableRows) {
				return
			}
			updateDiskHostListRow(o, &tableRows[id], false)
		},
	)

	hostHead := fynecontainer.NewBorder(
		nil,
		nil,
		widget.NewLabelWithStyle("Armazenamento no host (resumo)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		loopCheck,
		nil,
	)
	hostTop := fynecontainer.NewVBox(
		hostHead,
		filterEntry,
		fynecontainer.NewHBox(widget.NewLabel("Ordenação:"), layout.NewSpacer(), sortWrap(sortSelect)),
	)
	hdrBold := fyne.TextStyle{Bold: true}
	hdr0 := fynecontainer.NewGridWrap(fyne.NewSize(260, 26), widget.NewLabelWithStyle("Dispositivo", fyne.TextAlignLeading, hdrBold))
	hdr1 := fynecontainer.NewGridWrap(fyne.NewSize(220, 26), widget.NewLabelWithStyle("Tipo", fyne.TextAlignLeading, hdrBold))
	hdr2 := fynecontainer.NewGridWrap(fyne.NewSize(120, 26), widget.NewLabelWithStyle("Localização", fyne.TextAlignLeading, hdrBold))
	hdrLeft := fynecontainer.NewHBox(hdr0, hdr1, hdr2)
	hdr3 := widget.NewLabelWithStyle("Tamanho / uso", fyne.TextAlignLeading, hdrBold)
	headerRow := fynecontainer.NewBorder(nil, nil, hdrLeft, nil, hdr3)
	outerHeader := fynecontainer.NewMax(headerRow)
	hostPane := fynecontainer.NewBorder(
		fynecontainer.NewVBox(
			fynecontainer.NewPadded(hostTop),
			widget.NewSeparator(),
			outerHeader,
		),
		nil,
		nil,
		nil,
		fynecontainer.NewScroll(hostList),
	)

	// --- Tab: Detalhe técnico
	techEntry := widget.NewMultiLineEntry()
	techEntry.TextStyle = fyne.TextStyle{Monospace: true}
	techEntry.Wrapping = fyne.TextWrapOff
	techScroll := fynecontainer.NewScroll(techEntry)
	btnJumpVGS := widget.NewButton("Saltar para VGS", func() {
		txt := techEntry.Text
		idx := strings.Index(txt, "===VGS===")
		if idx < 0 {
			dialog.ShowInformation("Detalhe técnico", "Secção VGS não encontrada na saída actual.", ui.win)
			return
		}
		rest := txt[idx:]
		end := strings.Index(rest, "\n===PVS===")
		if end < 0 {
			end = strings.Index(rest, "\n\n===FIM===")
		}
		if end > 0 {
			rest = rest[:end]
		}
		d := dialog.NewCustom("VGS (extracção)", "Fechar", fynecontainer.NewScroll(widget.NewLabel(rest)), ui.win)
		d.Resize(fyne.NewSize(720, 420))
		d.Show()
	})
	techBar := fynecontainer.NewBorder(nil, nil, widget.NewLabelWithStyle("Saída bruta (df, LVM, avisos)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), btnJumpVGS, nil)
	techPane := fynecontainer.NewBorder(
		fynecontainer.NewPadded(techBar),
		nil,
		nil,
		nil,
		techScroll,
	)

	// --- Tab: Assistente
	lvSelect := widget.NewSelect([]string{"Caminho manual (editar o campo abaixo)"}, nil)
	lvPathEntry := widget.NewEntry()
	lvPathEntry.SetPlaceHolder("/dev/mapper/... ou /dev/grupo/nome-do-lv")
	addGiBEntry := widget.NewEntry()
	addGiBEntry.SetPlaceHolder("ex.: 5 (GiB a adicionar ao LV)")
	fsSelect := widget.NewSelect([]string{"ext4", "ext3", "ext2", "xfs", "btrfs"}, nil)
	fsSelect.Selected = "ext4"

	detailMount := widget.NewLabel("Montagem: —")
	detailType := widget.NewLabel("Tipo: —")
	detailFS := widget.NewLabel("Sistema de ficheiros: —")
	detailUse := widget.NewLabel("Tamanho / uso: —")
	detailUse.Wrapping = fyne.TextWrapWord
	usageBar := widget.NewProgressBar()
	usageBar.Hide()
	vgHint := widget.NewLabel("")
	vgHint.Wrapping = fyne.TextWrapWord

	var lastLVS string
	var lastRows []diskTableRow
	var probeMu sync.Mutex

	refreshAssistant := func() {
		probeMu.Lock()
		lvs := lastLVS
		rows := lastRows
		probeMu.Unlock()

		opts := diskAssistantOptions(rows)
		cur := lvSelect.Selected
		lvSelect.Options = opts
		if cur != "" {
			found := false
			for _, o := range opts {
				if o == cur {
					found = true
					break
				}
			}
			if found {
				lvSelect.Selected = cur
			} else {
				lvSelect.ClearSelected()
				lvSelect.Selected = opts[0]
			}
		} else {
			lvSelect.Selected = opts[0]
		}
		lvSelect.Refresh()

		dev := strings.TrimSpace(lvPathEntry.Text)
		if sel := lvSelect.Selected; sel != "" && !strings.HasPrefix(sel, "Caminho manual") {
			if p := parseAssistantSelection(sel); p != "" {
				dev = p
				lvPathEntry.SetText(p)
			}
		}
		if dev == "" {
			detailMount.SetText("Montagem: —")
			detailType.SetText("Tipo: —")
			detailFS.SetText("Sistema de ficheiros: —")
			detailUse.SetText("Tamanho / uso: —")
			usageBar.Hide()
			vgHint.SetText("")
			return
		}
		r, ok := findDiskRowByDev(rows, dev)
		if !ok {
			detailMount.SetText("Montagem: (desconhecida — actualize ou confira o caminho)")
			detailType.SetText("Tipo: —")
			detailFS.SetText("Sistema de ficheiros: —")
			detailUse.SetText("Tamanho / uso: —")
			usageBar.Hide()
		} else {
			detailMount.SetText("Montagem: " + emptyDash(r.Mount))
			detailType.SetText("Tipo: " + emptyDash(r.TypeLabel))
			detailFS.SetText("Sistema de ficheiros: " + emptyDash(r.Fstype))
			detailUse.SetText("Tamanho / uso: " + formatDiskUsageLine(r))
			if r.HasDF && r.TotalDF > 0 {
				usageBar.Show()
				usageBar.SetValue(r.UsePct / 100.0)
			} else {
				usageBar.Hide()
			}
		}
		vgMap := parseLVPathToVG(lvs)
		if vg, ok2 := vgMap[dev]; ok2 && vg != "" {
			vgHint.SetText("Volume group (lvs): " + vg)
		} else {
			vgHint.SetText("Associação ao VG (lvs) não foi detetada para este caminho.")
		}
	}

	lvSelect.OnChanged = func(string) { refreshAssistant() }

	btnCopyPath := widget.NewButtonWithIcon("Copiar caminho", theme.ContentCopyIcon(), func() {
		p := strings.TrimSpace(lvPathEntry.Text)
		if p == "" {
			return
		}
		if c := fyne.CurrentApp().Clipboard(); c != nil {
			c.SetContent(p)
			ui.status.SetText("Caminho copiado para a área de transferência.")
		}
	})
	btnOpenTech := widget.NewButton("Abrir detalhe técnico (secção VGS)", func() {})

	addHint := widget.NewLabel("Valor em gibibytes acrescentados ao LV ('lvextend -L +NG'). É preciso espaço livre no VG; sem espaço o comando falha (ver mensagens no detalhe técnico).")
	addHint.Wrapping = fyne.TextWrapWord

	btnExpand := widget.NewButtonWithIcon("Ampliar LV (LVM)", theme.DocumentCreateIcon(), nil)
	btnExpand.Importance = widget.HighImportance
	btnShrinkInfo := widget.NewButton("Sobre redução de LV", func() {
		dialog.ShowInformation("Sobre redução de LV",
			"Reduzir um volume lógico é arriscado: é necessário encolher primeiro o sistema de ficheiros (nem sempre suportado), fazer cópia de segurança e planear janela de manutenção. Este assistente só suporta ampliação.",
			ui.win)
	})

	assistantForm := fynecontainer.NewVBox(
		widget.NewLabelWithStyle("Assistente: ampliar volume lógico", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Volume lógico (lista a partir do lsblk da última actualização):"),
		lvSelect,
		widget.NewSeparator(),
		detailMount,
		detailType,
		detailFS,
		usageBar,
		detailUse,
		vgHint,
		widget.NewLabel("Para a listagem completa de PV/VG/LV, use o separador «Detalhe técnico»."),
		fynecontainer.NewHBox(btnCopyPath, btnOpenTech),
		widget.NewSeparator(),
		widget.NewForm(
			widget.NewFormItem("Caminho do LV (editável)", lvPathEntry),
			widget.NewFormItem("Acrescentar (GiB)", addGiBEntry),
			widget.NewFormItem("Sistema de ficheiros", fsSelect),
		),
		addHint,
		fynecontainer.NewHBox(btnExpand, btnShrinkInfo),
	)
	assistantScroll := fynecontainer.NewScroll(assistantForm)

	assistantTab := fynecontainer.NewTabItem("Assistente", assistantScroll)
	hostTab := fynecontainer.NewTabItem("Armazenamento no host", hostPane)
	techTab := fynecontainer.NewTabItem("Detalhe técnico", techPane)

	tabs := fynecontainer.NewAppTabs(assistantTab, hostTab, techTab)
	tabs.SetTabLocation(fynecontainer.TabLocationTop)

	btnOpenTech.OnTapped = func() {
		tabs.Select(techTab)
	}

	doRefresh := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		raw, err := ui.runDiskProbeScript(ctx)
		if err != nil {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("falha ao consultar o servidor: %w", err), ui.win)
			})
			return
		}
		lsblkJ, dfB, dfMapB, lvsB, vgsB, pvsB := splitDiskProbe(raw)
		tech := buildTechnicalDiskText(dfB, lvsB, vgsB, pvsB)

		mode := diskSortLsblkOrder
		switch sortSelect.Selected {
		case "Tamanho do bloco (maior primeiro)":
			mode = diskSortBlockDesc
		case "Uso % (maior primeiro)":
			mode = diskSortUsePctDesc
		}
		rows := buildDiskRowsFromProbe(lsblkJ, dfB, dfMapB, mode, filterEntry.Text, showLoop.Load())

		probeMu.Lock()
		lastLVS = lvsB
		lastRows = rows
		probeMu.Unlock()

		tableRowsMu.Lock()
		tableRows = rows
		tableRowsMu.Unlock()

		ts := time.Now().Format("15:04:05")
		fyne.Do(func() {
			techEntry.SetText(tech)
			hostList.Refresh()
			updatedAt.SetText("Dados actualizados em " + ts + ".")
			updateSudoBanner()
			refreshAssistant()
		})
	}

	btnRefresh.OnTapped = func() { go doRefresh() }
	filterEntry.OnChanged = func(string) { go doRefresh() }
	sortSelect.OnChanged = func(string) { go doRefresh() }
	loopCheck.OnChanged = func(bool) { go doRefresh() }

	lvPathEntry.OnChanged = func(string) { refreshAssistant() }

	btnExpand.OnTapped = func() {
		lv := strings.TrimSpace(lvPathEntry.Text)
		if lv == "" {
			dialog.ShowInformation("Ampliar LV", "Indique o caminho do volume lógico.", ui.win)
			return
		}
		if !ui.sudoEnabled {
			dialog.ShowInformation("Ampliar LV", "Active o superusuário (sudo) para executar lvextend no servidor.", ui.win)
			return
		}
		gStr := strings.TrimSpace(strings.ReplaceAll(addGiBEntry.Text, ",", "."))
		gib, err := strconv.ParseFloat(gStr, 64)
		if err != nil || gib <= 0 {
			dialog.ShowInformation("Ampliar LV", "Indique um valor positivo em GiB (ex.: 5 ou 2.5).", ui.win)
			return
		}
		fs := strings.TrimSpace(fsSelect.Selected)
		if fs == "" {
			fs = "ext4"
		}
		confirm := fmt.Sprintf(
			"Confirma a ampliação do LV?\n\n"+
				"LV: %s\n"+
				"Acrescentar: +%.4g GiB (lvextend -L +%.4gG)\n"+
				"Ficheiros: %s\n\n"+
				"Serão executados comandos privilegiados no servidor.",
			lv, gib, gib, fs,
		)
		dialog.ShowConfirm("Confirmar ampliação LVM", confirm, func(ok bool) {
			if !ok {
				return
			}
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
				defer cancel()
				if err := ui.ensureSudoSession(ctx); err != nil {
					fyne.Do(func() { dialog.ShowError(err, ui.win) })
					return
				}
				// lvextend + resize conforme FS
				inner := fmt.Sprintf(
					"set -e; LV=%s; "+
						"command -v lvextend >/dev/null 2>&1 || { echo 'lvextend não encontrado.' >&2; exit 1; }; "+
						"lvextend -L +%.10gG \"$LV\"",
					shellQuote(lv), gib,
				)
				cmd1 := fmt.Sprintf("sudo -S -p '' -u %s sh -lc %s", shellQuote(ui.sudoUser), shellQuote(inner))
				out1, stderr1, e1 := ui.runSSHCommandWithInput(ctx, cmd1, ui.sudoPass)
				if e1 != nil {
					fyne.Do(func() {
						msg := strings.TrimSpace(stderr1)
						if msg == "" {
							msg = strings.TrimSpace(out1)
						}
						dialog.ShowError(fmt.Errorf("lvextend: %w\n%s", e1, msg), ui.win)
					})
					return
				}
				growScript := growFSScript(lv, fs)
				cmd2 := fmt.Sprintf("sudo -S -p '' -u %s sh -lc %s", shellQuote(ui.sudoUser), shellQuote(growScript))
				out2, stderr2, e2 := ui.runSSHCommandWithInput(ctx, cmd2, ui.sudoPass)
				msg := strings.TrimSpace(out1)
				if msg != "" {
					msg += "\n\n"
				}
				msg += strings.TrimSpace(out2)
				if strings.TrimSpace(stderr2) != "" {
					msg += "\n\nstderr:\n" + strings.TrimSpace(stderr2)
				}
				if e2 != nil {
					fyne.Do(func() {
						dialog.ShowError(fmt.Errorf("extensão do sistema de ficheiros falhou: %w\n%s", e2, msg), ui.win)
					})
					return
				}
				appendAuditLog("disco", fmt.Sprintf("LV ampliado: %s +%.4g GiB (%s)", lv, gib, fs))
				fyne.Do(func() {
					dialog.ShowInformation("Ampliar LV", "Operação concluída (rever saída abaixo).\n\n"+truncateRunes(msg, 1800), ui.win)
					go doRefresh()
				})
			}()
		}, ui.win)
	}

	topBar := fynecontainer.NewVBox(
		fynecontainer.NewHBox(btnSudoOn, btnSudoOff, layout.NewSpacer()),
		sudoStatus,
		hint,
		fynecontainer.NewHBox(btnRefresh, autoCheck, layout.NewSpacer(), updatedAt),
	)

	root := fynecontainer.NewBorder(
		fynecontainer.NewPadded(topBar),
		nil,
		nil,
		nil,
		tabs,
	)

	// Sincronizar estado sudo quando voltar do diálogo global
	fyne.Do(func() {
		updateSudoBanner()
	})

	tabs.OnSelected = func(*fynecontainer.TabItem) {
		updateSudoBanner()
	}

	go func() {
		tick := time.NewTicker(90 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stopAuto:
				return
			case <-tick.C:
				if !autoOn.Load() {
					continue
				}
				if tabs.CurrentTab() != hostTab {
					continue
				}
				doRefresh()
			}
		}
	}()

	ui.openSettingsFullscreenWithBack("Discos e armazenamento no servidor", root, func() {
		stopOnce.Do(func() { close(stopAuto) })
	})

	go doRefresh()
}

func sortWrap(s *widget.Select) fyne.CanvasObject {
	return fynecontainer.NewGridWrap(fyne.NewSize(260, s.MinSize().Height), s)
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func growFSScript(lv, fs string) string {
	fs = strings.ToLower(strings.TrimSpace(fs))
	q := shellQuote(lv)
	switch fs {
	case "ext4", "ext3", "ext2":
		return "set -e; LV=" + q + "; command -v resize2fs >/dev/null 2>&1 || { echo 'resize2fs não encontrado.' >&2; exit 1; }; resize2fs \"$LV\""
	case "xfs":
		return "set -e; LV=" + q + "; command -v xfs_growfs >/dev/null 2>&1 || { echo 'xfs_growfs não encontrado.' >&2; exit 1; }; " +
			"M=$(findmnt -sn -o TARGET --source \"$LV\" 2>/dev/null || true); " +
			"if [ -z \"$M\" ]; then echo 'Não foi possível resolver o ponto de montagem para xfs_growfs.' >&2; exit 1; fi; " +
			"xfs_growfs \"$M\""
	case "btrfs":
		return "set -e; LV=" + q + "; command -v btrfs >/dev/null 2>&1 || { echo 'btrfs não encontrado.' >&2; exit 1; }; " +
			"M=$(findmnt -sn -o TARGET --source \"$LV\" 2>/dev/null || true); " +
			"if [ -z \"$M\" ]; then echo 'Não foi possível resolver o ponto de montagem para btrfs.' >&2; exit 1; fi; " +
			"btrfs filesystem resize max \"$M\""
	default:
		return "echo 'Sistema de ficheiros não suportado para crescimento automático.' >&2; exit 1"
	}
}
