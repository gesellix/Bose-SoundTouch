// Package export provides encryption helpers for the diagnostic export feature.
package export

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"filippo.io/age"
	"filippo.io/age/agessh"
)

// diagnosticPublicKey is embedded from diagnostic.pub at compile time.
// diagnostic.pub is kept in sync with keys/public/diagnostic.pub by
// scripts/setup-diagnostic-key.sh. To verify it matches the maintainer's
// GitHub SSH keys:
//
//	curl -s https://github.com/gesellix.keys | grep "$(awk '{print $2}' keys/public/diagnostic.pub)"
//
//go:embed diagnostic.pub
var diagnosticPublicKeyRaw string

// diagnosticPublicKey returns the trimmed SSH public key line.
func diagnosticPublicKey() string {
	return strings.TrimSpace(diagnosticPublicKeyRaw)
}

// EncryptDiagnostic encrypts plaintext using the embedded SSH public key
// and returns the age-encrypted bytes. The result can only be decrypted
// with the corresponding SSH private key (keys/private/diagnostic).
func EncryptDiagnostic(plaintext []byte) ([]byte, error) {
	recipient, err := agessh.ParseRecipient(diagnosticPublicKey())
	if err != nil {
		return nil, fmt.Errorf("parse recipient key: %w", err)
	}

	var buf bytes.Buffer

	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, fmt.Errorf("age encrypt: %w", err)
	}

	if _, err := w.Write(plaintext); err != nil {
		return nil, fmt.Errorf("write plaintext: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close age writer: %w", err)
	}

	return buf.Bytes(), nil
}
