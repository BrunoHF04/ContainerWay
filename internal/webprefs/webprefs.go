package webprefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"containerway/internal/configdir"
)

// Settings preferências exclusivas da interface web (web.json).
type Settings struct {
	Policies      Policies          `json:"policies"`
	Notifications Notifications     `json:"notifications"`
	LoginFailures map[string]int    `json:"loginFailures,omitempty"`
}

// Policies políticas de sessão e acesso web.
type Policies struct {
	SessionIdleMinutes  int      `json:"sessionIdleMinutes"`
	MaxLoginAttempts    int      `json:"maxLoginAttempts"`
	ForcePasswordChange []string `json:"forcePasswordChange"`
}

// Notifications eventos que podem gerar e-mail (quando SMTP ativo).
type Notifications struct {
	AutomationFail bool `json:"automationFail"`
	DockerStopped  bool `json:"dockerStopped"`
	DiskSpaceLow   bool `json:"diskSpaceLow"`
	LoginFailure   bool `json:"loginFailure"`
}

var prefsMu sync.Mutex

// Default devolve valores padrão.
func Default() Settings {
	return Settings{
		Policies: Policies{
			SessionIdleMinutes:  0,
			MaxLoginAttempts:    0,
			ForcePasswordChange: nil,
		},
		Notifications: Notifications{
			AutomationFail: true,
			DockerStopped:  false,
			DiskSpaceLow:   true,
			LoginFailure:   true,
		},
	}
}

// Path devolve o caminho de web.json.
func Path() (string, error) {
	return prefsPath()
}

func prefsPath() (string, error) {
	root, err := configdir.Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "web.json"), nil
}

// Load lê web.json ou devolve padrões.
func Load() (Settings, error) {
	p, err := prefsPath()
	if err != nil {
		return Default(), err
	}
	bb, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Default(), err
	}
	var s Settings
	if err := json.Unmarshal(bb, &s); err != nil {
		return Default(), err
	}
	return mergeDefaults(s), nil
}

// Save grava web.json.
func Save(s Settings) error {
	prefsMu.Lock()
	defer prefsMu.Unlock()
	p, err := prefsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	s = mergeDefaults(s)
	bb, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, bb, 0o644)
}

func mergeDefaults(s Settings) Settings {
	d := Default()
	if s.Policies.SessionIdleMinutes < 0 {
		s.Policies.SessionIdleMinutes = 0
	}
	if s.Policies.MaxLoginAttempts < 0 {
		s.Policies.MaxLoginAttempts = 0
	}
	if s.Notifications == (Notifications{}) {
		s.Notifications = d.Notifications
	}
	return s
}

// MustChangePassword indica se o utilizador deve alterar a senha no próximo acesso.
func MustChangePassword(username string) bool {
	s, err := Load()
	if err != nil {
		return false
	}
	key := normalizeUser(username)
	for _, u := range s.Policies.ForcePasswordChange {
		if normalizeUser(u) == key {
			return true
		}
	}
	return false
}

// RecordLoginFailure incrementa falhas de login do utilizador.
func RecordLoginFailure(username string) (int, error) {
	s, err := Load()
	if err != nil {
		return 0, err
	}
	if s.LoginFailures == nil {
		s.LoginFailures = map[string]int{}
	}
	key := normalizeUser(username)
	s.LoginFailures[key]++
	n := s.LoginFailures[key]
	if err := Save(s); err != nil {
		return n, err
	}
	return n, nil
}

// ResetLoginFailures zera contador após login bem-sucedido.
func ResetLoginFailures(username string) error {
	s, err := Load()
	if err != nil {
		return err
	}
	if len(s.LoginFailures) == 0 {
		return nil
	}
	key := normalizeUser(username)
	delete(s.LoginFailures, key)
	return Save(s)
}

// LoginFailuresFor devolve tentativas falhadas registadas.
func LoginFailuresFor(username string) int {
	s, err := Load()
	if err != nil || s.LoginFailures == nil {
		return 0
	}
	return s.LoginFailures[normalizeUser(username)]
}

// ClearPasswordChangeRequirement remove utilizador da lista de troca obrigatória.
func ClearPasswordChangeRequirement(username string) error {
	s, err := Load()
	if err != nil {
		return err
	}
	key := normalizeUser(username)
	out := make([]string, 0, len(s.Policies.ForcePasswordChange))
	for _, u := range s.Policies.ForcePasswordChange {
		if normalizeUser(u) != key {
			out = append(out, u)
		}
	}
	s.Policies.ForcePasswordChange = out
	return Save(s)
}

func normalizeUser(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
