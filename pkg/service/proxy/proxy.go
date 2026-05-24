// Package proxy provides a logging reverse proxy used for speaker traffic debugging.
package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
)

// alwaysSensitiveHeaders are stripped from log output unconditionally — they
// carry credentials whose plaintext value should never appear in a log line
// regardless of how the LoggingProxy was constructed.
var alwaysSensitiveHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Cookie",
	"Set-Cookie",
	"X-Api-Key",
	"X-Bose-Token",
}

// sensitiveHeaders is kept for backwards compatibility with callers that
// reference it by name; it now mirrors alwaysSensitiveHeaders.
var sensitiveHeaders = alwaysSensitiveHeaders

// LoggingProxy wraps a ReverseProxy to provide instrumentation.
type LoggingProxy struct {
	Proxy         *httputil.ReverseProxy
	Redact        bool
	LogBody       bool
	RecordEnabled bool
	MaxBodySize   int64
	Recorder      *Recorder

	// UnsafeLogCredentialHeaders disables the otherwise-unconditional
	// redaction of credential-bearing headers (Authorization, Cookie, …) in
	// LogRequest / LogResponse output. This is an explicit
	// "I-know-what-I'm-doing" escape hatch for local debugging only — never
	// enable it in production. Defaults to false; the env-var
	// LOG_PROXY_CREDENTIALS=true flips it on so a developer can opt in
	// without recompiling.
	UnsafeLogCredentialHeaders bool
}

// NewLoggingProxy creates a lightweight logger for HTTP requests/responses.
func NewLoggingProxy(_ string, redact bool) *LoggingProxy {
	// targetURL logic should be handled by the caller or we can parse it here
	return &LoggingProxy{
		Redact:                     redact,
		LogBody:                    os.Getenv("LOG_PROXY_BODY") == "true",
		UnsafeLogCredentialHeaders: os.Getenv("LOG_PROXY_CREDENTIALS") == "true",
		MaxBodySize:                1024 * 10, // 10KB default limit for logging
	}
}

// SetRecorder sets the recorder for the proxy.
func (lp *LoggingProxy) SetRecorder(r *Recorder) {
	lp.Recorder = r
}

// LogRequest prints an abbreviated request with optional header/body redaction.
func (lp *LoggingProxy) LogRequest(r *http.Request) {
	headers := formatHeaders(r.Header, lp.Redact, lp.UnsafeLogCredentialHeaders)

	bodyStr := "[HIDDEN]"

	if lp.LogBody && shouldLogBody(r.Header.Get("Content-Type")) {
		if r.Body != nil {
			bodyBytes, _ := io.ReadAll(r.Body)

			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			if int64(len(bodyBytes)) > lp.MaxBodySize {
				bodyStr = string(bodyBytes[:lp.MaxBodySize]) + "... [TRUNCATED]"
			} else {
				bodyStr = string(bodyBytes)
			}
		} else {
			bodyStr = "[EMPTY]"
		}
	}

	log.Printf("[PROXY_REQ] %s %s\n  Headers:\n%s\n  Body: %s", r.Method, sanitizeLog(r.URL.String()), headers, sanitizeLog(bodyStr))
}

// LogResponse prints an abbreviated response with optional header/body redaction.
func (lp *LoggingProxy) LogResponse(r *http.Response) {
	headers := formatHeaders(r.Header, lp.Redact, lp.UnsafeLogCredentialHeaders)

	bodyStr := "[HIDDEN]"

	if lp.LogBody && shouldLogBody(r.Header.Get("Content-Type")) {
		if r.Body != nil {
			bodyBytes, _ := io.ReadAll(r.Body)

			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			if int64(len(bodyBytes)) > lp.MaxBodySize {
				bodyStr = string(bodyBytes[:lp.MaxBodySize]) + "... [TRUNCATED]"
			} else {
				bodyStr = string(bodyBytes)
			}
		} else {
			bodyStr = "[EMPTY]"
		}
	}

	log.Printf("[PROXY_RES] %d %s\n  Headers:\n%s\n  Body: %s", r.StatusCode, sanitizeLog(r.Request.URL.String()), headers, sanitizeLog(bodyStr))

	if lp.Recorder != nil && lp.RecordEnabled {
		_ = lp.Recorder.Record("upstream", r.Request, r)
	}
}

func formatHeaders(h http.Header, redact, unsafeLogCredentials bool) string {
	var sb strings.Builder
	// In Go, http.Header is a map[string][]string.
	// Iterating over the map directly allows us to see the actual keys
	// stored in the map, which might not be canonical if set directly.
	for k, vv := range h {
		val := strings.Join(vv, ", ")
		// Credentials (Authorization, Cookie, …) are redacted by default.
		// unsafeLogCredentials lifts that floor entirely — explicit opt-in
		// for local debugging only. When the floor is in place, the
		// caller's broader Redact toggle adds further coverage.
		switch {
		case unsafeLogCredentials:
			// No redaction.
		case isAlwaysSensitive(k):
			val = "[REDACTED]"
		case redact && isSensitive(k):
			val = "[REDACTED]"
		}

		fmt.Fprintf(&sb, "    %s: %s\n", k, val)
	}

	return strings.TrimSuffix(sb.String(), "\n")
}

// isAlwaysSensitive returns true for credential-bearing headers that must
// never appear unredacted in logs regardless of caller configuration.
func isAlwaysSensitive(header string) bool {
	for _, h := range alwaysSensitiveHeaders {
		if strings.EqualFold(h, header) {
			return true
		}
	}

	return false
}

func isSensitive(header string) bool {
	for _, h := range sensitiveHeaders {
		if strings.EqualFold(h, header) {
			return true
		}
	}

	return false
}

func shouldLogBody(contentType string) bool {
	contentType = strings.ToLower(contentType)

	return strings.Contains(contentType, "xml") ||
		strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "text") ||
		contentType == ""
}
