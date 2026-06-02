package webapp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"containerway/internal/termbanner"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const (
	defaultTermCols = 80
	defaultTermRows = 24
	maxTermCols     = 500
	maxTermRows     = 200
)

type wsTermResize struct {
	Op   string `json:"op"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func clampTermSize(cols, rows int) (int, int) {
	if cols < 10 {
		cols = defaultTermCols
	}
	if cols > maxTermCols {
		cols = maxTermCols
	}
	if rows < 4 {
		rows = defaultTermRows
	}
	if rows > maxTermRows {
		rows = maxTermRows
	}
	return cols, rows
}

func termSizeFromQuery(r *http.Request) (cols, rows int) {
	cols = defaultTermCols
	rows = defaultTermRows
	if v, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("cols"))); err == nil {
		cols = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("rows"))); err == nil {
		rows = v
	}
	return clampTermSize(cols, rows)
}

func parseTermResizeMsg(msg []byte) (cols, rows int, ok bool) {
	var ctrl wsTermResize
	if err := json.Unmarshal(msg, &ctrl); err != nil || ctrl.Op != "resize" {
		return 0, 0, false
	}
	cols, rows = clampTermSize(ctrl.Cols, ctrl.Rows)
	return cols, rows, true
}

// handleTerminalWS abre sessão de terminal SSH interativo via WebSocket.
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	tok, ok := sessionTokenFromRequest(r)
	if !ok {
		http.Error(w, "não autenticado", http.StatusUnauthorized)
		return
	}
	ws, ok := s.store.getWebToken(tok)
	if !ok {
		http.Error(w, "sessão expirada", http.StatusUnauthorized)
		return
	}
	if !userHasRequestPermission(ws.Username, r.Method, r.URL.Path) {
		http.Error(w, "sem permissão", http.StatusForbidden)
		return
	}
	b := s.store.getSSH(tok)
	if b == nil || b.Sess == nil {
		http.Error(w, "SSH offline", http.StatusPreconditionFailed)
		return
	}
	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sess, err := b.Sess.SSH.NewSession()
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("erro ao abrir sessão SSH\r\n"))
		return
	}
	defer sess.Close()

	cols, rows := termSizeFromQuery(r)

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("PTY indisponível\r\n"))
		return
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		return
	}
	host := strings.TrimSpace(b.Host)
	if host == "" && b.Sess != nil {
		host = b.Sess.HostAddr()
	}
	cwd := strings.TrimSpace(r.URL.Query().Get("cwd"))
	lang := termbanner.NormalizeLang(r.URL.Query().Get("lang"))
	startCmd := termbanner.ShellStart(cwd, host, lang)
	if err := sess.Start(startCmd); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("erro ao iniciar shell\r\n"))
		return
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				_ = conn.WriteMessage(websocket.TextMessage, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				_ = conn.WriteMessage(websocket.TextMessage, buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				_ = sess.Signal(ssh.SIGINT)
				return
			}
			if len(msg) > 0 && msg[0] == '{' {
				if c, r, ok := parseTermResizeMsg(msg); ok {
					_ = sess.WindowChange(r, c)
					continue
				}
			}
			if _, err := stdin.Write(msg); err != nil {
				return
			}
		}
	}()

	_ = sess.Wait()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}
