package accessauth

import (
	"strings"

	"containerway/internal/fyneprefs"
)

const (
	defaultUser     = "admin"
	defaultPassword = "!q1w2e3r4$"
)

// User conta de acesso local ao aplicativo.
type User struct {
	Username    string       `json:"username"`
	Password    string       `json:"password"`
	DisplayName string       `json:"displayName"`
	Permissions *Permissions `json:"permissions,omitempty"`
}

// LoadUsers devolve contas de acesso (preferências Fyne ou padrão admin).
func LoadUsers() []User {
	list, err := fyneprefs.LoadUsers()
	if err != nil || len(list) == 0 {
		return defaultUsers()
	}
	out := make([]User, len(list))
	for i, u := range list {
		out[i] = User{
			Username:    u.Username,
			Password:    u.Password,
			DisplayName: u.DisplayName,
			Permissions: permsFromPrefs(u.Permissions),
		}
	}
	return out
}

// SaveUsers persiste contas no preferences.json (formato desktop).
func SaveUsers(users []User) error {
	in := make([]fyneprefs.AccessUser, len(users))
	for i, u := range users {
		in[i] = fyneprefs.AccessUser{
			Username:    u.Username,
			Password:    u.Password,
			DisplayName: u.DisplayName,
			Permissions: permsToPrefs(u.Permissions),
		}
	}
	return fyneprefs.SaveUsers(in)
}

func permsFromPrefs(p *fyneprefs.UserPermissions) *Permissions {
	if p == nil {
		return nil
	}
	return &Permissions{
		Screens: append([]string(nil), p.Screens...),
		Actions: append([]string(nil), p.Actions...),
	}
}

func permsToPrefs(p *Permissions) *fyneprefs.UserPermissions {
	if p == nil {
		return nil
	}
	return &fyneprefs.UserPermissions{
		Screens: append([]string(nil), p.Screens...),
		Actions: append([]string(nil), p.Actions...),
	}
}

// PermissionsForUser devolve permissões efetivas de um utilizador persistido.
func PermissionsForUser(username string) Permissions {
	users := LoadUsers()
	key := normalizeUsername(username)
	for _, u := range users {
		if u.Username == key {
			return ResolvePermissions(u.Username, u.Permissions)
		}
	}
	if IsAdminUsername(username) {
		return FullPermissions()
	}
	return DefaultPermissions()
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

// defaultUsers devolve a conta administrador padrão.
func defaultUsers() []User {
	return []User{{
		Username:    defaultUser,
		Password:    defaultPassword,
		DisplayName: "Administrador",
	}}
}

// normalizeUsername normaliza o nome de utilizador para comparação.
func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
