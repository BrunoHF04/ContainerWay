package main

import (
	"flag"
	"log"

	"containerway/internal/webapp"
)

// main inicia o servidor web local do ContainerWay.
func main() {
	addr := flag.String("addr", "127.0.0.1:8765", "endereço HTTP (use 127.0.0.1 para acesso só local)")
	flag.Parse()
	if err := webapp.Run(webapp.Options{Addr: *addr}); err != nil {
		log.Fatal(err)
	}
}
