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
	flag.Parse()

	webapp.InitLogging()

	if err := webapp.Run(webapp.Options{
		Addr:         *addr,
		OpenBrowser:  !*noBrowser,
		AutoShutdown: *autoShutdown,
	}); err != nil {
		log.Fatal(err)
	}
}
