//go:build ignore

// decrypt-diagnostic.go — maintainer-side helper to decrypt a diagnostic report.
//
// The decrypted content is a .tar.gz archive containing:
//   - diagnostic.json   structured health/device summary
//   - datastore/...     raw XML files from the sender's datastore
//
// Usage:
//
//	# Decrypt to stdout and extract in one step:
//	go run scripts/decrypt-diagnostic.go aftertouch-diagnostic-<timestamp>.age | tar xz
//
//	# Or decrypt to a .tar.gz file first:
//	go run scripts/decrypt-diagnostic.go aftertouch-diagnostic-<timestamp>.age > report.tar.gz
//	tar xzf report.tar.gz
//
// The private key is read from keys/private/diagnostic (relative to the repo root).
package main

import (
	"fmt"
	"io"
	"os"

	"filippo.io/age"
	"filippo.io/age/agessh"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run scripts/decrypt-diagnostic.go <file.age>")
		os.Exit(1)
	}

	keyPath := "keys/private/diagnostic"
	privKeyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read private key %s: %v\n", keyPath, err)
		os.Exit(1)
	}

	id, err := agessh.ParseIdentity(privKeyBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse identity: %v\n", err)
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	defer f.Close()

	r, err := age.Decrypt(f, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decrypt: %v\n", err)
		os.Exit(1)
	}

	if _, err := io.Copy(os.Stdout, r); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
}
