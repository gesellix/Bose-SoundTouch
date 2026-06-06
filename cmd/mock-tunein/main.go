// Package main provides a mock TuneIn (radiotime.com) server for testing.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/gesellix/bose-soundtouch/pkg/testutils/tunein"
)

func main() {
	port := flag.Int("port", 8080, "Port to listen on")

	flag.Parse()

	log.Printf("Starting mock TuneIn server on port %d", *port)

	// Plaintext HTTP is intentional: this is a throwaway test mock that only
	// runs on the loopback / CI compose network, never in production.
	// nosemgrep: go.lang.security.audit.net.use-tls.use-tls
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), tunein.NewTuneInHandler()); err != nil {
		log.Fatal(err)
	}
}
