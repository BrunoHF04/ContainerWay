package accessauth

import "testing"

func TestAuthenticateDefaultAdmin(t *testing.T) {
	u, ok := Authenticate("admin", defaultPassword)
	if !ok {
		t.Fatal("esperava autenticação do admin padrão")
	}
	if u.Username != "admin" {
		t.Fatalf("username=%q", u.Username)
	}
}

func TestAuthenticateInvalid(t *testing.T) {
	if _, ok := Authenticate("admin", "wrong"); ok {
		t.Fatal("não deveria autenticar senha errada")
	}
}
