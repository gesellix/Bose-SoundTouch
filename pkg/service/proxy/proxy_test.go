package proxy

import (
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestLoggingProxy_Redaction(t *testing.T) {
	lp := NewLoggingProxy("http://example.com", true)
	if !lp.Redact {
		t.Error("Expected redact to be true")
	}
}

func TestIsSensitive(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{"Authorization", true},
		{"authorization", true},
		{"Cookie", true},
		{"X-Bose-Token", true},
		{"Content-Type", false},
		{"Accept", false},
	}

	for _, tt := range tests {
		if got := isSensitive(tt.header); got != tt.want {
			t.Errorf("isSensitive(%q) = %v, want %v", tt.header, got, tt.want)
		}
	}
}

func TestShouldLogBody(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"application/xml", true},
		{"application/json", true},
		{"text/plain", true},
		{"text/html", true},
		{"", true},
		{"audio/mpeg", false},
		{"application/octet-stream", false},
	}

	for _, tt := range tests {
		if got := shouldLogBody(tt.contentType); got != tt.want {
			t.Errorf("shouldLogBody(%q) = %v, want %v", tt.contentType, got, tt.want)
		}
	}
}

func TestLoggingProxy_LogRequest(t *testing.T) {
	os.Setenv("LOG_PROXY_BODY", "true")
	defer os.Unsetenv("LOG_PROXY_BODY")

	lp := NewLoggingProxy("http://example.com", true)

	body := "test body content"
	req := httptest.NewRequest("POST", "http://example.com/api", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "bearer secret")

	lp.LogRequest(req)

	// Check if body is still readable
	readBody, _ := io.ReadAll(req.Body)
	if string(readBody) != body {
		t.Errorf("Request body was consumed or changed, got %q, want %q", string(readBody), body)
	}
}
