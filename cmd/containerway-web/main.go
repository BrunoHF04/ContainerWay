//go:generate go run ../../cmd/iconforge -web

package main

import (
	"flag"
	"log"

	"containerway/internal/webapp"
)

// main inicia o ContainerWay com UI no browser (servidor local + abertura automática).
func main() {
	addr := flag.String("addr", "127.0.0.1:8765", "endereço HTTP (127.0.0.1 = só neste PC)")
	noBrowser := flag.Bool("no-browser", false, "não abrir o navegador automaticamente")
	autoShutdown := flag.Bool("auto-shutdown", true, "encerrar o serviço automaticamente ao fechar o navegador")

	// Flags para VPN Tailscale / Headscale
	tsAuthKey := flag.String("ts-authkey", "", "chave de autenticação da Tailscale/Headscale (env: CONTAINERWAY_TS_AUTHKEY)")
	tsHostname := flag.String("ts-hostname", "containerway-client", "nome do nó na VPN (env: CONTAINERWAY_TS_HOSTNAME)")
	tsSSHPort := flag.Int("ts-ssh-port", 2222, "porta SSH física exposta na VPN, 0 para desativar (env: CONTAINERWAY_TS_SSH_PORT)")
	tsControlURL := flag.String("ts-control-url", "", "URL do servidor Headscale (opcional, env: CONTAINERWAY_TS_CONTROL_URL)")

	// Flags adicionais de gerenciamento Headscale (para o servidor Administrador)
	hsURL := flag.String("hs-url", "", "URL da VPS Headscale (servidor admin, env: CONTAINERWAY_HS_URL)")
	hsAPIKey := flag.String("hs-apikey", "", "chave de API do Headscale (servidor admin, env: CONTAINERWAY_HS_APIKEY)")

	flag.Parse()

	webapp.InitLogging()

	if err := webapp.Run(webapp.Options{
		Addr:         *addr,
		OpenBrowser:  !*noBrowser,
		AutoShutdown: *autoShutdown,
		TSAuthKey:    *tsAuthKey,
		TSHostname:   *tsHostname,
		TSSSHPort:    *tsSSHPort,
		TSControlURL: *tsControlURL,
		HSURL:        *hsURL,
		HSApiKey:     *hsAPIKey,
	}); err != nil {
		log.Fatal(err)
	}
}
