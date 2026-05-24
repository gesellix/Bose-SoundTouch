package stockholm

import "strings"

// sanitizeLog strips newline characters from s to prevent log-injection
// (CodeQL go/log-injection). Values from speakers, HTTP requests, and
// external APIs may contain attacker-controlled newlines.
func sanitizeLog(s string) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)

	return s
}
