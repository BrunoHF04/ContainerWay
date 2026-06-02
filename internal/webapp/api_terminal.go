package webapp

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
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

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm", 120, 40, modes); err != nil {
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
	if err := sess.Shell(); err != nil {
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
