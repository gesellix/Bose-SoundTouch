package proxy

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	if err := os.Setenv("LOG_PROXY_BODY", "true"); err != nil {
		t.Fatalf("Failed to set LOG_PROXY_BODY: %v", err)
	}

	defer func() { _ = os.Unsetenv("LOG_PROXY_BODY") }()

	lp := NewLoggingProxy("http://example.com", true)
	lp.LogBody = true

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

	// Test truncation
	lp.MaxBodySize = 4
	req2 := httptest.NewRequest("POST", "http://example.com/api", strings.NewReader("1234567890"))
	req2.Header.Set("Content-Type", "text/plain")
	lp.LogRequest(req2)
}

func TestLoggingProxy_LogResponse(t *testing.T) {
	lp := NewLoggingProxy("http://example.com", true)
	lp.LogBody = true

	body := "response content"
	req := httptest.NewRequest("GET", "http://example.com/api", nil)
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.WriteString(body)
	res := w.Result()
	res.Request = req

	lp.LogResponse(res)

	// Check if body is still readable
	readBody, _ := io.ReadAll(res.Body)
	if string(readBody) != body {
		t.Errorf("Response body was consumed or changed, got %q, want %q", string(readBody), body)
	}

	// Test with recorder
	tmpDir, _ := os.MkdirTemp("", "proxy-recorder-test")
	defer os.RemoveAll(tmpDir)
	recorder := NewRecorder(tmpDir)
	lp.SetRecorder(recorder)
	lp.RecordEnabled = true

	lp.LogResponse(res)

	// Verify recording exists
	interactionsDir := filepath.Join(tmpDir, "interactions", recorder.SessionID, "upstream", "api")
	files, _ := os.ReadDir(interactionsDir)
	if len(files) == 0 {
		t.Error("LogResponse did not record the interaction")
	}
}
