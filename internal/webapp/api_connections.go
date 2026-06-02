package webapp

import (
	"encoding/json"
	"net/http"
	"strings"

	"containerway/internal/connectcfg"
)

// handleConnections gere perfis SSH guardados (connections.json).
func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.requireWebAuth(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleConnectionsGet(w, r)
	case http.MethodPost:
		s.handleConnectionsSave(w, r)
	case http.MethodDelete:
		s.handleConnectionsDelete(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

func (s *Server) handleConnectionsGet(w http.ResponseWriter, r *http.Request) {
	list, err := connectcfg.LoadAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name != "" {
		c, ok := connectcfg.FindByName(list, name)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "perfil não encontrado"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"profile": connectionDetailJSON(c)})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, connectionSummaryJSON(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
}

func (s *Server) handleConnectionsSave(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            string `json:"name"`
		Host            string `json:"host"`
		User            string `json:"user"`
		Password        string `json:"password"`
		KeyPath         string `json:"keyPath"`
		KeyPass         string `json:"keyPass"`
		KnownHosts      string `json:"knownHosts"`
		InsecureHostKey bool   `json:"insecureHostKey"`
		DockerSocket    string `json:"dockerSocket"`
		SavePassword    bool   `json:"savePassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "corpo inválido"})
		return
	}
	name := strings.TrimSpace(body.Name)
	host := strings.TrimSpace(body.Host)
	user := strings.TrimSpace(body.User)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nome do perfil obrigatório"})
		return
	}
	if host == "" || user == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host e usuário obrigatórios"})
		return
	}
	incoming := connectcfg.SavedConnection{
		Name:            name,
		Host:            host,
		User:            user,
		KeyPath:         strings.TrimSpace(body.KeyPath),
		KnownHosts:      strings.TrimSpace(body.KnownHosts),
		InsecureHostKey: body.InsecureHostKey,
		DockerSocket:    strings.TrimSpace(body.DockerSocket),
	}
	if body.SavePassword {
		incoming.Password = body.Password
		incoming.KeyPass = body.KeyPass
	}
	list, err := connectcfg.LoadAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if existing, ok := connectcfg.FindByName(list, name); ok && !body.SavePassword {
		incoming.Password = existing.Password
		incoming.KeyPass = existing.KeyPass
	} else if body.SavePassword {
		incoming.Password = body.Password
		incoming.KeyPass = body.KeyPass
	}
	list = connectcfg.Upsert(list, incoming)
	if err := connectcfg.Save(list); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"profile": connectionSummaryJSON(incoming),
	})
}

func (s *Server) handleConnectionsDelete(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name obrigatório"})
		return
	}
	list, err := connectcfg.LoadAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if _, ok := connectcfg.FindByName(list, name); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "perfil não encontrado"})
		return
	}
	list = connectcfg.RemoveByName(list, name)
	if err := connectcfg.Save(list); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func connectionSummaryJSON(c connectcfg.SavedConnection) map[string]any {
	return map[string]any{
		"name":            c.Name,
		"host":            c.Host,
		"user":            c.User,
		"hasPassword":     strings.TrimSpace(c.Password) != "",
		"hasKey":          strings.TrimSpace(c.KeyPath) != "",
		"insecureHostKey": c.InsecureHostKey,
		"dockerSocket":    c.DockerSocket,
	}
}

func connectionDetailJSON(c connectcfg.SavedConnection) map[string]any {
	m := connectionSummaryJSON(c)
	m["password"] = c.Password
	m["keyPath"] = c.KeyPath
	m["keyPass"] = c.KeyPass
	m["knownHosts"] = c.KnownHosts
	return m
}
