package handlers

import "strings"

// sanitizeLog strips newline characters from s to prevent log-injection
// (CodeQL go/log-injection). Values from speakers, HTTP requests, and
// external APIs may contain attacker-controlled newlines.
func sanitizeLog(s string) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)

	return s
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
