package policy

import "testing"

func TestForbidInsecureHostKeyEnv(t *testing.T) {
	t.Setenv("CONTAINERWAY_FORBID_INSECURE_HOSTKEY", "1")
	if !ForbidInsecureHostKey() {
		t.Fatal("esperado true com env=1")
	}
}
