package session

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

func parsePrivateKey(pemData []byte, passphrase string) (ssh.Signer, error) {
	if len(pemData) == 0 {
		return nil, fmt.Errorf("chave vazia")
	}
	head := string(pemData)
	if strings.Contains(head, "PuTTY-User-Key-File") {
		return nil, fmt.Errorf("formato PPK não suportado nesta versão; exporte com: puttygen key.ppk -O private-openssh -o key.pem")
	}

	if strings.TrimSpace(passphrase) == "" {
		signer, err := ssh.ParsePrivateKey(pemData)
		if err != nil {
			return nil, fmt.Errorf("parse chave: %w", err)
		}
		return signer, nil
	}
	signer, err := ssh.ParsePrivateKeyWithPassphrase(pemData, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("parse chave (passphrase?): %w", err)
	}
	return signer, nil
}
