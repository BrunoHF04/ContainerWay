package webapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"containerway/internal/composeopt"
)

func composeDiscoverCmd(roots string, maxN, depth int) string {
	// Sem aspas simples no script: shellQuote envolve tudo em '…' e quebraria -name '…'.
	script := `
roots=` + shellQuote(roots) + `
max=` + strconv.Itoa(maxN) + `
depth=` + strconv.Itoa(depth) + `
for root in $roots; do
  [ -d "$root" ] || continue
  find "$root" -maxdepth "$depth" -type f \( \
    -name docker-compose.yml -o -name docker-compose.yaml \
    -o -name compose.yml -o -name compose.yaml \
    -o -name "docker-compose*.yml" -o -name "docker-compose*.yaml" \
    -o -name "docker-stack*.yml" -o -name "docker-stack*.yaml" \
    -o -name "*stack*.yml" -o -name "*stack*.yaml" \
    -o -name "*swarm*.yml" -o -name "*swarm*.yaml" \
  \) 2>/dev/null
done | sort -u | head -n "$max"
`
	return "sh -lc " + shellQuote(strings.TrimSpace(script))
}

func composeClassifyCmd(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("for f in")
	for _, p := range paths {
		b.WriteString(" ")
		b.WriteString(shellQuote(p))
	}
	b.WriteString(`; do
  [ -f "$f" ] || continue
  if grep -qE "replicas:|placement:" "$f" 2>/dev/null && grep -q "deploy:" "$f" 2>/dev/null; then
    echo "$f|swarm"
  fi
done`)
	return "sh -lc " + shellQuote(strings.TrimSpace(b.String()))
}

type swarmHostInfo struct {
	Active bool
	Nodes  int
	Stacks []composeopt.StackRef
}

func (b *sshBundle) swarmHostInfo(ctx context.Context) swarmHostInfo {
	out := swarmHostInfo{}
	if b == nil || b.Sess == nil || b.Sess.SSH == nil {
		return out
	}
	script := `
state=$(docker info --format "{{.Swarm.LocalNodeState}}" 2>/dev/null || echo inactive)
nodes=0
if [ "$state" = "active" ] || [ "$state" = "pending" ]; then
  nodes=$(docker node ls -q 2>/dev/null | wc -l | tr -d " ")
fi
echo "SWARM_STATE=$state"
echo "SWARM_NODES=$nodes"
docker stack ls --format "{{.Name}}" 2>/dev/null | while read -r n; do
  [ -n "$n" ] && echo "STACK=$n"
done
`
	raw, _, err := b.runSSHCommand(ctx, "sh -lc "+shellQuote(strings.TrimSpace(script)), "")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SWARM_STATE=") {
			st := strings.TrimPrefix(line, "SWARM_STATE=")
			out.Active = st == "active" || st == "pending"
		}
		if strings.HasPrefix(line, "SWARM_NODES=") {
			out.Nodes, _ = strconv.Atoi(strings.TrimPrefix(line, "SWARM_NODES="))
		}
		if strings.HasPrefix(line, "STACK=") {
			name := strings.TrimPrefix(line, "STACK=")
			if name != "" {
				out.Stacks = append(out.Stacks, composeopt.StackRef{Name: name})
			}
		}
	}
	if out.Active && out.Nodes < 1 {
		out.Nodes = 1
	}
	return out
}

func (s *Server) handleComposeOptDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	roots := strings.TrimSpace(q.Get("roots"))
	if roots == "" {
		roots = "/opt /home /srv /var/www"
	}
	maxN := 50
	if v := strings.TrimSpace(q.Get("max")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			maxN = n
		}
	}
	depth := 8
	if v := strings.TrimSpace(q.Get("depth")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 12 {
			depth = n
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	swarm := b.swarmHostInfo(ctx)
	cmd := composeDiscoverCmd(roots, maxN, depth)
	out, stderr, err := b.runSSHCommand(ctx, cmd, "")
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg != "" {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": msg})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	files := composeopt.ParseDiscoverLines(out)
	if len(files) > 0 && len(files) <= 30 {
		paths := make([]string, len(files))
		for i := range files {
			paths[i] = files[i].Path
		}
		if classifyCmd := composeClassifyCmd(paths); classifyCmd != "" {
			if clsOut, _, clsErr := b.runSSHCommand(ctx, classifyCmd, ""); clsErr == nil {
				files = composeopt.ParseClassifyLines(clsOut, files)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"files":       files,
		"count":       len(files),
		"roots":       roots,
		"max":         maxN,
		"depth":       depth,
		"swarmActive": swarm.Active,
		"swarmNodes":  swarm.Nodes,
		"stacks":      swarm.Stacks,
	})
}

// composeValidateCmd valida YAML no host remoto com docker stack config ou docker compose config.
func composeValidateCmd(content, mode string) string {
	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	script := `
content_b64=` + shellQuote(b64) + `
mode=` + shellQuote(mode) + `
tmp=$(mktemp /tmp/cw-copt.XXXXXX.yml)
echo "$content_b64" | base64 -d > "$tmp" 2>/dev/null || { rm -f "$tmp"; exit 2; }
if [ "$mode" = "swarm" ]; then
  docker stack config -c "$tmp" >/tmp/cw-copt-val.out 2>&1
else
  docker compose -f "$tmp" config >/tmp/cw-copt-val.out 2>&1
fi
ec=$?
if [ $ec -eq 0 ]; then
  rm -f "$tmp" /tmp/cw-copt-val.out
  exit 0
fi
tail -30 /tmp/cw-copt-val.out
rm -f "$tmp" /tmp/cw-copt-val.out
exit 1
`
	return "sh -lc " + shellQuote(strings.TrimSpace(script))
}

func (s *Server) handleComposeOptValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var body struct {
		Content string `json:"content"`
		Mode    string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return
	}
	content := strings.TrimSpace(body.Content)
	if content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content obrigatório"})
		return
	}
	mode := strings.TrimSpace(body.Mode)
	if mode == "" || mode == "auto" {
		mode = composeopt.ModeCompose
	}
	if mode != composeopt.ModeSwarm && mode != composeopt.ModeCompose {
		mode = composeopt.ModeCompose
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	out, stderr, err := b.runSSHCommand(ctx, composeValidateCmd(content, mode), "")
	if err != nil {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = strings.TrimSpace(stderr)
		}
		if msg == "" {
			msg = err.Error()
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": msg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleComposeOptAnalyze(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.composeOptAnalyzePath(w, r)
	case http.MethodPost:
		s.composeOptAnalyzeBody(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

func (s *Server) composeOptAnalyzePath(w http.ResponseWriter, r *http.Request) {
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path obrigatório"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	data, err := readComposeOptFile(ctx, b, path)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	s.composeOptAnalyzeContent(w, r, b, path, string(data), mode)
}

func (s *Server) composeOptAnalyzeBody(w http.ResponseWriter, r *http.Request) {
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Mode    string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return
	}
	path := strings.TrimSpace(body.Path)
	if path == "" {
		path = "compose.yml"
	}
	s.composeOptAnalyzeContent(w, r, b, path, body.Content, body.Mode)
}

func (s *Server) composeOptAnalyzeContent(w http.ResponseWriter, r *http.Request, b *sshBundle, path, content, mode string) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	ov, err := b.runDesktopOverview(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	swarm := b.swarmHostInfo(ctx)
	host := composeopt.FromOverview(
		ov.Hostname, ov.CPUs, ov.MemTotal, ov.MemUsed, ov.MemFree, ov.MemPct, ov.Load1, ov.UpdatedAt,
	)
	opts := composeopt.AnalyzeOptions{
		Mode:        mode,
		SwarmActive: swarm.Active,
		SwarmNodes:  swarm.Nodes,
	}
	if opts.SwarmNodes < 1 && swarm.Active {
		opts.SwarmNodes = 1
	}
	result, err := composeopt.Analyze(path, content, host, opts)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// readComposeOptFile lê YAML do host: SFTP, sudo (se ativo) ou cat via SSH.
func readComposeOptFile(ctx context.Context, b *sshBundle, p string) ([]byte, error) {
	data, err := readRemoteFileForEditor(ctx, b, p, "")
	if err == nil && len(data) > 0 {
		return data, nil
	}
	if err == nil {
		return nil, fmt.Errorf("arquivo vazio")
	}
	b.sudoMu.Lock()
	sudoOn := b.sudoEnabled
	b.sudoMu.Unlock()
	if sudoOn {
		if data, e := b.readHostFileWithSudo(ctx, p); e == nil && len(data) > 0 {
			return data, nil
		}
	}
	inner := "cat " + shellQuote(p) + " 2>/dev/null"
	cmd := "sh -lc " + shellQuote(inner)
	out, stderr, runErr := b.runSSHCommand(ctx, cmd, "")
	if runErr != nil {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = err.Error()
		}
		if sudoOn {
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, fmt.Errorf("%s — ative o sudo no topo da aplicação para ler este arquivo", msg)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, fmt.Errorf("arquivo vazio ou sem permissão de leitura — ative o sudo")
	}
	if len(out) > maxEditorFileBytes {
		return nil, fmt.Errorf("arquivo muito grande (máx. %d MB)", maxEditorFileBytes/(1<<20))
	}
	return []byte(out), nil
}
