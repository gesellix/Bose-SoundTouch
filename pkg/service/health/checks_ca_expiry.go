package health

import (
	"crypto/x509"
	"fmt"
	"strings"
	"time"
)

// CheckIDCACertExpiry is the registry id of the CA-cert expiry
// check.
const CheckIDCACertExpiry = "ca_cert_expiry"

// CA-expiry thresholds. Tunable here rather than per-deployment
// because the consequence of a missed warning is the same
// everywhere: leaves issued by the CA will be rejected.
const (
	caExpiryWarnThreshold     = 30 * 24 * time.Hour
	caExpiryInfoThreshold     = 90 * 24 * time.Hour
	caExpiryRecentlyValidated = 365 * 24 * time.Hour
)

// CACertPathFunc returns the on-disk path of AfterTouch's own
// CA cert. Optional; when nil the manual command falls back to a
// neutral hint.
type CACertPathFunc func() string

// RegisterCACertExpiryCheck registers a check that reads
// AfterTouch's own CA cert (via caCertFn) and emits a finding
// when its NotAfter is in the past or in the warn/info windows.
//
// Why a separate check from service_cert_chain: even when the
// served leaf validates today (or is correctly classified as
// self-signed), the CA's eventual expiry will break every leaf
// it ever issued. Operators should regenerate before that
// happens — and re-pair speakers, since their stored trust
// anchor will no longer cover newly-issued leaves.
//
// caCertPathFn is used purely to render a remediation command
// pointing at the actual on-disk path. Pass nil to skip the
// path mention.
func RegisterCACertExpiryCheck(r *Registry, caCertFn func() *x509.Certificate, caCertPathFn CACertPathFunc) {
	r.Register(Check{
		ID:    CheckIDCACertExpiry,
		Title: "AfterTouch CA cert is not near expiry",
		Run: func() []Finding {
			return runCACertExpiryCheck(caCertFn, caCertPathFn, time.Now())
		},
	})
}

func runCACertExpiryCheck(caCertFn func() *x509.Certificate, caCertPathFn CACertPathFunc, now time.Time) []Finding {
	if caCertFn == nil {
		return nil
	}

	cert := caCertFn()
	if cert == nil {
		return []Finding{{
			Severity: SeverityInfo,
			Message:  "Couldn't load AfterTouch's own CA cert; expiry not checked.",
			Details:  "service_cert_chain falls back to a Subject==Issuer heuristic for the same reason. Verify the CA path is readable from the service host.",
		}}
	}

	if cert.NotAfter.IsZero() {
		return []Finding{{
			Severity: SeverityWarning,
			Message:  "AfterTouch CA cert has no NotAfter set; treat as expired.",
		}}
	}

	remaining := cert.NotAfter.Sub(now)
	expiresAt := cert.NotAfter.UTC().Format("2006-01-02")

	switch {
	case remaining <= 0:
		return []Finding{caExpiryFinding(SeverityError,
			fmt.Sprintf("AfterTouch CA cert expired %d day(s) ago (on %s).", daysRounded(-remaining), expiresAt),
			"Speakers will reject any leaf signed by this CA. Regenerate now: stop the service, remove the CA files, restart so EnsureCA reissues, then run setup install-ca against every paired speaker.",
			cert, caCertPathFn,
		)}
	case remaining <= caExpiryWarnThreshold:
		return []Finding{caExpiryFinding(SeverityWarning,
			fmt.Sprintf("AfterTouch CA cert expires in %d day(s) (on %s).", daysRounded(remaining), expiresAt),
			"Plan a regeneration. Every paired speaker will need setup install-ca again after, since their stored trust anchor won't cover the new leaves.",
			cert, caCertPathFn,
		)}
	case remaining <= caExpiryInfoThreshold:
		return []Finding{caExpiryFinding(SeverityInfo,
			fmt.Sprintf("AfterTouch CA cert expires in %d day(s) (on %s).", daysRounded(remaining), expiresAt),
			"No immediate action; surfaced here so the renewal isn't a surprise.",
			cert, caCertPathFn,
		)}
	}

	// > 90 days remaining; nothing to surface. Optionally we
	// could emit a positive info finding ("valid until …") but
	// the empty-findings path lets the check roll up to OK,
	// which is the clearer "healthy" signal.
	_ = caExpiryRecentlyValidated

	return nil
}

// daysRounded converts a duration to a whole-day count, rounded
// to nearest. Avoids the "expires in 59 days" surprise when the
// real value is 60 days minus a fraction-of-a-second from ASN.1
// time truncation.
func daysRounded(d time.Duration) int {
	return int((d + 12*time.Hour) / (24 * time.Hour))
}

func caExpiryFinding(severity Severity, message, details string, cert *x509.Certificate, caCertPathFn CACertPathFunc) Finding {
	enrichedDetails := fmt.Sprintf("%s Subject: %s. Valid from: %s. Valid to: %s.",
		details,
		cert.Subject.String(),
		cert.NotBefore.UTC().Format(time.RFC3339),
		cert.NotAfter.UTC().Format(time.RFC3339),
	)

	commands := []ManualCommand{{
		Label:   "Regenerate (destructive — requires re-installing the CA on every speaker afterwards):",
		Command: caRegenCommand(caCertPathFn),
		Hint:    "Adjust the path to match your deployment if it differs. The service's EnsureCA() reissues a fresh CA at startup when the file is missing.",
	}}

	return Finding{
		Severity:       severity,
		Message:        message,
		Details:        enrichedDetails,
		ManualCommands: commands,
	}
}

func caRegenCommand(caCertPathFn CACertPathFunc) string {
	if caCertPathFn == nil {
		return "# stop soundtouch-service, remove the CA cert + key files, restart"
	}

	path := strings.TrimSpace(caCertPathFn())
	if path == "" {
		return "# stop soundtouch-service, remove the CA cert + key files, restart"
	}

	// Key path conventionally sits next to the cert with a `.key`
	// extension or matching basename; surface the cert path
	// explicitly and let the operator pick up the key by sight.
	keyHint := strings.TrimSuffix(path, ".crt") + ".key"

	return fmt.Sprintf("# stop the service, then:\nrm '%s' '%s'\n# restart soundtouch-service; EnsureCA reissues at boot.", path, keyHint)
}
