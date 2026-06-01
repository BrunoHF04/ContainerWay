package webapp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/gorilla/websocket"
)

var dockerExecUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleDockerExecWS abre shell interativo no contêiner via WebSocket.
func (s *Server) handleDockerExecWS(w http.ResponseWriter, r *http.Request) {
	tok, ok := sessionTokenFromRequest(r)
	if !ok {
		http.Error(w, "não autenticado", http.StatusUnauthorized)
		return
	}
	if _, ok := s.store.getWebToken(tok); !ok {
		http.Error(w, "sessão expirada", http.StatusUnauthorized)
		return
	}
	b := s.store.getSSH(tok)
	if b == nil || b.Sess == nil || b.Sess.Docker == nil {
		http.Error(w, "Docker offline", http.StatusPreconditionFailed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Error(w, "id obrigatório", http.StatusBadRequest)
		return
	}
	shell := strings.TrimSpace(r.URL.Query().Get("shell"))
	if shell == "" {
		shell = "/bin/sh"
	}

	conn, err := dockerExecUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	fullID, err := resolveContainerID(ctx, b.Sess.Docker, id)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("erro: "+err.Error()+"\r\n"))
		return
	}

	execCfg := dcontainer.ExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          []string{shell},
	}
	created, err := b.Sess.Docker.ContainerExecCreate(ctx, fullID, execCfg)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("erro ao criar exec: "+err.Error()+"\r\n"))
		return
	}

	attach, err := b.Sess.Docker.ContainerExecAttach(ctx, created.ID, dcontainer.ExecStartOptions{Tty: true})
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("erro ao ligar exec: "+err.Error()+"\r\n"))
		return
	}
	defer attach.Close()

	_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[32mConsola do contêiner ligada.\x1b[0m\r\n"))

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := attach.Reader.Read(buf)
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
				cancel()
				return
			}
			if len(msg) == 0 {
				continue
			}
			if _, err := attach.Conn.Write(msg); err != nil {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
	case <-time.After(24 * time.Hour):
	}
	_, _ = io.Copy(io.Discard, attach.Reader)
}
