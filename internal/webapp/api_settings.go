package webapp

import (
	"encoding/json"
	"net/http"
	"runtime"
	"strings"

	"containerway/internal/accessauth"
	"containerway/internal/configdir"
	"containerway/internal/webprefs"
)

// handleAuthMe GET perfil; PATCH nome/senha.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleAuthMeGet(w, r)
	case http.MethodPatch, http.MethodPut:
		s.handleAuthMePatch(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

func (s *Server) handleAuthMeGet(w http.ResponseWriter, r *http.Request) {
	tok, ok := sessionTokenFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	ws, ok := s.store.getWebToken(tok)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":       true,
		"username":            ws.Username,
		"displayName":         ws.DisplayName,
		"isAdmin":             isAdminUser(ws.Username),
		"permissions":         permissionsPayload(ws.Username),
		"mustChangePassword":  webprefs.MustChangePassword(ws.Username),
	})
}

func (s *Server) handleAuthMePatch(w http.ResponseWriter, r *http.Request) {
	tok, ws, ok := s.requireWebAuth(w, r)
	if !ok {
		return
	}
	var body struct {
		DisplayName     string `json:"displayName"`
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	if err := accessauth.UpdateProfile(ws.Username, body.DisplayName, body.CurrentPassword, body.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.NewPassword) != "" {
		_ = webprefs.ClearPasswordChangeRequirement(ws.Username)
	}
	users := accessauth.LoadUsers()
	display := ws.DisplayName
	for _, u := range users {
		if u.Username == ws.Username {
			display = u.DisplayName
			break
		}
	}
	s.store.mu.Lock()
	if cur, ok := s.store.web[tok]; ok {
		cur.DisplayName = display
		s.store.web[tok] = cur
	}
	s.store.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":             "ok",
		"displayName":        display,
		"mustChangePassword": webprefs.MustChangePassword(ws.Username),
	})
}

// handleAdminWebSettings GET/PUT políticas e notificações web.
func (s *Server) handleAdminWebSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireWebAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := webprefs.Load()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, webSettingsResponse(cfg))
	case http.MethodPut:
		var body webSettingsBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}
		prev, err := webprefs.Load()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		cfg := body.merge(prev)
		if err := webprefs.Save(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, webSettingsResponse(cfg))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

type webSettingsBody struct {
	Policies      *webprefs.Policies      `json:"policies"`
	Notifications *webprefs.Notifications `json:"notifications"`
}

func (b webSettingsBody) merge(prev webprefs.Settings) webprefs.Settings {
	out := prev
	if b.Policies != nil {
		out.Policies = *b.Policies
	}
	if b.Notifications != nil {
		out.Notifications = *b.Notifications
	}
	return out
}

func webSettingsResponse(cfg webprefs.Settings) map[string]any {
	return map[string]any{
		"policies":      cfg.Policies,
		"notifications": cfg.Notifications,
	}
}

// handleSettingsDiagnostics GET informação de diagnóstico (autenticado).
func (s *Server) handleSettingsDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, ws, ok := s.requireWebAuth(w, r)
	if !ok {
		return
	}
	cfgRoot, _ := configdir.Root()
	fynePath, _ := configdir.FynePreferencesPath()
	webPath, _ := webprefsPath()
	logPath, _ := logFilePath()
	b := s.store.getSSH(sessionTokenFromRequestSilent(r))
	sshConnected := b != nil && b.Sess != nil
	writeJSON(w, http.StatusOK, map[string]any{
		"product":      "ContainerWay Web",
		"version":      Version,
		"goos":         runtime.GOOS,
		"goarch":       runtime.GOARCH,
		"user":         ws.Username,
		"displayName":  ws.DisplayName,
		"isAdmin":      isAdminUser(ws.Username),
		"permissions":  permissionsPayload(ws.Username),
		"sshConnected": sshConnected,
		"paths": map[string]string{
			"config":      cfgRoot,
			"preferences": fynePath,
			"webSettings": webPath,
			"webLog":      logPath,
		},
	})
}

func sessionTokenFromRequestSilent(r *http.Request) string {
	tok, _ := sessionTokenFromRequest(r)
	return tok
}

func webprefsPath() (string, error) {
	return webprefs.Path()
}
