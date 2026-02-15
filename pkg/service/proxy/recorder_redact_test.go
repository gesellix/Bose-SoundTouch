package proxy

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecorder_Redaction(t *testing.T) {
	// Disable async for testing
	t.Setenv("RECORDER_ASYNC", "false")

	tmpDir, err := os.MkdirTemp("", "recorder-redact-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)
	r.Redact = true // Enable redaction

	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	req.Header.Set("Authorization", "Bearer sensitive-token")
	req.Header.Set("X-Custom", "safe-value")

	w := httptest.NewRecorder()
	w.Header().Set("X-Bose-Token", "sensitive-bose-token")
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.WriteString("hello")
	res := w.Result()
	res.Request = req

	err = r.Record("test", req, res)
	if err != nil {
		t.Fatalf("Failed to record: %v", err)
	}

	// Find the recorded file
	var recordedFile string
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".http") {
			recordedFile = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Error walking temp dir: %v", err)
	}

	if recordedFile == "" {
		t.Fatal("No recorded .http file found")
	}

	content, err := os.ReadFile(recordedFile)
	if err != nil {
		t.Fatalf("Failed to read recorded file: %v", err)
	}

	contentStr := string(content)

	// Check for redaction in request headers
	if strings.Contains(contentStr, "sensitive-token") {
		t.Errorf("Recorded file contains sensitive Authorization header value:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "Authorization: [REDACTED]") {
		t.Errorf("Recorded file does not contain redacted Authorization header:\n%s", contentStr)
	}

	// Check for redaction in response headers
	if strings.Contains(contentStr, "sensitive-bose-token") {
		t.Errorf("Recorded file contains sensitive X-Bose-Token header value:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "X-Bose-Token: [REDACTED]") {
		t.Errorf("Recorded file does not contain redacted X-Bose-Token header:\n%s", contentStr)
	}

	// Check that non-sensitive headers are NOT redacted
	if !strings.Contains(contentStr, "X-Custom: safe-value") {
		t.Errorf("Recorded file missing non-sensitive header or it was incorrectly redacted:\n%s", contentStr)
	}
}

func TestRecorder_NoRedaction(t *testing.T) {
	// Disable async for testing
	t.Setenv("RECORDER_ASYNC", "false")

	tmpDir, err := os.MkdirTemp("", "recorder-no-redact-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)
	r.Redact = false // Disable redaction

	req := httptest.NewRequest("GET", "http://example.com/api/test", nil)
	req.Header.Set("Authorization", "Bearer sensitive-token")

	w := httptest.NewRecorder()
	w.Header().Set("X-Bose-Token", "sensitive-bose-token")
	_, _ = w.WriteString("hello")
	res := w.Result()
	res.Request = req

	err = r.Record("test", req, res)
	if err != nil {
		t.Fatalf("Failed to record: %v", err)
	}

	// Find the recorded file
	var recordedFile string
	filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && strings.HasSuffix(path, ".http") {
			recordedFile = path
		}
		return nil
	})

	content, _ := os.ReadFile(recordedFile)
	contentStr := string(content)

	if !strings.Contains(contentStr, "Bearer sensitive-token") {
		t.Errorf("Recorded file should contain sensitive Authorization header when Redact=false:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "sensitive-bose-token") {
		t.Errorf("Recorded file should contain sensitive X-Bose-Token header when Redact=false:\n%s", contentStr)
	}
}
