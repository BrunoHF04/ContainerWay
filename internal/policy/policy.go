// Package policy lê restrições opcionais (ambiente ou ficheiro local) para ambientes corporativos.
package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const envForbidInsecure = "CONTAINERWAY_FORBID_INSECURE_HOSTKEY"

type filePolicy struct {
	ForbidInsecureHostKey bool `json:"forbidInsecureHostKey"`
}

// ForbidInsecureHostKey devolve true se a política exige known_hosts (sem ignorar chave de host).
// Ordem: variável de ambiente CONTAINERWAY_FORBID_INSECURE_HOSTKEY=1; depois ficheiro policy.json.
func ForbidInsecureHostKey() bool {
	if v := strings.TrimSpace(os.Getenv(envForbidInsecure)); v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") {
		return true
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	p := filepath.Join(dir, "ContainerWay", "policy.json")
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	var fp filePolicy
	if json.Unmarshal(b, &fp) != nil {
		return false
	}
	return fp.ForbidInsecureHostKey
}
