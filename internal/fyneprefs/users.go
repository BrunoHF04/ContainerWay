package fyneprefs

import (
	"encoding/json"
	"strings"
)

const (
	KeyAccessUsers = "access.users"
	defaultUser    = "admin"
	defaultPass    = "!q1w2e3r4$"
)

// AccessUser conta de acesso local.
type AccessUser struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// LoadUsers devolve contas persistidas (ou admin padrão).
func LoadUsers() ([]AccessUser, error) {
	m, err := loadMap()
	if err != nil {
		return nil, err
	}
	users, err := parseUsersRaw(m[KeyAccessUsers])
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return defaultUsers(), nil
	}
	return ensureAdmin(users), nil
}

// SaveUsers grava contas no preferences.json (formato compatível com a app desktop).
func SaveUsers(users []AccessUser) error {
	normalized := normalizeUsers(users)
	m, err := loadMap()
	if err != nil {
		return err
	}
	buf, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	m[KeyAccessUsers] = json.RawMessage(buf)
	return saveMap(m)
}

// parseUsersRaw interpreta access.users (array JSON ou string JSON com array).
func parseUsersRaw(raw json.RawMessage) ([]AccessUser, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	// Valor pode ser string JSON com o array dentro.
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil && strings.TrimSpace(asString) != "" {
		return parseUsersBytes([]byte(asString))
	}
	return parseUsersBytes(raw)
}

func parseUsersBytes(bb []byte) ([]AccessUser, error) {
	if len(strings.TrimSpace(string(bb))) == 0 {
		return nil, nil
	}
	var flex []map[string]string
	if err := json.Unmarshal(bb, &flex); err != nil {
		return nil, err
	}
	out := make([]AccessUser, 0, len(flex))
	seen := map[string]struct{}{}
	for _, row := range flex {
		name := normalizeUsername(firstNonEmpty(row, "username", "Username"))
		pass := strings.TrimSpace(firstNonEmpty(row, "password", "Password"))
		if name == "" || pass == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		display := strings.TrimSpace(firstNonEmpty(row, "display_name", "displayName", "DisplayName"))
		if display == "" {
			display = name
		}
		out = append(out, AccessUser{Username: name, Password: pass, DisplayName: display})
	}
	return out, nil
}

func firstNonEmpty(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(m[k]); v != "" {
			return v
		}
	}
	return ""
}

func normalizeUsers(users []AccessUser) []AccessUser {
	seen := map[string]struct{}{}
	out := make([]AccessUser, 0, len(users))
	for _, u := range users {
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
			display = name
		}
		out = append(out, AccessUser{Username: name, Password: pass, DisplayName: display})
	}
	if _, ok := seen[normalizeUsername(defaultUser)]; !ok {
		out = append(out, defaultUsers()...)
	}
	return ensureAdmin(out)
}

func defaultUsers() []AccessUser {
	return []AccessUser{{
		Username:    defaultUser,
		Password:    defaultPass,
		DisplayName: "Administrador",
	}}
}

func ensureAdmin(users []AccessUser) []AccessUser {
	adminName := normalizeUsername(defaultUser)
	seen := false
	out := make([]AccessUser, 0, len(users)+1)
	for _, u := range users {
		if u.Username == adminName {
			seen = true
			u.Password = defaultPass
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

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
