package webapp

import (
	"encoding/json"
	"net/http"
	"strings"

	"containerway/internal/accessauth"
	"containerway/internal/fyneprefs"
	"containerway/internal/mailnotify"
)

// requireWebAdmin exige sessão web de administrador.
func (s *Server) requireWebAdmin(w http.ResponseWriter, r *http.Request) (webSession, bool) {
	_, ws, ok := s.requireWebAuth(w, r)
	if !ok {
		return webSession{}, false
	}
	if !isAdminUser(ws.Username) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "apenas administrador"})
		return webSession{}, false
	}
	return ws, true
}

// handleAdminUsers GET lista utilizadores; PUT grava conjunto.
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireWebAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := fyneprefs.LoadUsers()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		out := make([]map[string]any, 0, len(users))
		for _, u := range users {
			var stored *accessauth.Permissions
			if u.Permissions != nil {
				stored = &accessauth.Permissions{
					Screens: append([]string(nil), u.Permissions.Screens...),
					Actions: append([]string(nil), u.Permissions.Actions...),
				}
			}
			perms := accessauth.ResolvePermissions(u.Username, stored)
			out = append(out, map[string]any{
				"username":    u.Username,
				"displayName": u.DisplayName,
				"isAdmin":     isAdminUser(u.Username),
				"permissions": map[string]any{
					"screens": perms.Screens,
					"actions": perms.Actions,
				},
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": out})
	case http.MethodPut:
		var body struct {
			Users []struct {
				Username    string `json:"username"`
				DisplayName string `json:"displayName"`
				Password    string `json:"password"`
				Permissions *struct {
					Screens []string `json:"screens"`
					Actions []string `json:"actions"`
				} `json:"permissions"`
			} `json:"users"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}
		existing, err := fyneprefs.LoadUsers()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		byName := map[string]fyneprefs.AccessUser{}
		for _, u := range existing {
			byName[u.Username] = u
		}
		saved := make([]accessauth.User, 0, len(body.Users))
		for _, u := range body.Users {
			name := strings.ToLower(strings.TrimSpace(u.Username))
			if name == "" {
				continue
			}
			pass := strings.TrimSpace(u.Password)
			if pass == "" {
				if old, ok := byName[name]; ok {
					pass = old.Password
				}
			}
			if pass == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "senha obrigatória para novo usuário: " + name})
				return
			}
			display := strings.TrimSpace(u.DisplayName)
			if display == "" {
				display = name
			}
			entry := accessauth.User{
				Username:    name,
				Password:    pass,
				DisplayName: display,
			}
			if !isAdminUser(name) && u.Permissions != nil {
				entry.Permissions = accessauth.SanitizePermissions(&accessauth.Permissions{
					Screens: append([]string(nil), u.Permissions.Screens...),
					Actions: append([]string(nil), u.Permissions.Actions...),
				})
			}
			saved = append(saved, entry)
		}
		if len(saved) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lista de usuários vazia"})
			return
		}
		if err := accessauth.SaveUsers(saved); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

// handleAdminMail GET/PUT configuração SMTP; POST teste de envio.
func (s *Server) handleAdminMail(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireWebAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := fyneprefs.LoadMail()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, mailSettingsResponse(cfg))
	case http.MethodPut:
		var body mailSettingsBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}
		prev, err := fyneprefs.LoadMail()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		cfg, err := body.toSettings(prev)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := fyneprefs.SaveMail(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, mailSettingsResponse(cfg))
	case http.MethodPost:
		var body struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
			return
		}
		cfg, err := fyneprefs.LoadMail()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		subject := "ContainerWay — teste de e-mail"
		msg := "Mensagem de teste enviada pela interface web ContainerWay."
		switch strings.ToLower(strings.TrimSpace(body.Mode)) {
		case "self":
			if err := cfg.SendTestToSelf(subject, msg); err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
		case "recipients", "all", "":
			if err := cfg.Send(subject, msg); err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode deve ser self ou recipients"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "enviado"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

type mailSettingsBody struct {
	Enabled     bool     `json:"enabled"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	User        string   `json:"user"`
	Password    string   `json:"password"`
	From        string   `json:"from"`
	Recipients  []string `json:"recipients"`
	RecipientsT string   `json:"recipientsText"`
}

func (b mailSettingsBody) toSettings(prev mailnotify.Settings) (mailnotify.Settings, error) {
	port := b.Port
	if port <= 0 {
		port = prev.Port
	}
	if port <= 0 {
		port = 587
	}
	pass := strings.TrimSpace(b.Password)
	if pass == "" {
		pass = prev.Password
	}
	rec := mailnotify.NormalizeRecipients(b.Recipients)
	if strings.TrimSpace(b.RecipientsT) != "" {
		var parts []string
		for _, seg := range strings.Split(b.RecipientsT, ",") {
			if t := strings.TrimSpace(seg); t != "" {
				parts = append(parts, t)
			}
		}
		rec = mailnotify.NormalizeRecipients(parts)
	}
	if len(rec) == 0 && len(b.Recipients) == 0 && b.RecipientsT == "" {
		rec = prev.Recipients
	}
	return mailnotify.Settings{
		Enabled:    b.Enabled,
		Host:       strings.TrimSpace(b.Host),
		Port:       port,
		User:       strings.TrimSpace(b.User),
		Password:   pass,
		From:       strings.TrimSpace(b.From),
		Recipients: rec,
	}, nil
}

func mailSettingsResponse(cfg mailnotify.Settings) map[string]any {
	return map[string]any{
		"enabled":     cfg.Enabled,
		"host":        cfg.Host,
		"port":        cfg.Port,
		"user":        cfg.User,
		"from":        cfg.From,
		"recipients":  cfg.Recipients,
		"hasPassword": strings.TrimSpace(cfg.Password) != "",
		"valid":       cfg.Valid(),
		"validTransport": cfg.ValidTransport(),
	}
}
