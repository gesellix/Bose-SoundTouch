// Package main provides a mock Amazon LWA server for testing purposes.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/gesellix/bose-soundtouch/pkg/testutils/amazon"
)

func main() {
	port := flag.Int("port", 8080, "Port to listen on")

	flag.Parse()

	log.Printf("Starting mock Amazon LWA server on port %d", *port)

	// Plaintext HTTP is intentional: this is a throwaway test mock that only
	// runs on the loopback / CI compose network, never in production.
	// nosemgrep: go.lang.security.audit.net.use-tls.use-tls
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), amazon.NewAmazonHandler()); err != nil {
		log.Fatal(err)
	}
}
