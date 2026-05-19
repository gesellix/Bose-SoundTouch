package health

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"
)

// buildCA returns a CA cert with the given NotBefore / NotAfter.
// Self-signed; subject doesn't matter for these tests.
func buildCA(t *testing.T, notBefore, notAfter time.Time) *x509.Certificate {
	t.Helper()

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(2026),
		Subject:               pkix.Name{CommonName: "AfterTouch Local Root CA", Organization: []string{"AfterTouch Test"}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	template.Issuer = template.Subject

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	return cert
}

func TestCAExpiry_HealthyCertProducesNoFinding(t *testing.T) {
	now := time.Now()
	cert := buildCA(t, now.Add(-30*24*time.Hour), now.Add(2*365*24*time.Hour))

	got := runCACertExpiryCheck(func() *x509.Certificate { return cert }, nil, now)
	if len(got) != 0 {
		t.Errorf("expected no findings for healthy cert, got %+v", got)
	}
}

func TestCAExpiry_InfoLevelInsideNinetyDays(t *testing.T) {
	now := time.Now()
	cert := buildCA(t, now.Add(-30*24*time.Hour), now.Add(60*24*time.Hour))

	got := runCACertExpiryCheck(func() *x509.Certificate { return cert }, nil, now)
	if len(got) != 1 || got[0].Severity != SeverityInfo {
		t.Fatalf("expected one info finding, got %+v", got)
	}

	if !strings.Contains(got[0].Message, "60") {
		t.Errorf("expected day count in message, got %q", got[0].Message)
	}
}

func TestCAExpiry_WarningInsideThirtyDays(t *testing.T) {
	now := time.Now()
	cert := buildCA(t, now.Add(-30*24*time.Hour), now.Add(15*24*time.Hour))

	got := runCACertExpiryCheck(func() *x509.Certificate { return cert }, nil, now)
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning, got %+v", got)
	}

	if !strings.Contains(got[0].Message, "15") {
		t.Errorf("expected day count in message, got %q", got[0].Message)
	}

	if !strings.Contains(got[0].Details, "Plan a regeneration") {
		t.Errorf("expected regen guidance in details, got %q", got[0].Details)
	}
}

func TestCAExpiry_ErrorWhenExpired(t *testing.T) {
	now := time.Now()
	cert := buildCA(t, now.Add(-365*24*time.Hour), now.Add(-2*24*time.Hour))

	got := runCACertExpiryCheck(func() *x509.Certificate { return cert }, nil, now)
	if len(got) != 1 || got[0].Severity != SeverityError {
		t.Fatalf("expected one error, got %+v", got)
	}

	if !strings.Contains(got[0].Message, "expired") {
		t.Errorf("expected 'expired' in message, got %q", got[0].Message)
	}

	if !strings.Contains(got[0].Message, "2 day") {
		t.Errorf("expected day count in message, got %q", got[0].Message)
	}
}

func TestCAExpiry_NoCAGracefulInfo(t *testing.T) {
	got := runCACertExpiryCheck(func() *x509.Certificate { return nil }, nil, time.Now())
	if len(got) != 1 || got[0].Severity != SeverityInfo {
		t.Fatalf("expected one info finding when CA missing, got %+v", got)
	}
}

func TestCAExpiry_NilLoaderSkipsEntirely(t *testing.T) {
	got := runCACertExpiryCheck(nil, nil, time.Now())
	if len(got) != 0 {
		t.Errorf("expected no findings with nil loader, got %+v", got)
	}
}

func TestCARegenCommand_IncludesActualPath(t *testing.T) {
	got := caRegenCommand(func() string { return "/var/lib/aftertouch/ca.crt" })
	if !strings.Contains(got, "/var/lib/aftertouch/ca.crt") {
		t.Errorf("expected cert path in command, got %q", got)
	}

	if !strings.Contains(got, "/var/lib/aftertouch/ca.key") {
		t.Errorf("expected .key sibling path in command, got %q", got)
	}
}

func TestCARegenCommand_FallsBackWhenPathUnknown(t *testing.T) {
	got := caRegenCommand(nil)
	if !strings.Contains(got, "remove the CA") {
		t.Errorf("expected fallback hint when path unknown, got %q", got)
	}

	got = caRegenCommand(func() string { return "" })
	if !strings.Contains(got, "remove the CA") {
		t.Errorf("expected fallback hint when path empty, got %q", got)
	}
}

func TestCAExpiry_ManualCommandPathRendered(t *testing.T) {
	now := time.Now()
	cert := buildCA(t, now.Add(-30*24*time.Hour), now.Add(5*24*time.Hour))

	got := runCACertExpiryCheck(
		func() *x509.Certificate { return cert },
		func() string { return "/srv/aftertouch/ca.crt" },
		now,
	)

	if len(got) != 1 || len(got[0].ManualCommands) != 1 {
		t.Fatalf("expected one manual command, got %+v", got)
	}

	cmd := got[0].ManualCommands[0]
	if !strings.Contains(cmd.Command, "/srv/aftertouch/ca.crt") {
		t.Errorf("expected actual path in command, got %q", cmd.Command)
	}
}
