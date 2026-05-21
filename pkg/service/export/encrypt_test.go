package export

import (
	"testing"

	"filippo.io/age/agessh"
)

// TestDiagnosticPublicKeyParseable ensures the embedded diagnostic.pub is a
// valid SSH public key that age can use as a recipient. Catches a truncated or
// corrupted embed before it reaches a user trying to send a report.
func TestDiagnosticPublicKeyParseable(t *testing.T) {
	key := diagnosticPublicKey()
	if key == "" {
		t.Fatal("embedded diagnostic.pub is empty")
	}
	if _, err := agessh.ParseRecipient(key); err != nil {
		t.Errorf("embedded diagnostic.pub is not a valid age SSH recipient: %v", err)
	}
}
