package discovery

import (
	"log"
	"net"
	"strings"
	"sync/atomic"
)

// verboseLogging toggles the per-packet / per-header diagnostic output
// that was historically emitted unconditionally during UPnP and mDNS
// discovery. The service binary leaves it at its zero value (off) so
// the log stays useful at info-level; the CLI's `discover` command
// flips it on so interactive runs surface full protocol details.
//
// Stored as an int32 so the read path in logVerbose is allocation-
// free (atomic.Bool would work too on Go 1.19+, but a uint8 lookup
// keeps the toggle hot-path even on older toolchains we still build
// against in CI).
var verboseLogging atomic.Bool

// SetVerbose enables (or disables) the package-wide verbose-discovery
// log toggle. Safe to call from any goroutine.
func SetVerbose(v bool) {
	verboseLogging.Store(v)
}

// IsVerbose reports the current value of the verbose toggle. Mainly
// for tests that want to assert the CLI flipped it on.
func IsVerbose() bool {
	return verboseLogging.Load()
}

// logVerbose forwards to log.Printf only when verbose-discovery
// logging is enabled. The fast path (verbose off) is a single
// atomic load + branch, so it's safe to scatter calls liberally
// across the hot path.
func logVerbose(format string, args ...any) {
	if verboseLogging.Load() {
		log.Printf(format, args...)
	}
}

// sanitizeLog strips newline characters from s to prevent log-injection
// (CodeQL go/log-injection). Values from speakers, HTTP requests, and
// external APIs may contain attacker-controlled newlines.
func sanitizeLog(s string) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)

	return s
}

// remoteAddrString safely converts a net.Addr to a string, returning
// an empty string when addr is nil (dns.ResponseWriter.RemoteAddr may
// return nil in unit-test contexts).
func remoteAddrString(w interface{ RemoteAddr() net.Addr }) string {
	if ra := w.RemoteAddr(); ra != nil {
		return ra.String()
	}

	return ""
}

// sanitizeErr returns err.Error() with newlines stripped to prevent log
// injection when error messages contain user-controlled values. Use in
// place of bare "%v, err" in log calls where err may wrap external data.
func sanitizeErr(err error) string {
	if err == nil {
		return "<nil>"
	}

	return sanitizeLog(err.Error())
}
