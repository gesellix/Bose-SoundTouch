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

var sensitiveHeaders = []string{
	"Authorization",
	"Cookie",
	"X-Bose-Token",
}

// LoggingProxy wraps a ReverseProxy to provide instrumentation.
type LoggingProxy struct {
	Proxy       *httputil.ReverseProxy
	Redact      bool
	LogBody     bool
	MaxBodySize int64
}

func NewLoggingProxy(targetURL string, redact bool) *LoggingProxy {
	// targetURL logic should be handled by the caller or we can parse it here
	return &LoggingProxy{
		Redact:      redact,
		LogBody:     os.Getenv("LOG_PROXY_BODY") == "true",
		MaxBodySize: 1024 * 10, // 10KB default limit for logging
	}
}

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

	log.Printf("[PROXY_REQ] %s %s\n  Headers:\n%s\n  Body: %s", r.Method, r.URL.String(), headers, bodyStr)
}

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

	log.Printf("[PROXY_RES] %d %s\n  Headers:\n%s\n  Body: %s", r.StatusCode, r.Request.URL.String(), headers, bodyStr)
}

func formatHeaders(h http.Header, redact bool) string {
	var sb strings.Builder
	// In Go, http.Header is a map[string][]string.
	// Iterating over the map directly allows us to see the actual keys
	// stored in the map, which might not be canonical if set directly.
	for k, vv := range h {
		val := strings.Join(vv, ", ")
		if redact && isSensitive(k) {
			val = "[REDACTED]"
		}
		sb.WriteString(fmt.Sprintf("    %s: %s\n", k, val))
	}
	return strings.TrimSuffix(sb.String(), "\n")
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
