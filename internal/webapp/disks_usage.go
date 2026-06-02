package webapp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"containerway/internal/diskutil"
)

const usageScanMaxEntries = 400

// handleDisksUsage devolve tamanhos dos filhos imediatos de um diretório no host (estilo ncdu/TreeSize).
func (s *Server) handleDisksUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	dir, err := sanitizeDiskUsagePath(r.URL.Query().Get("path"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if r.URL.Query().Get("stream") == "1" {
		s.handleDisksUsageStream(w, r, b, dir)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	result, err := b.scanDiskUsage(ctx, dir)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDisksUsageStream(w http.ResponseWriter, r *http.Request, b *sshBundle, dir string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming indisponível"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	emit := func(ev map[string]any) error {
		if err := enc.Encode(ev); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	result, err := b.streamDiskUsage(ctx, dir, emit)
	if err != nil {
		_ = emit(map[string]any{"type": "error", "error": err.Error()})
		return
	}
	_ = emit(map[string]any{
		"type":       "done",
		"path":       result["path"],
		"parent":     result["parent"],
		"totalBytes": result["totalBytes"],
		"entries":    result["entries"],
		"truncated":  result["truncated"],
		"roots":      result["roots"],
	})
}

func sanitizeDiskUsagePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/", nil
	}
	p = path.Clean(p)
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("caminho inválido")
	}
	if p != "/" && strings.Contains(p, "..") {
		return "", fmt.Errorf("caminho inválido")
	}
	return p, nil
}

type diskUsageEntry struct {
	Name      string  `json:"name"`
	Path      string  `json:"path"`
	SizeBytes uint64  `json:"sizeBytes"`
	IsDir     bool    `json:"isDir"`
	Pct       float64 `json:"pct"`
	Error     string  `json:"error,omitempty"`
}

func diskUsageScript(dir string) string {
	return fmt.Sprintf(`set +e
P=%s
TOTAL_KB=$(du -x -sk "$P" 2>/dev/null | awk '{print $1+0}')
echo "TOTAL_KB=$TOTAL_KB"
TMP=$(mktemp)
find "$P" -mindepth 1 -maxdepth 1 2>/dev/null > "$TMP"
N=$(wc -l < "$TMP" | awk '{print $1+0}')
echo "ENTRY_COUNT=$N"
i=0
while IFS= read -r ent; do
  [ -z "$ent" ] && continue
  i=$((i+1))
  bn="${ent##*/}"
  echo "PROGRESS	$i	$N	$bn"
  sz=$(du -x -sk "$ent" 2>/dev/null | awk '{print $1+0}')
  if [ -d "$ent" ]; then t=d; else t=f; fi
  printf '%%s\t%%s\t%%s\t%%s\n' "$sz" "$ent" "$t" "$bn"
done < "$TMP"
rm -f "$TMP"
`, shellQuote(dir))
}

func (b *sshBundle) scanDiskUsage(ctx context.Context, dir string) (map[string]any, error) {
	out, err := b.runUsageScript(ctx, diskUsageScript(dir))
	if err != nil {
		return nil, err
	}
	return b.parseUsageOutput(dir, out)
}

func (b *sshBundle) streamDiskUsage(ctx context.Context, dir string, emit func(map[string]any) error) (map[string]any, error) {
	var totalKiB uint64
	var entries []diskUsageEntry
	truncated := false

	script := diskUsageScript(dir)
	err := b.runUsageScriptLines(ctx, script, func(line string) error {
		line = strings.TrimSpace(line)
		if line == "" {
			return nil
		}
		if strings.HasPrefix(line, "TOTAL_KB=") {
			v, _ := strconv.ParseUint(strings.TrimPrefix(line, "TOTAL_KB="), 10, 64)
			totalKiB = v
			return emit(map[string]any{"type": "phase", "phase": "total"})
		}
		if strings.HasPrefix(line, "ENTRY_COUNT=") {
			n, _ := strconv.Atoi(strings.TrimPrefix(line, "ENTRY_COUNT="))
			return emit(map[string]any{"type": "start", "total": n})
		}
		if strings.HasPrefix(line, "PROGRESS\t") {
			parts := strings.Split(line, "\t")
			if len(parts) >= 4 {
				cur, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
				tot, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
				name := strings.TrimSpace(parts[3])
				pct := 0
				if tot > 0 {
					pct = int(float64(cur) / float64(tot) * 100)
				}
				return emit(map[string]any{
					"type":    "progress",
					"current": cur,
					"total":   tot,
					"name":    name,
					"pct":     pct,
				})
			}
			return nil
		}
		entry, ok := parseUsageEntryLine(line)
		if !ok {
			return nil
		}
		entries = append(entries, entry)
		if len(entries) >= usageScanMaxEntries {
			truncated = true
			return io.EOF
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buildUsageResult(dir, entries, totalKiB, truncated), nil
}

func parseUsageEntryLine(line string) (diskUsageEntry, bool) {
	parts := strings.Split(line, "\t")
	if len(parts) < 4 {
		return diskUsageEntry{}, false
	}
	szKiB, _ := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 64)
	p := strings.TrimSpace(parts[1])
	isDir := strings.TrimSpace(parts[2]) == "d"
	name := strings.TrimSpace(parts[3])
	if name == "" {
		name = path.Base(p)
	}
	return diskUsageEntry{
		Name:      name,
		Path:      p,
		SizeBytes: szKiB * 1024,
		IsDir:     isDir,
	}, true
}

func buildUsageResult(dir string, entries []diskUsageEntry, totalKiB uint64, truncated bool) map[string]any {
	totalBytes := totalKiB * 1024
	if totalBytes == 0 && len(entries) > 0 {
		var sum uint64
		for _, e := range entries {
			sum += e.SizeBytes
		}
		totalBytes = sum
	}
	denom := totalBytes
	if denom == 0 {
		denom = 1
	}
	for i := range entries {
		entries[i].Pct = float64(entries[i].SizeBytes) / float64(denom) * 100
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].SizeBytes == entries[j].SizeBytes {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].SizeBytes > entries[j].SizeBytes
	})
	parent := path.Dir(dir)
	if parent == dir {
		parent = ""
	}
	return map[string]any{
		"path":       dir,
		"parent":     parent,
		"totalBytes": totalBytes,
		"entries":    entries,
		"truncated":  truncated,
		"roots":      diskutil.UsageRootsFromRows(nil),
	}
}

func (b *sshBundle) parseUsageOutput(dir, raw string) (map[string]any, error) {
	var totalKiB uint64
	var entries []diskUsageEntry
	truncated := false
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "TOTAL_KB=") {
			v, _ := strconv.ParseUint(strings.TrimPrefix(line, "TOTAL_KB="), 10, 64)
			totalKiB = v
			continue
		}
		if strings.HasPrefix(line, "ENTRY_COUNT=") || strings.HasPrefix(line, "PROGRESS\t") {
			continue
		}
		entry, ok := parseUsageEntryLine(line)
		if !ok {
			continue
		}
		entries = append(entries, entry)
		if len(entries) >= usageScanMaxEntries {
			truncated = true
			break
		}
	}
	return buildUsageResult(dir, entries, totalKiB, truncated), nil
}

func (b *sshBundle) runUsageScript(ctx context.Context, script string) (string, error) {
	if b == nil || b.Sess == nil || b.Sess.SSH == nil {
		return "", fmt.Errorf("sessão SSH indisponível")
	}
	cmd, input, err := b.usageCommand(ctx, script)
	if err != nil {
		return "", err
	}
	out, _, err := b.runSSHCommand(ctx, cmd, input)
	return out, err
}

func (b *sshBundle) runUsageScriptLines(ctx context.Context, script string, onLine func(string) error) error {
	if b == nil || b.Sess == nil || b.Sess.SSH == nil {
		return fmt.Errorf("sessão SSH indisponível")
	}
	cmd, input, err := b.usageCommand(ctx, script)
	if err != nil {
		return err
	}

	type res struct{ err error }
	done := make(chan res, 1)

	go func() {
		sess, err := b.Sess.SSH.NewSession()
		if err != nil {
			done <- res{err}
			return
		}
		defer sess.Close()

		stdout, err := sess.StdoutPipe()
		if err != nil {
			done <- res{err}
			return
		}
		stdin, err := sess.StdinPipe()
		if err != nil {
			done <- res{err}
			return
		}
		if err := sess.Start(cmd); err != nil {
			done <- res{err}
			return
		}
		if input != "" {
			_, _ = io.WriteString(stdin, input+"\n")
		}
		_ = stdin.Close()

		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			if err := onLine(sc.Text()); err != nil {
				_ = sess.Close()
				done <- res{err}
				return
			}
		}
		if err := sc.Err(); err != nil {
			done <- res{err}
			return
		}
		done <- res{sess.Wait()}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case r := <-done:
		return r.err
	}
}

func (b *sshBundle) usageCommand(ctx context.Context, script string) (cmd, input string, err error) {
	b.sudoMu.Lock()
	sudoOn := b.sudoEnabled
	user := b.sudoUser
	pass := b.sudoPass
	b.sudoMu.Unlock()
	if sudoOn && strings.TrimSpace(user) != "" && strings.TrimSpace(pass) != "" {
		if err := b.ensureSudoSession(ctx); err != nil {
			return "", "", err
		}
		return fmt.Sprintf("sudo -S -p '' -u %s sh -lc %s", shellQuote(user), shellQuote(script)), pass, nil
	}
	return "sh -lc " + shellQuote(script), "", nil
}
