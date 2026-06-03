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

func (b *sshBundle) disksRequireSudo(w http.ResponseWriter) (user, pass string, ok bool) {
	b.sudoMu.Lock()
	sudoOn := b.sudoEnabled
	user = b.sudoUser
	pass = b.sudoPass
	b.sudoMu.Unlock()
	if !sudoOn || strings.TrimSpace(user) == "" || strings.TrimSpace(pass) == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "ative o sudo para esta operação LVM"})
		return "", "", false
	}
	return user, pass, true
}

func (s *Server) disksRunSudoScript(ctx context.Context, b *sshBundle, w http.ResponseWriter, script string) (string, bool) {
	user, pass, ok := b.disksRequireSudo(w)
	if !ok {
		return "", false
	}
	if err := b.ensureSudoSession(ctx); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return "", false
	}
	cmd := fmt.Sprintf("sudo -S -p '' -u %s sh -lc %s", shellQuote(user), shellQuote(script))
	out, stderr, err := b.runSSHCommand(ctx, cmd, pass)
	msg := strings.TrimSpace(out)
	if strings.TrimSpace(stderr) != "" {
		if msg != "" {
			msg += "\n\n"
		}
		msg += "stderr:\n" + strings.TrimSpace(stderr)
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "output": msg})
		return "", false
	}
	return msg, true
}

type disksLVRequest struct {
	LV       string  `json:"lv"`
	GiB      float64 `json:"gib"`
	FS       string  `json:"fs"`
	SizeMode string  `json:"sizeMode"` // delta | absolute
}

func disksNormFS(fs string) string {
	fs = strings.TrimSpace(fs)
	if fs == "" {
		return "ext4"
	}
	return fs
}

func disksNormSizeMode(m string) string {
	if strings.TrimSpace(m) == "absolute" {
		return "absolute"
	}
	return "delta"
}

func disksLVScriptExtend(lv string, gib float64, mode string) string {
	if disksNormSizeMode(mode) == "absolute" {
		return diskutil.LvextendAbsScript(lv, gib)
	}
	return diskutil.LvextendScript(lv, gib)
}

func disksLVScriptShrink(lv string, gib float64, fs, mode string) string {
	if disksNormSizeMode(mode) == "absolute" {
		return diskutil.LvreduceAbsScript(lv, gib, fs)
	}
	return diskutil.ShrinkLVScript(lv, gib, fs)
}

// handleDisksFsck verificação read-only do FS.
func (s *Server) handleDisksFsck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req disksLVRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	lv := strings.TrimSpace(req.LV)
	if lv == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique o LV"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	script := diskutil.FsckCheckScript(lv, disksNormFS(req.FS))
	msg, ok2 := s.disksRunSudoScript(ctx, b, w, script)
	if !ok2 {
		return
	}
	b.addOperation("fsck: "+lv, "info")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": msg})
}

type disksSnapshotCreateRequest struct {
	LV       string  `json:"lv"`
	Name     string  `json:"name"`
	GiB      float64 `json:"gib"`
}

func (s *Server) handleDisksSnapshotCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req disksSnapshotCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	lv := strings.TrimSpace(req.LV)
	name := strings.TrimSpace(req.Name)
	if lv == "" || name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique LV e nome do snapshot"})
		return
	}
	if req.GiB <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique tamanho positivo em GB"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	script := diskutil.SnapshotCreateScript(lv, name, req.GiB)
	msg, ok2 := s.disksRunSudoScript(ctx, b, w, script)
	if !ok2 {
		return
	}
	b.addOperation(fmt.Sprintf("snapshot criado: %s em %s", name, lv), "info")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": msg})
}

type disksSnapshotRemoveRequest struct {
	SnapPath string `json:"snapPath"`
}

func (s *Server) handleDisksSnapshotRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req disksSnapshotRemoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	snap := strings.TrimSpace(req.SnapPath)
	if snap == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique o caminho do snapshot"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	script := diskutil.SnapshotRemoveScript(snap)
	msg, ok2 := s.disksRunSudoScript(ctx, b, w, script)
	if !ok2 {
		return
	}
	b.addOperation("snapshot removido: "+snap, "info")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": msg})
}

type disksLVCreateRequest struct {
	VG   string  `json:"vg"`
	Name string  `json:"name"`
	GiB  float64 `json:"gib"`
	FS   string  `json:"fs"`
	Mkfs bool    `json:"mkfs"`
}

func (s *Server) handleDisksLVCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req disksLVCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	vg := strings.TrimSpace(req.VG)
	name := strings.TrimSpace(req.Name)
	if vg == "" || name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique VG e nome do LV"})
		return
	}
	if req.GiB <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique tamanho positivo em GB"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	script := diskutil.LVCreateScript(vg, name, req.GiB, disksNormFS(req.FS), req.Mkfs)
	msg, ok2 := s.disksRunSudoScript(ctx, b, w, script)
	if !ok2 {
		return
	}
	b.addOperation(fmt.Sprintf("LV criado: %s/%s", vg, name), "info")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": msg})
}

type disksResizeFSRequest struct {
	LV   string  `json:"lv"`
	GiB  float64 `json:"gib"`
	FS   string  `json:"fs"`
	Grow bool    `json:"grow"`
}

func (s *Server) handleDisksResizeFS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req disksResizeFSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	lv := strings.TrimSpace(req.LV)
	if lv == "" || req.GiB <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique LV e GB positivo"})
		return
	}
	fs := disksNormFS(req.FS)
	if !req.Grow && fs == "xfs" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "XFS não suporta reduzir o sistema de arquivos"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
	defer cancel()
	script := diskutil.ResizeFSScriptOnly(lv, fs, req.GiB, req.Grow)
	msg, ok2 := s.disksRunSudoScript(ctx, b, w, script)
	if !ok2 {
		return
	}
	b.addOperation(fmt.Sprintf("resize FS %s grow=%v", lv, req.Grow), "info")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": msg})
}

type disksLVRenameRequest struct {
	LV      string `json:"lv"`
	NewName string `json:"newName"`
}

func (s *Server) handleDisksLVRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req disksLVRenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	lv := strings.TrimSpace(req.LV)
	newName := strings.TrimSpace(req.NewName)
	if lv == "" || newName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique LV e novo nome"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	vg, oldName := diskutil.LVNameAndVG(lv, "")
	if vg == "" || oldName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "não foi possível resolver VG/nome do LV"})
		return
	}
	script := diskutil.LVRenameScript(vg, oldName, newName)
	msg, ok2 := s.disksRunSudoScript(ctx, b, w, script)
	if !ok2 {
		return
	}
	b.addOperation(fmt.Sprintf("LV renomeado: %s → %s", oldName, newName), "info")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": msg})
}

type disksVgchangeRequest struct {
	VG       string `json:"vg"`
	Activate bool   `json:"activate"`
}

func (s *Server) handleDisksVgchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req disksVgchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	vg := strings.TrimSpace(req.VG)
	if vg == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique o VG"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	script := diskutil.VgchangeScript(vg, req.Activate)
	msg, ok2 := s.disksRunSudoScript(ctx, b, w, script)
	if !ok2 {
		return
	}
	b.addOperation(fmt.Sprintf("vgchange %s activate=%v", vg, req.Activate), "info")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": msg})
}

func (s *Server) handleDisksFstrim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var req disksLVRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	lv := strings.TrimSpace(req.LV)
	if lv == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique o LV"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	script := diskutil.FstrimScript(lv)
	msg, ok2 := s.disksRunSudoScript(ctx, b, w, script)
	if !ok2 {
		return
	}
	b.addOperation("fstrim: "+lv, "info")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": msg})
}

func (s *Server) handleDisksSmart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	dev := strings.TrimSpace(r.URL.Query().Get("dev"))
	if dev == "" && r.Method == http.MethodPost {
		var body struct {
			Dev string `json:"dev"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		dev = strings.TrimSpace(body.Dev)
	}
	if dev == "" || !strings.HasPrefix(dev, "/dev/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "indique dispositivo /dev/…"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	script := diskutil.SmartctlScript(dev)
	runPlain := func() (string, error) {
		cmd := "sh -lc " + shellQuote(script)
		out, stderr, err := b.runSSHCommand(ctx, cmd, "")
		msg := strings.TrimSpace(out)
		if strings.TrimSpace(stderr) != "" {
			if msg != "" {
				msg += "\n\n"
			}
			msg += "stderr:\n" + strings.TrimSpace(stderr)
		}
		return msg, err
	}
	b.sudoMu.Lock()
	sudoOn := b.sudoEnabled && strings.TrimSpace(b.sudoUser) != "" && strings.TrimSpace(b.sudoPass) != ""
	user := b.sudoUser
	pass := b.sudoPass
	b.sudoMu.Unlock()
	var msg string
	var err error
	if sudoOn && b.ensureSudoSession(ctx) == nil {
		cmd := fmt.Sprintf("sudo -S -p '' -u %s sh -lc %s", shellQuote(user), shellQuote(script))
		out, stderr, e := b.runSSHCommand(ctx, cmd, pass)
		msg = strings.TrimSpace(out)
		if strings.TrimSpace(stderr) != "" {
			if msg != "" {
				msg += "\n\n"
			}
			msg += "stderr:\n" + strings.TrimSpace(stderr)
		}
		err = e
	}
	if err != nil {
		msg, err = runPlain()
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error(), "output": msg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": msg})
}
