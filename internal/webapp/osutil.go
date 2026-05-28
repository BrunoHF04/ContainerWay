package webapp

import (
	"os"
	"runtime"
)

// osUserHome devolve o diretório home do utilizador atual.
func osUserHome() (string, error) {
	if runtime.GOOS == "windows" {
		if h := os.Getenv("USERPROFILE"); h != "" {
			return h, nil
		}
	}
	return os.UserHomeDir()
}
