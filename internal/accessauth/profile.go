package accessauth

import (
	"errors"
	"strings"
)

var (
	errWrongPassword    = errors.New("senha atual incorreta")
	errDisplayNameEmpty = errors.New("nome não pode ficar vazio")
)

// UpdateProfile altera nome e/ou senha do utilizador autenticado.
func UpdateProfile(username, displayName, currentPassword, newPassword string) error {
	key := normalizeUsername(username)
	users := LoadUsers()
	idx := -1
	for i, u := range users {
		if u.Username == key {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errors.New("utilizador não encontrado")
	}
	if dn := strings.TrimSpace(displayName); dn != "" {
		users[idx].DisplayName = dn
	}
	if np := strings.TrimSpace(newPassword); np != "" {
		cur := strings.TrimSpace(currentPassword)
		if cur == "" || users[idx].Password != cur {
			return errWrongPassword
		}
		users[idx].Password = np
	}
	if strings.TrimSpace(users[idx].DisplayName) == "" {
		return errDisplayNameEmpty
	}
	return SaveUsers(users)
}
