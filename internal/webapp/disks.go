package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"containerway/internal/diskutil"
)

func (b *sshBundle) runDiskProbeScript(ctx context.Context) (string, error) {
	if b == nil || b.Sess == nil || b.Sess.SSH == nil {
		return "", fmt.Errorf("sessão SSH indisponível")
	}
	script := diskutil.ProbeScript
	b.sudoMu.Lock()
	sudoOn := b.sudoEnabled
	user := b.sudoUser
	pass := b.sudoPass
	b.sudoMu.Unlock()
	if sudoOn && strings.TrimSpace(user) != "" && strings.TrimSpace(pass) != "" {
		if err := b.ensureSudoSession(ctx); err != nil {
			return "", err
		}
		cmd := fmt.Sprintf("sudo -S -p '' -u %s sh -lc %s", shellQuote(user), shellQuote(script))
		out, _, err := b.runSSHCommand(ctx, cmd, pass)
		return out, err
	}
	cmd := "sh -lc " + shellQuote(script)
	out, _, err := b.runSSHCommand(ctx, cmd, "")
	return out, err
}

func parseDiskSortMode(s string) diskutil.SortMode {
	switch strings.TrimSpace(s) {
	case "block":
		return diskutil.SortBlockDesc
	case "use":
		return diskutil.SortUsePctDesc
	default:
		return diskutil.SortLsblkOrder
	}
}

// handleDisksProbe devolve sondagem completa (tabela, técnico, LVM).
func (s *Server) handleDisksProbe(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha na sondagem: %v", err)})
		return
	}
	lsblkJ, dfB, dfMapB, lvsB, vgsB, pvsB := diskutil.SplitProbe(raw)
	sortMode := parseDiskSortMode(r.URL.Query().Get("sort"))
	filter := r.URL.Query().Get("filter")
	showLoop := r.URL.Query().Get("showLoop") == "1" || r.URL.Query().Get("showLoop") == "true"
	rows := diskutil.BuildRows(lsblkJ, dfB, dfMapB, sortMode, filter, showLoop)
	lvRecs := diskutil.ParseLVSBlock(lvsB)
	vgStats := diskutil.ParseVGSBlock(vgsB)
	writeJSON(w, http.StatusOK, map[string]any{
		"rows":       rows,
		"technical":  diskutil.BuildTechnicalText(dfB, lvsB, vgsB, pvsB),
		"lvs":        lvsB,
		"lvRecords":  lvRecs,
		"vgStats":    vgStats,
		"lvOptions":  diskutil.AssistantOptions(rows, lvRecs),
		"usageRoots": diskutil.UsageRootsFromRows(rows),
		"updatedAt":  time.Now().Format("15:04:05"),
		"sudo":       b.sudoStatus(),
	})
}

type disksExtendRequest struct {
	LV       string  `json:"lv"`
	GiB      float64 `json:"gib"`
	FS       string  `json:"fs"`
	SizeMode string  `json:"sizeMode"`
}

// handleDisksExtendLV executa lvextend + resize do sistema de ficheiros (requer sudo).
func (s *Server) handleDisksExtendLV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req disksExtendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	lv := strings.TrimSpace(req.LV)
	if lv == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique o caminho do LV"})
		return
	}
	if req.GiB <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique um valor positivo em GB"})
		return
	}
	fs := strings.TrimSpace(req.FS)
	if fs == "" {
		fs = "ext4"
	}
	b.sudoMu.Lock()
	sudoOn := b.sudoEnabled
	user := b.sudoUser
	pass := b.sudoPass
	b.sudoMu.Unlock()
	if !sudoOn || strings.TrimSpace(user) == "" || strings.TrimSpace(pass) == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "ative o sudo para ampliar volumes LVM"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
	defer cancel()
	if err := b.ensureSudoSession(ctx); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	cmd1 := fmt.Sprintf("sudo -S -p '' -u %s sh -lc %s", shellQuote(user), shellQuote(disksLVScriptExtend(lv, req.GiB, req.SizeMode)))
	out1, stderr1, e1 := b.runSSHCommand(ctx, cmd1, pass)
	if e1 != nil {
		msg := strings.TrimSpace(stderr1)
		if msg == "" {
			msg = strings.TrimSpace(out1)
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("lvextend: %v — %s", e1, msg)})
		return
	}
	growScript := diskutil.GrowFSScript(lv, fs)
	cmd2 := fmt.Sprintf("sudo -S -p '' -u %s sh -lc %s", shellQuote(user), shellQuote(growScript))
	out2, stderr2, e2 := b.runSSHCommand(ctx, cmd2, pass)
	msg := strings.TrimSpace(out1)
	if msg != "" {
		msg += "\n\n"
	}
	msg += strings.TrimSpace(out2)
	if strings.TrimSpace(stderr2) != "" {
		msg += "\n\nstderr:\n" + strings.TrimSpace(stderr2)
	}
	if e2 != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("redimensionar FS: %v", e2), "output": msg})
		return
	}
	b.addOperation(fmt.Sprintf("LV ampliado: %s +%.4g GB (%s)", lv, req.GiB, fs), "info")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"output": msg,
	})
}

type disksShrinkRequest struct {
	LV       string  `json:"lv"`
	GiB      float64 `json:"gib"`
	FS       string  `json:"fs"`
	SizeMode string  `json:"sizeMode"`
}

// handleDisksShrinkLV reduz um LV (resize do FS + lvreduce; requer sudo).
func (s *Server) handleDisksShrinkLV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req disksShrinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	lv := strings.TrimSpace(req.LV)
	if lv == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique o caminho do LV"})
		return
	}
	if req.GiB <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique um valor positivo em GB"})
		return
	}
	fs := strings.TrimSpace(req.FS)
	if fs == "" {
		fs = "ext4"
	}
	b.sudoMu.Lock()
	sudoOn := b.sudoEnabled
	user := b.sudoUser
	pass := b.sudoPass
	b.sudoMu.Unlock()
	if !sudoOn || strings.TrimSpace(user) == "" || strings.TrimSpace(pass) == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "ative o sudo para reduzir volumes LVM"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
	defer cancel()
	if err := b.ensureSudoSession(ctx); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	shrinkScript := disksLVScriptShrink(lv, req.GiB, fs, req.SizeMode)
	cmd := fmt.Sprintf("sudo -S -p '' -u %s sh -lc %s", shellQuote(user), shellQuote(shrinkScript))
	out, stderr, err := b.runSSHCommand(ctx, cmd, pass)
	msg := strings.TrimSpace(out)
	if strings.TrimSpace(stderr) != "" {
		if msg != "" {
			msg += "\n\n"
		}
		msg += "stderr:\n" + strings.TrimSpace(stderr)
	}
	if err != nil {
		hint := diskutil.InterpretLVCmdError(stderr, out, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":  fmt.Sprintf("reduzir LV: %s", hint),
			"output": msg,
		})
		return
	}
	b.addOperation(fmt.Sprintf("LV reduzido: %s -%.4g GB (%s)", lv, req.GiB, fs), "info")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"output": msg,
	})
}
