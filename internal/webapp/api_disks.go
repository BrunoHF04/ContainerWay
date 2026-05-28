package webapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const diskProbeScriptWeb = `set +e
printf '%s\n' '===LSBLK_JSON==='
lsblk -J -b 2>/dev/null || printf '%s\n' '{"blockdevices":[]}'
printf '\n%s\n' '===DF==='
df -h -T 2>/dev/null || true
printf '\n%s\n' '===FIM==='
`

type lsblkNodeWeb struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Type     string          `json:"type"`
	Size     json.RawMessage `json:"size"`
	Mount    string          `json:"mountpoint"`
	Children []lsblkNodeWeb  `json:"children"`
}

type lsblkRootWeb struct {
	Blockdevices []lsblkNodeWeb `json:"blockdevices"`
}

// handleDisksSummary devolve visão resumida de discos no host remoto.
func (s *Server) handleDisksSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	out, err := runSSHScript(ctx, b.Sess.SSH, diskProbeScriptWeb)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	lsblkPart, dfPart := splitDiskProbe(out)
	devices := parseLsblkFlat(lsblkPart)
	writeJSON(w, http.StatusOK, map[string]any{
		"devices": devices,
		"dfRaw":   dfPart,
	})
}

// runSSHScript executa script bash no host remoto e devolve stdout.
func runSSHScript(ctx context.Context, client *ssh.Client, script string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	var buf bytes.Buffer
	sess.Stdout = &buf
	sess.Stderr = &buf
	if err := sess.Run(script); err != nil {
		if buf.Len() == 0 {
			return "", err
		}
	}
	return buf.String(), nil
}

// splitDiskProbe separa secções do script de sondagem.
func splitDiskProbe(out string) (lsblkJSON, dfText string) {
	const m1 = "===LSBLK_JSON==="
	const m2 = "===DF==="
	const m3 = "===FIM==="
	i1 := strings.Index(out, m1)
	i2 := strings.Index(out, m2)
	i3 := strings.Index(out, m3)
	if i1 >= 0 && i2 > i1 {
		lsblkJSON = strings.TrimSpace(out[i1+len(m1) : i2])
	}
	if i2 >= 0 {
		end := len(out)
		if i3 > i2 {
			end = i3
		}
		dfText = strings.TrimSpace(out[i2+len(m2) : end])
	}
	return lsblkJSON, dfText
}

// parseLsblkFlat achata árvore lsblk para lista simples.
func parseLsblkFlat(jsonText string) []map[string]any {
	var root lsblkRootWeb
	if err := json.Unmarshal([]byte(jsonText), &root); err != nil {
		return nil
	}
	var out []map[string]any
	var walk func(n lsblkNodeWeb, depth int)
	walk = func(n lsblkNodeWeb, depth int) {
		p := n.Path
		if p == "" {
			p = "/dev/" + n.Name
		}
		out = append(out, map[string]any{
			"name":   n.Name,
			"path":   p,
			"type":   n.Type,
			"size":   string(n.Size),
			"mount":  n.Mount,
			"depth":  depth,
		})
		for _, ch := range n.Children {
			walk(ch, depth+1)
		}
	}
	for _, n := range root.Blockdevices {
		walk(n, 0)
	}
	return out
}
