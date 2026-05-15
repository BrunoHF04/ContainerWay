package appui

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	fynecontainer "fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	dcontainer "github.com/docker/docker/api/types/container"
	dcontainertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

type dockerManagerRow struct {
	ID          string
	Name        string
	ShortID     string
	State       string
	Image       string
	StatsLine   string
	DetailsLine string
	ExtraLine   string
	CPUPercent  float64
	MemUsage    uint64
	MemLimit    uint64
	MemPercent  float64
	NetRx       uint64
	NetTx       uint64
	DiskRead    uint64
	DiskWrite   uint64
	NetRxRate   uint64
	NetTxRate   uint64
	DiskRRate   uint64
	DiskWRate   uint64
	Pids        uint64
	RestartCnt  int
	UptimeLabel string
	Running     bool
	Restarting  bool
	AlertLevel  int // 0 normal, 1 alerta, 2 crítico
}

type dockerRateSample struct {
	At        time.Time
	NetRx     uint64
	NetTx     uint64
	DiskRead  uint64
	DiskWrite uint64
}

var errComposeMetadataMissing = errors.New("metadados do docker compose ausentes")

// dockerStateLabel devolve o rótulo traduzido do estado do contêiner.
func dockerStateLabel(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running":
		return tr("mod_docker_state_running")
	case "exited":
		return tr("mod_docker_state_exited")
	case "paused":
		return tr("mod_docker_state_paused")
	case "restarting":
		return tr("mod_docker_state_restarting")
	case "dead":
		return tr("mod_docker_state_dead")
	case "created":
		return tr("mod_docker_state_created")
	case "removing":
		return tr("mod_docker_state_removing")
	default:
		if state == "" {
			return tr("mod_docker_state_unknown")
		}
		return state
	}
}

// buildDockerManagerRows executa parte da logica deste modulo.
func buildDockerManagerRows(list []dcontainer.Summary) []dockerManagerRow {
	out := make([]dockerManagerRow, 0, len(list))
	for _, c := range list {
		if c.State != dcontainer.StateRunning && c.State != dcontainer.StateRestarting {
			continue
		}
		id := strings.TrimPrefix(c.ID, "sha256:")
		short := id
		if len(short) > 12 {
			short = short[:12]
		}
		disp := containerDisplayName(c)
		if disp == "" {
			disp = tr("mod_docker_no_name")
		}
		st := string(c.State)
		out = append(out, dockerManagerRow{
			ID:          c.ID,
			Name:        disp,
			ShortID:     short,
			State:       st,
			Image:       c.Image,
			StatsLine:   tr("mod_docker_stats_pending"),
			DetailsLine: fmt.Sprintf(tr("mod_docker_details_short_fmt"), dockerStateLabel(st), short),
			ExtraLine:   "",
			Running:     c.State == dcontainer.StateRunning,
			Restarting:  c.State == dcontainer.StateRestarting,
		})
	}
	return out
}

// formatBytes executa parte da logica deste modulo.
func formatBytes(v uint64) string {
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

// formatRateBytes executa parte da logica deste modulo.
func formatRateBytes(v uint64) string {
	return formatBytes(v) + "/s"
}

// computeCPUPercent executa parte da logica deste modulo.
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

// collectNetIO executa parte da logica deste modulo.
func collectNetIO(stats dcontainertypes.StatsResponse) (rx, tx uint64) {
	for _, nw := range stats.Networks {
		rx += nw.RxBytes
		tx += nw.TxBytes
	}
	return rx, tx
}

// collectBlockIO executa parte da logica deste modulo.
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

// calcUptimeLabel executa parte da logica deste modulo.
func calcUptimeLabel(startedAt string) string {
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

// alertLevel executa parte da logica deste modulo.
func alertLevel(cpuPct, memPct float64) int {
	if cpuPct >= 90 || memPct >= 90 {
		return 2
	}
	if cpuPct >= 75 || memPct >= 75 {
		return 1
	}
	return 0
}

// rowBgByAlert executa parte da logica deste modulo.
func rowBgByAlert(level int) color.Color {
	switch level {
	case 2:
		return color.NRGBA{R: 128, G: 30, B: 30, A: 110}
	case 1:
		return color.NRGBA{R: 120, G: 95, B: 18, A: 105}
	default:
		return color.NRGBA{A: 0}
	}
}

// sortRows executa parte da logica deste modulo.
func sortRows(rows []dockerManagerRow, key string) {
	switch key {
	case tr("mod_docker_sort_name_az"):
		sort.SliceStable(rows, func(i, j int) bool {
			return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
		})
	case tr("mod_docker_sort_name_za"):
		sort.SliceStable(rows, func(i, j int) bool {
			return strings.ToLower(rows[i].Name) > strings.ToLower(rows[j].Name)
		})
	case tr("mod_docker_sort_cpu"):
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].CPUPercent > rows[j].CPUPercent })
	case tr("mod_docker_sort_mem"):
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].MemPercent > rows[j].MemPercent })
	case tr("mod_docker_sort_net"):
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].NetRx+rows[i].NetTx > rows[j].NetRx+rows[j].NetTx })
	case tr("mod_docker_sort_disk"):
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].DiskRead+rows[i].DiskWrite > rows[j].DiskRead+rows[j].DiskWrite })
	case tr("mod_docker_sort_restarts"):
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].RestartCnt > rows[j].RestartCnt })
	}
}

// matchRowFilter executa parte da logica deste modulo.
func matchRowFilter(r dockerManagerRow, q string) bool {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		return true
	}
	h := strings.ToLower(strings.Join([]string{
		r.Name,
		r.ShortID,
		r.State,
		r.Image,
		r.UptimeLabel,
	}, " "))
	return strings.Contains(h, q)
}

// loadDockerStatsRow executa parte da logica deste modulo.
func (ui *explorer) loadDockerStatsRow(containerID string, prev dockerRateSample) (dockerManagerRow, dockerRateSample, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	resp, err := ui.s.Docker.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return dockerManagerRow{}, dockerRateSample{}, err
	}
	defer resp.Body.Close()

	var stats dcontainertypes.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return dockerManagerRow{}, dockerRateSample{}, err
	}
	inspect, err := ui.s.Docker.ContainerInspect(ctx, containerID)
	if err != nil {
		return dockerManagerRow{}, dockerRateSample{}, err
	}

	cpuPct := computeCPUPercent(stats)
	memUsage := stats.MemoryStats.Usage
	memLimit := stats.MemoryStats.Limit
	memPct := 0.0
	if memLimit > 0 {
		memPct = (float64(memUsage) / float64(memLimit)) * 100
	}

	rx, tx := collectNetIO(stats)
	rd, wr := collectBlockIO(stats)
	pids := stats.PidsStats.Current
	now := time.Now()

	var rxRate, txRate, rdRate, wrRate uint64
	if !prev.At.IsZero() {
		sec := now.Sub(prev.At).Seconds()
		if sec > 0.1 {
			rxRate = uint64(float64(rx-prev.NetRx) / sec)
			txRate = uint64(float64(tx-prev.NetTx) / sec)
			rdRate = uint64(float64(rd-prev.DiskRead) / sec)
			wrRate = uint64(float64(wr-prev.DiskWrite) / sec)
		}
	}
	sample := dockerRateSample{At: now, NetRx: rx, NetTx: tx, DiskRead: rd, DiskWrite: wr}

	main := fmt.Sprintf(
		tr("mod_docker_stats_line_fmt"),
		math.Max(cpuPct, 0),
		formatBytes(memUsage),
		formatBytes(memLimit),
		math.Max(memPct, 0),
		formatBytes(rx),
		formatBytes(tx),
		formatRateBytes(rxRate),
		formatRateBytes(txRate),
		formatBytes(rd),
		formatBytes(wr),
		formatRateBytes(rdRate),
		formatRateBytes(wrRate),
	)
	uptime := calcUptimeLabel(inspect.State.StartedAt)
	details := fmt.Sprintf(tr("mod_docker_details_line_fmt"), dockerStateLabel(inspect.State.Status), uptime, inspect.RestartCount, pids)
	extra := fmt.Sprintf(tr("mod_docker_image_fmt"), truncateRunes(inspect.Image, 84))

	return dockerManagerRow{
		ID:          inspect.ID,
		Name:        strings.TrimPrefix(inspect.Name, "/"),
		ShortID:     truncateRunes(strings.TrimPrefix(inspect.ID, "sha256:"), 12),
		State:       inspect.State.Status,
		Image:       inspect.Config.Image,
		StatsLine:   main,
		DetailsLine: details,
		ExtraLine:   extra,
		CPUPercent:  cpuPct,
		MemUsage:    memUsage,
		MemLimit:    memLimit,
		MemPercent:  memPct,
		NetRx:       rx,
		NetTx:       tx,
		DiskRead:    rd,
		DiskWrite:   wr,
		NetRxRate:   rxRate,
		NetTxRate:   txRate,
		DiskRRate:   rdRate,
		DiskWRate:   wrRate,
		Pids:        pids,
		RestartCnt:  inspect.RestartCount,
		UptimeLabel: uptime,
		Running:     inspect.State.Running,
		Restarting:  inspect.State.Restarting,
		AlertLevel:  alertLevel(cpuPct, memPct),
	}, sample, nil
}

// loadDockerContainerLogs executa parte da logica deste modulo.
func (ui *explorer) loadDockerContainerLogs(containerID string, tail int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	resp, err := ui.s.Docker.ContainerLogs(ctx, containerID, dcontainertypes.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Tail:       strconv.Itoa(tail),
	})
	if err != nil {
		return "", err
	}
	defer resp.Close()

	raw, err := io.ReadAll(resp)
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return tr("mod_docker_no_logs"), nil
	}

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	if _, demuxErr := stdcopy.StdCopy(&outBuf, &errBuf, bytes.NewReader(raw)); demuxErr != nil {
		// Alguns contêineres (TTY) retornam stream sem multiplexação; nesse caso usamos o bruto.
		text := strings.TrimSpace(string(raw))
		if text == "" {
			return tr("mod_docker_no_logs"), nil
		}
		return text, nil
	}

	text := strings.TrimSpace(outBuf.String())
	stderrText := strings.TrimSpace(errBuf.String())
	if stderrText != "" {
		if text != "" {
			text += "\n\n"
		}
		text += "[stderr]\n" + stderrText
	}
	if strings.TrimSpace(text) == "" {
		return tr("mod_docker_no_logs"), nil
	}
	return text, nil
}

// forceRecreateContainer tenta recriar via Docker Compose quando possível.
func (ui *explorer) forceRecreateContainer(containerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	inspect, err := ui.s.Docker.ContainerInspect(ctx, containerID)
	if err != nil {
		return err
	}
	labels := inspect.Config.Labels
	project := strings.TrimSpace(labels["com.docker.compose.project"])
	service := strings.TrimSpace(labels["com.docker.compose.service"])
	if project == "" || service == "" {
		return errComposeMetadataMissing
	}
	workingDir := strings.TrimSpace(labels["com.docker.compose.project.working_dir"])
	configFilesRaw := strings.TrimSpace(labels["com.docker.compose.project.config_files"])
	configFiles := make([]string, 0, 4)
	for _, f := range strings.Split(configFilesRaw, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !filepath.IsAbs(f) && workingDir != "" {
			f = filepath.Join(workingDir, f)
		}
		configFiles = append(configFiles, f)
	}

	base := "docker compose -p " + shellQuote(project)
	for _, f := range configFiles {
		base += " -f " + shellQuote(f)
	}
	commandVariants := []string{
		base + " up -d --force-recreate --pull always " + shellQuote(service),
		base + " up -d --force-recreate " + shellQuote(service),
		"docker-compose -p " + shellQuote(project) + " up -d --force-recreate " + shellQuote(service),
	}
	if workingDir != "" {
		for i, cmd := range commandVariants {
			commandVariants[i] = "cd " + shellQuote(workingDir) + " && " + cmd
		}
	}

	var lastErr error
	for _, cmd := range commandVariants {
		stdout, stderr, runErr := ui.runSSHCommandWithInput(ctx, "sh -lc "+shellQuote(cmd), "")
		if runErr == nil {
			return nil
		}
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		if detail != "" {
			lastErr = fmt.Errorf("%v: %s", runErr, detail)
		} else {
			lastErr = runErr
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf(tr("mod_docker_recreate_fail_fmt"), service)
	}
	return lastErr
}

// showDockerContainerManager executa parte da logica deste modulo.
func (ui *explorer) showDockerContainerManager() {
	if ui.s == nil || ui.s.Docker == nil {
		dialog.ShowError(fmt.Errorf("%s", tr("mod_docker_client_unavail")), ui.win)
		return
	}
	appendAuditLog("docker", tr("mod_docker_mgr_audit_opened"))

	skCPU := tr("mod_docker_sort_cpu")
	skMem := tr("mod_docker_sort_mem")
	skNet := tr("mod_docker_sort_net")
	skDisk := tr("mod_docker_sort_disk")
	skRestarts := tr("mod_docker_sort_restarts")
	skNameAZ := tr("mod_docker_sort_name_az")
	skNameZA := tr("mod_docker_sort_name_za")

	rows := []dockerManagerRow{}
	allRows := []dockerManagerRow{}
	prevSamples := map[string]dockerRateSample{}
	filterQuery := ""
	sortKey := skCPU
	expanded := map[string]bool{}
	listWidget := widget.NewList(
		func() int { return len(rows) },
		func() fyne.CanvasObject {
			bg := canvas.NewRectangle(color.NRGBA{A: 0})
			title := widget.NewLabel("container")
			title.TextStyle = fyne.TextStyle{Bold: true}
			title.Wrapping = fyne.TextWrapOff
			stats := widget.NewLabel("stats")
			stats.Wrapping = fyne.TextWrapOff
			details := widget.NewLabel("details")
			details.Wrapping = fyne.TextWrapOff
			extra := widget.NewLabel("extra")
			extra.Wrapping = fyne.TextWrapOff
			content := fynecontainer.NewVBox(title, stats, details, extra, widget.NewSeparator())
			return fynecontainer.NewMax(bg, fynecontainer.NewPadded(content))
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			layer := o.(*fyne.Container)
			bg := layer.Objects[0].(*canvas.Rectangle)
			padded := layer.Objects[1].(*fyne.Container)
			box := padded.Objects[0].(*fyne.Container)
			title := box.Objects[0].(*widget.Label)
			stats := box.Objects[1].(*widget.Label)
			details := box.Objects[2].(*widget.Label)
			extra := box.Objects[3].(*widget.Label)
			if id < 0 || int(id) >= len(rows) {
				title.SetText("")
				stats.SetText("")
				details.SetText("")
				extra.SetText("")
				bg.FillColor = color.NRGBA{A: 0}
				bg.Refresh()
				return
			}
			row := rows[id]
			title.SetText(truncateRunes(fmt.Sprintf("%s  (%s)", row.Name, row.ShortID), 96))
			stats.SetText(truncateRunes(row.StatsLine, 160))
			details.SetText(truncateRunes(row.DetailsLine, 160))
			if expanded[row.ID] {
				extra.SetText(truncateRunes(row.ExtraLine, 160))
			} else {
				extra.SetText("")
			}
			bg.FillColor = rowBgByAlert(row.AlertLevel)
			bg.Refresh()
		},
	)

	hint := widget.NewLabel(tr("mod_docker_hint"))
	hint.Wrapping = fyne.TextWrapWord

	statusLbl := widget.NewLabel("")
	statusLbl.Wrapping = fyne.TextWrapOff
	detailsPanel := widget.NewLabel("")
	detailsPanel.Wrapping = fyne.TextWrapWord
	detailsScroll := fynecontainer.NewScroll(detailsPanel)

	var selectedID widget.ListItemID = -1
	selectedContainerID := ""
	autoRefresh := atomic.Bool{}
	autoRefresh.Store(true)
	refreshing := atomic.Bool{}
	stopAuto := make(chan struct{})
	var stopOnce sync.Once

	btnRestartSel := widget.NewButtonWithIcon(tr("mod_docker_btn_restart_sel"), theme.MediaReplayIcon(), nil)
	btnRestartSel.Importance = widget.HighImportance
	btnRestartSel.Disable()

	btnLogs := widget.NewButtonWithIcon(tr("mod_docker_btn_logs"), theme.DocumentIcon(), nil)
	btnLogs.Importance = widget.MediumImportance
	btnLogs.Disable()

	btnRestartAll := widget.NewButtonWithIcon(tr("mod_docker_btn_restart_all"), theme.ViewRefreshIcon(), nil)
	btnRestartAll.Importance = widget.WarningImportance
	btnToggleDetails := widget.NewButtonWithIcon(tr("mod_docker_btn_expand"), theme.VisibilityIcon(), nil)
	btnToggleDetails.Importance = widget.MediumImportance
	btnToggleDetails.Disable()
	btnExport := widget.NewButtonWithIcon(tr("mod_docker_btn_export_csv"), theme.DocumentSaveIcon(), nil)
	btnExport.Importance = widget.MediumImportance

	sortSelect := widget.NewSelect([]string{
		skCPU,
		skMem,
		skNet,
		skDisk,
		skRestarts,
		skNameAZ,
		skNameZA,
	}, func(sel string) {
		if strings.TrimSpace(sel) == "" {
			return
		}
		sortKey = sel
	})
	sortSelect.SetSelected(sortKey)
	filterEntry := widget.NewEntry()
	filterEntry.SetPlaceHolder(tr("mod_docker_filter_ph"))

	applyView := func() {
		filtered := make([]dockerManagerRow, 0, len(allRows))
		for _, r := range allRows {
			if matchRowFilter(r, filterQuery) {
				filtered = append(filtered, r)
			}
		}
		sortRows(filtered, sortKey)
		rows = filtered
		listWidget.Refresh()
		for i, r := range rows {
			if expanded[r.ID] {
				listWidget.SetItemHeight(widget.ListItemID(i), 152)
			} else {
				listWidget.SetItemHeight(widget.ListItemID(i), 116)
			}
		}
		selectedID = -1
		if selectedContainerID != "" {
			for i, r := range rows {
				if r.ID == selectedContainerID {
					selectedID = widget.ListItemID(i)
					listWidget.Select(selectedID)
					break
				}
			}
		}
		if selectedID < 0 || int(selectedID) >= len(rows) {
			listWidget.UnselectAll()
			btnRestartSel.Disable()
			btnLogs.Disable()
			btnToggleDetails.Disable()
			detailsPanel.SetText(tr("mod_docker_pick_details"))
		} else {
			btnRestartSel.Enable()
			btnLogs.Enable()
			btnToggleDetails.Enable()
		}
	}

	refresh := func() {
		if !refreshing.CompareAndSwap(false, true) {
			return
		}
		defer refreshing.Store(false)
		if ui.s == nil || ui.s.Docker == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		list, err := ui.s.Docker.ContainerList(ctx, dcontainer.ListOptions{All: false})
		if err != nil {
			fyne.Do(func() {
				statusLbl.SetText(fmt.Sprintf(tr("mod_docker_list_err_fmt"), err.Error()))
				rows = nil
				allRows = nil
				listWidget.Refresh()
				listWidget.UnselectAll()
				selectedID = -1
				btnRestartSel.Disable()
				btnLogs.Disable()
				btnToggleDetails.Disable()
			})
			return
		}
		next := buildDockerManagerRows(list)
		for i := range next {
			metricRow, sample, sErr := ui.loadDockerStatsRow(next[i].ID, prevSamples[next[i].ID])
			if sErr != nil {
				next[i].StatsLine = tr("mod_docker_stats_unavail")
				next[i].DetailsLine = fmt.Sprintf(tr("mod_docker_stats_err_detail_fmt"), dockerStateLabel(next[i].State), next[i].ShortID, sErr)
				continue
			}
			next[i] = metricRow
			prevSamples[next[i].ID] = sample
		}
		fyne.Do(func() {
			allRows = next
			mode := tr("mod_docker_realtime_on")
			if !autoRefresh.Load() {
				mode = tr("mod_docker_realtime_paused")
			}
			statusLbl.SetText(fmt.Sprintf(tr("mod_docker_status_bar_fmt"), len(allRows), time.Now().Format("15:04:05"), mode))
			applyView()
		})
	}

	btnRefresh := widget.NewButtonWithIcon(tr("mod_docker_btn_refresh"), theme.ViewRefreshIcon(), func() {
		fyne.Do(func() { statusLbl.SetText(tr("mod_docker_updating")) })
		go refresh()
	})
	var btnAuto *widget.Button
	btnAuto = widget.NewButtonWithIcon(tr("mod_docker_pause_rt"), theme.MediaPauseIcon(), func() {
		if autoRefresh.Load() {
			autoRefresh.Store(false)
			btnAuto.SetText(tr("mod_docker_start_rt"))
			btnAuto.SetIcon(theme.MediaPlayIcon())
			return
		}
		autoRefresh.Store(true)
		btnAuto.SetText(tr("mod_docker_pause_rt"))
		btnAuto.SetIcon(theme.MediaPauseIcon())
		go refresh()
	})
	btnAuto.Importance = widget.MediumImportance

	listWidget.OnSelected = func(id widget.ListItemID) {
		selectedID = id
		if id < 0 || int(id) >= len(rows) {
			selectedContainerID = ""
			btnRestartSel.Disable()
			btnLogs.Disable()
			btnToggleDetails.Disable()
			return
		}
		selectedContainerID = rows[id].ID
		btnRestartSel.Enable()
		btnLogs.Enable()
		btnToggleDetails.Enable()
		btnToggleDetails.SetText(tr("mod_docker_btn_expand"))
		if expanded[selectedContainerID] {
			btnToggleDetails.SetText(tr("mod_docker_btn_collapse"))
		}
		r := rows[id]
		detailsPanel.SetText(fmt.Sprintf(
			tr("mod_docker_detail_panel_fmt"),
			r.Name, r.ShortID, dockerStateLabel(r.State), r.Image,
			r.CPUPercent,
			formatBytes(r.MemUsage), formatBytes(r.MemLimit), r.MemPercent,
			formatBytes(r.NetRx), formatBytes(r.NetTx), formatBytes(r.NetRxRate), formatBytes(r.NetTxRate),
			formatBytes(r.DiskRead), formatBytes(r.DiskWrite), formatBytes(r.DiskRRate), formatBytes(r.DiskWRate),
			r.Pids, r.UptimeLabel, r.RestartCnt,
		))
	}
	listWidget.OnUnselected = func(_ widget.ListItemID) {
		selectedID = -1
		selectedContainerID = ""
		btnRestartSel.Disable()
		btnLogs.Disable()
		btnToggleDetails.Disable()
		detailsPanel.SetText(tr("mod_docker_pick_details"))
	}

	btnToggleDetails.OnTapped = func() {
		if selectedID < 0 || int(selectedID) >= len(rows) {
			return
		}
		id := rows[selectedID].ID
		expanded[id] = !expanded[id]
		if expanded[id] {
			btnToggleDetails.SetText(tr("mod_docker_btn_collapse"))
		} else {
			btnToggleDetails.SetText(tr("mod_docker_btn_expand"))
		}
		applyView()
	}

	filterEntry.OnChanged = func(text string) {
		filterQuery = text
		applyView()
	}

	btnExport.OnTapped = func() {
		saveDlg := dialog.NewFileSave(func(dst fyne.URIWriteCloser, err error) {
			if err != nil {
				dialog.ShowError(fmt.Errorf(tr("mod_export_prep_fail"), err), ui.win)
				return
			}
			if dst == nil {
				return
			}
			defer dst.Close()
			w := csv.NewWriter(dst)
			_ = w.Write([]string{"nome", "id_curto", "estado", "cpu_pct", "mem_uso", "mem_limite", "mem_pct", "net_rx", "net_tx", "disk_read", "disk_write", "net_rx_rate", "net_tx_rate", "disk_r_rate", "disk_w_rate", "pids", "uptime", "restarts", "imagem"})
			for _, r := range rows {
				_ = w.Write([]string{
					r.Name,
					r.ShortID,
					r.State,
					fmt.Sprintf("%.2f", r.CPUPercent),
					strconv.FormatUint(r.MemUsage, 10),
					strconv.FormatUint(r.MemLimit, 10),
					fmt.Sprintf("%.2f", r.MemPercent),
					strconv.FormatUint(r.NetRx, 10),
					strconv.FormatUint(r.NetTx, 10),
					strconv.FormatUint(r.DiskRead, 10),
					strconv.FormatUint(r.DiskWrite, 10),
					strconv.FormatUint(r.NetRxRate, 10),
					strconv.FormatUint(r.NetTxRate, 10),
					strconv.FormatUint(r.DiskRRate, 10),
					strconv.FormatUint(r.DiskWRate, 10),
					strconv.FormatUint(r.Pids, 10),
					r.UptimeLabel,
					strconv.Itoa(r.RestartCnt),
					r.Image,
				})
			}
			w.Flush()
			if wErr := w.Error(); wErr != nil {
				dialog.ShowError(fmt.Errorf(tr("mod_export_write_fail"), wErr), ui.win)
				return
			}
			dialog.ShowInformation(tr("mod_export_ok_title"), tr("mod_export_ok_body"), ui.win)
		}, ui.win)
		saveDlg.SetFileName("docker-stats-snapshot.csv")
		saveDlg.Show()
	}

	btnLogs.OnTapped = func() {
		if selectedID < 0 || int(selectedID) >= len(rows) {
			dialog.ShowInformation(tr("mod_logs_save_ok_title"), tr("mod_logs_pick"), ui.win)
			return
		}
		row := rows[selectedID]
		logText := widget.NewTextGrid()
		logText.ShowLineNumbers = false
		logScroll := fynecontainer.NewScroll(logText)
		logStatus := widget.NewLabel(tr("mod_docker_logs_loading"))
		logStatus.Wrapping = fyne.TextWrapWord
		currentLogText := ""
		logAuto := atomic.Bool{}
		logAuto.Store(true)
		logRefreshing := atomic.Bool{}
		stopLogs := make(chan struct{})
		var stopLogsOnce sync.Once

		loadLogs := func() {
			if !logRefreshing.CompareAndSwap(false, true) {
				return
			}
			go func() {
				defer logRefreshing.Store(false)
				text, err := ui.loadDockerContainerLogs(row.ID, 500)
				fyne.Do(func() {
					if err != nil {
						logStatus.SetText(fmt.Sprintf(tr("mod_docker_logs_load_fail_fmt"), err.Error()))
						return
					}
					currentLogText = text
					logText.SetText(text)
					mode := tr("mod_docker_realtime_on")
					if !logAuto.Load() {
						mode = tr("mod_docker_realtime_paused")
					}
					logStatus.SetText(fmt.Sprintf(tr("mod_docker_logs_status_fmt"), row.Name, row.ShortID, time.Now().Format("15:04:05"), mode))
				})
			}()
		}

		btnRefreshLogs := widget.NewButtonWithIcon(tr("mod_docker_logs_refresh"), theme.ViewRefreshIcon(), func() {
			logStatus.SetText(tr("mod_docker_logs_updating"))
			loadLogs()
		})
		btnRefreshLogs.Importance = widget.MediumImportance
		var btnAutoLogs *widget.Button
		btnAutoLogs = widget.NewButtonWithIcon(tr("mod_docker_pause_rt"), theme.MediaPauseIcon(), func() {
			if logAuto.Load() {
				logAuto.Store(false)
				btnAutoLogs.SetText(tr("mod_docker_start_rt"))
				btnAutoLogs.SetIcon(theme.MediaPlayIcon())
				logStatus.SetText(tr("mod_docker_logs_rt_paused"))
				return
			}
			logAuto.Store(true)
			btnAutoLogs.SetText(tr("mod_docker_pause_rt"))
			btnAutoLogs.SetIcon(theme.MediaPauseIcon())
			logStatus.SetText(tr("mod_docker_logs_rt_on_updating"))
			loadLogs()
		})
		btnAutoLogs.Importance = widget.MediumImportance
		btnDownloadLogs := widget.NewButtonWithIcon(tr("mod_docker_logs_download"), theme.DownloadIcon(), func() {
			saveDlg := dialog.NewFileSave(func(dst fyne.URIWriteCloser, err error) {
				if err != nil {
					dialog.ShowError(fmt.Errorf(tr("mod_logs_save_prep"), err), ui.win)
					return
				}
				if dst == nil {
					return
				}
				defer dst.Close()
				if _, wErr := io.WriteString(dst, currentLogText); wErr != nil {
					dialog.ShowError(fmt.Errorf(tr("mod_logs_save_write"), wErr), ui.win)
					return
				}
				dialog.ShowInformation(tr("mod_logs_save_ok_title"), tr("mod_logs_save_ok_body"), ui.win)
			}, ui.win)
			saveDlg.SetFileName(fmt.Sprintf("container-%s-logs.txt", row.ShortID))
			saveDlg.Show()
		})
		btnDownloadLogs.Importance = widget.MediumImportance

		body := fynecontainer.NewBorder(
			fynecontainer.NewVBox(logStatus, widget.NewSeparator(), fynecontainer.NewHBox(btnRefreshLogs, btnAutoLogs, btnDownloadLogs)),
			nil,
			nil,
			nil,
			logScroll,
		)
		dlg := dialog.NewCustom(
			fmt.Sprintf(tr("mod_docker_logs_dlg_title_fmt"), row.Name, row.ShortID),
			tr("compare_close"),
			fynecontainer.NewPadded(body),
			ui.win,
		)
		dlg.Resize(fyne.NewSize(960, 620))
		dlg.SetOnClosed(func() {
			stopLogsOnce.Do(func() { close(stopLogs) })
		})
		dlg.Show()
		loadLogs()
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stopLogs:
					return
				case <-ticker.C:
					if logAuto.Load() {
						loadLogs()
					}
				}
			}
		}()
	}

	doRestartOne := func(containerID, humanName string, onDone func(err error)) {
		go func() {
			if ui.s == nil || ui.s.Docker == nil {
				fyne.Do(func() { onDone(fmt.Errorf("%s", tr("mod_docker_session_unavail"))) })
				return
			}
			err := ui.forceRecreateContainer(containerID)
			if errors.Is(err, errComposeMetadataMissing) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				err = ui.s.Docker.ContainerRestart(ctx, containerID, dcontainer.StopOptions{})
			}
			fyne.Do(func() { onDone(err) })
			if err == nil {
				appendAuditLog("docker", fmt.Sprintf(tr("mod_docker_audit_restarted_fmt"), humanName))
			}
		}()
	}

	btnRestartSel.OnTapped = func() {
		if selectedID < 0 || int(selectedID) >= len(rows) {
			return
		}
		r := rows[selectedID]
		dialog.ShowConfirm(
			tr("mod_docker_confirm_one_title"),
			fmt.Sprintf(tr("mod_docker_confirm_one_body_fmt"), r.Name, r.ShortID),
			func(ok bool) {
				if !ok {
					return
				}
				fyne.Do(func() { statusLbl.SetText(tr("mod_docker_restarting_sel")) })
				doRestartOne(r.ID, r.Name, func(err error) {
					if err != nil {
						dialog.ShowError(fmt.Errorf(tr("mod_docker_restart_fail"), err), ui.win)
					} else {
						dialog.ShowInformation(tr("mod_docker_restart_title"), tr("mod_docker_restart_ok"), ui.win)
					}
					go refresh()
				})
			},
			ui.win,
		)
	}

	btnRestartAll.OnTapped = func() {
		targets := append([]dockerManagerRow(nil), rows...)
		if len(targets) == 0 {
			dialog.ShowInformation(tr("mod_docker_batch_title"), tr("mod_docker_none_running"), ui.win)
			return
		}
		dialog.ShowConfirm(
			tr("mod_docker_confirm_batch_title"),
			fmt.Sprintf(tr("mod_docker_confirm_batch_body_fmt"), len(targets)),
			func(ok bool) {
				if !ok {
					return
				}
				go func() {
					var errs []string
					for i, t := range targets {
						fyne.Do(func() {
							statusLbl.SetText(fmt.Sprintf(tr("mod_docker_restarting_batch_fmt"), i+1, len(targets)))
						})
						if ui.s == nil || ui.s.Docker == nil {
							errs = append(errs, tr("mod_docker_session_unavail"))
							break
						}
						err := ui.forceRecreateContainer(t.ID)
						if errors.Is(err, errComposeMetadataMissing) {
							ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
							err = ui.s.Docker.ContainerRestart(ctx, t.ID, dcontainer.StopOptions{})
							cancel()
						}
						if err != nil {
							errs = append(errs, fmt.Sprintf("%s: %v", truncateRunes(t.Name, 48), err))
						} else {
							appendAuditLog("docker", fmt.Sprintf(tr("mod_docker_audit_restarted_batch_fmt"), t.Name))
						}
					}
					fyne.Do(func() {
						if len(errs) == 0 {
							dialog.ShowInformation(tr("mod_docker_batch_title"), fmt.Sprintf(tr("mod_docker_batch_done_fmt"), len(targets)), ui.win)
						} else {
							dialog.ShowError(fmt.Errorf(tr("mod_docker_batch_err_fmt"), strings.Join(errs, "\n")), ui.win)
						}
						go refresh()
					})
				}()
			},
			ui.win,
		)
	}

	sortWrap := fynecontainer.NewGridWrap(fyne.NewSize(190, sortSelect.MinSize().Height), sortSelect)
	topControls := fynecontainer.NewBorder(
		nil,
		nil,
		widget.NewLabel(tr("mod_docker_filter_label")),
		fynecontainer.NewHBox(sortWrap, btnExport),
		filterEntry,
	)
	actions := fynecontainer.NewHBox(btnRefresh, btnAuto, btnLogs, btnToggleDetails, btnRestartSel, btnRestartAll, layout.NewSpacer())
	footer := fynecontainer.NewVBox(
		widget.NewSeparator(),
		actions,
		statusLbl,
	)
	mainSplit := fynecontainer.NewHSplit(
		fynecontainer.NewPadded(listWidget),
		fynecontainer.NewPadded(detailsScroll),
	)
	mainSplit.SetOffset(0.68)
	content := fynecontainer.NewBorder(
		fynecontainer.NewVBox(hint, widget.NewSeparator(), topControls),
		footer,
		nil,
		nil,
		mainSplit,
	)

	ui.openSettingsFullscreenWithBack(tr("mod_docker_mgr_screen_title"), content, func() {
		stopOnce.Do(func() { close(stopAuto) })
	})

	go refresh()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopAuto:
				return
			case <-ticker.C:
				if autoRefresh.Load() {
					go refresh()
				}
			}
		}
	}()
	detailsPanel.SetText(tr("mod_docker_mgr_select_details"))
}
