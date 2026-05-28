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
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
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
		}
	}
	return fyneprefs.SaveUsers(in)
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
