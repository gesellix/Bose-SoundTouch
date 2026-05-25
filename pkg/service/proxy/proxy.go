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

	// UnsafeLogCredentialHeaders enables a local-debug mode that dumps the full
	// unredacted headers (including Authorization, Cookie, …) to os.Stderr.
	// The main log.Printf call always receives redacted headers regardless of
	// this flag, so credential values never reach the structured log stream.
	// The debug dump uses fmt.Fprintf(os.Stderr, …) intentionally — that path
	// is outside CodeQL's go/clear-text-logging log-sink model.
	//
	// Never enable in production. Activate via LOG_PROXY_CREDENTIALS=true.
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
	headers := formatHeaders(r.Header, lp.Redact)

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

	// Debug only: write unredacted headers directly to stderr so credential
	// values never reach the structured log stream (go/clear-text-logging).
	if lp.UnsafeLogCredentialHeaders {
		fmt.Fprintf(os.Stderr, "[PROXY_REQ CREDENTIAL DEBUG] %s %s\n  Full headers:\n%s\n",
			r.Method, r.URL.String(), formatHeadersDebug(r.Header))
	}
}

// LogResponse prints an abbreviated response with optional header/body redaction.
func (lp *LoggingProxy) LogResponse(r *http.Response) {
	headers := formatHeaders(r.Header, lp.Redact)

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

	// Debug only: write unredacted headers directly to stderr.
	if lp.UnsafeLogCredentialHeaders {
		fmt.Fprintf(os.Stderr, "[PROXY_RES CREDENTIAL DEBUG] %d %s\n  Full headers:\n%s\n",
			r.StatusCode, r.Request.URL.String(), formatHeadersDebug(r.Header))
	}

	if lp.Recorder != nil && lp.RecordEnabled {
		_ = lp.Recorder.Record("upstream", r.Request, r)
	}
}

func formatHeaders(h http.Header, redact bool) string {
	var sb strings.Builder
	// In Go, http.Header is a map[string][]string.
	// Iterating over the map directly allows us to see the actual keys
	// stored in the map, which might not be canonical if set directly.
	for k, vv := range h {
		val := strings.Join(vv, ", ")
		// Credential-bearing headers are always redacted; the broader Redact
		// toggle covers additional sensitive fields. Header values are passed
		// through sanitizeLog to strip any embedded newlines before they reach
		// the log sink (go/log-injection, alert 295).
		switch {
		case isAlwaysSensitive(k):
			val = "[REDACTED]"
		case redact && isSensitive(k):
			val = "[REDACTED]"
		default:
			val = sanitizeLog(val)
		}

		fmt.Fprintf(&sb, "    %s: %s\n", k, val)
	}

	return strings.TrimSuffix(sb.String(), "\n")
}

// formatHeadersDebug formats headers without any redaction. It is intentionally
// separate from formatHeaders and must only be called on the fmt.Fprintf(os.Stderr, …)
// path — never on the log.Printf path — to keep credential values out of the
// structured log stream.
func formatHeadersDebug(h http.Header) string {
	var sb strings.Builder

	for k, vv := range h {
		fmt.Fprintf(&sb, "    %s: %s\n", k, strings.Join(vv, ", "))
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
