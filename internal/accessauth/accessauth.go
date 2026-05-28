package accessauth

import (
	"encoding/json"
	"os"
	"strings"

	"containerway/internal/configdir"
)

const (
	defaultUser     = "admin"
	defaultPassword = "!q1w2e3r4$"
	fyneUsersKey    = "access.users"
)

// User conta de acesso local ao aplicativo.
type User struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

// LoadUsers devolve contas de acesso (preferências Fyne ou padrão admin).
func LoadUsers() []User {
	if users := loadFromFynePreferences(); len(users) > 0 {
		return ensureAdmin(users)
	}
	return defaultUsers()
}

// Authenticate valida utilizador e senha de acesso local.
func Authenticate(username, password string) (User, bool) {
	key := normalizeUsername(username)
	pass := strings.TrimSpace(password)
	if key == "" || pass == "" {
		return User{}, false
	}
	for _, u := range LoadUsers() {
		if u.Username == key && u.Password == pass {
			return u, true
		}
	}
	return User{}, false
}

// loadFromFynePreferences lê access.users do preferences.json da app desktop.
func loadFromFynePreferences() []User {
	path, err := configdir.FynePreferencesPath()
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var prefs map[string]json.RawMessage
	if err := json.Unmarshal(b, &prefs); err != nil {
		return nil
	}
	raw, ok := prefs[fyneUsersKey]
	if !ok || len(raw) == 0 {
		return nil
	}
	var stored []struct {
		Username    string `json:"Username"`
		Password    string `json:"Password"`
		DisplayName string `json:"DisplayName"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]User, 0, len(stored))
	for _, u := range stored {
		name := normalizeUsername(u.Username)
		pass := strings.TrimSpace(u.Password)
		if name == "" || pass == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		display := strings.TrimSpace(u.DisplayName)
		if display == "" {
			display = u.Username
		}
		out = append(out, User{Username: name, Password: pass, DisplayName: display})
	}
	return out
}

// defaultUsers devolve a conta administrador padrão.
func defaultUsers() []User {
	return []User{{
		Username:    defaultUser,
		Password:    defaultPassword,
		DisplayName: "Administrador",
	}}
}

// ensureAdmin garante que admin existe com a senha padrão do produto.
func ensureAdmin(users []User) []User {
	adminName := normalizeUsername(defaultUser)
	seen := false
	out := make([]User, 0, len(users)+1)
	for _, u := range users {
		if u.Username == adminName {
			seen = true
			u.Password = defaultPassword
			if strings.TrimSpace(u.DisplayName) == "" {
				u.DisplayName = "Administrador"
			}
		}
		out = append(out, u)
	}
	if !seen {
		out = append(out, defaultUsers()...)
	}
	return out
}

// normalizeUsername normaliza o nome de utilizador para comparação.
func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
