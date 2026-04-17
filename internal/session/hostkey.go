package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func buildHostKeyCallback(c Credentials) (ssh.HostKeyCallback, error) {
	if c.InsecureHostKey {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	var files []string
	for _, f := range c.KnownHostsFiles {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		files = append(files, filepath.Clean(f))
	}
	if len(files) == 0 {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			def := filepath.Join(home, ".ssh", "known_hosts")
			if st, err := os.Stat(def); err == nil && !st.IsDir() {
				files = []string{def}
			}
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("known_hosts: informe um ou mais caminhos (separados por |) ou marque \"Ignorar chave de host SSH\"")
	}
	return knownhosts.New(files...)
}
