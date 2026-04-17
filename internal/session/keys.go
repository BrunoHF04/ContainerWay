package session

import (
	"fmt"
	"strings"

	"github.com/kayrus/putty"
	"golang.org/x/crypto/ssh"
)

func parsePrivateKey(pemData []byte, passphrase string) (ssh.Signer, error) {
	if len(pemData) == 0 {
		return nil, fmt.Errorf("chave vazia")
	}
	head := string(pemData)
	if strings.Contains(head, "PuTTY-User-Key-File") {
		pk, err := putty.New(pemData)
		if err != nil {
			return nil, fmt.Errorf("ppk: %w", err)
		}
		raw, err := pk.ParseRawPrivateKey([]byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("ppk (passphrase?): %w", err)
		}
		signer, err := ssh.NewSignerFromKey(raw)
		if err != nil {
			return nil, fmt.Errorf("ppk signer: %w", err)
		}
		return signer, nil
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
