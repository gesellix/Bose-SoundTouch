package proxy

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRecorder_Record_Structure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)

	tests := []struct {
		name     string
		category string
		path     string
		expected string // Expected subdirectory after interactions/{sessionID}/{category}/
	}{
		{
			name:     "root_path",
			category: "self",
			path:     "/",
			expected: "root",
		},
		{
			name:     "simple_path",
			category: "self",
			path:     "/setup/info",
			expected: "setup/info",
		},
		{
			name:     "path_with_ip",
			category: "self",
			path:     "/setup/info/192.168.178.35",
			expected: "setup/info/{ip}",
		},
		{
			name:     "upstream_path",
			category: "upstream",
			path:     "/v1/playback/station/s123",
			expected: "v1/playback/station/s123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Method: "GET",
				URL: &url.URL{
					Path: tt.path,
				},
				Header: make(http.Header),
			}

			err := r.Record(tt.category, req, nil)
			if err != nil {
				t.Fatalf("Record failed: %v", err)
			}

			expectedDir := filepath.Join(tmpDir, "interactions", r.SessionID, tt.category, tt.expected)
			if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
				t.Errorf("Expected directory %s does not exist", expectedDir)
			}

			// Check if file was created
			files, _ := os.ReadDir(expectedDir)
			if len(files) == 0 {
				t.Errorf("No files created in %s", expectedDir)
			}
			for _, f := range files {
				if !strings.Contains(f.Name(), "-GET.http") {
					t.Errorf("Unexpected filename: %s", f.Name())
				}
				// Verify prefix is 4 digits
				if len(f.Name()) < 5 || !isDigit(f.Name()[0]) || !isDigit(f.Name()[1]) || !isDigit(f.Name()[2]) || !isDigit(f.Name()[3]) || f.Name()[4] != '-' {
					t.Errorf("Filename %s does not have correct 0000- prefix", f.Name())
				}
			}
		})
	}
}

func TestRecorder_Record_Sanitization(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-sanitization-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)
	// Add a custom pattern
	r.Patterns = append(r.Patterns, PathPattern{
		Name:        "DeviceID",
		Regexp:      `^A81B\w{8}$`,
		Replacement: "{deviceId}",
	})
	// Re-compile
	for i := range r.Patterns {
		re, _ := regexp.Compile(r.Patterns[i].Regexp)
		r.Patterns[i].compiled = re
	}

	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path: "/info/192.168.178.35/A81B6A536A98",
		},
		Header: make(http.Header),
	}
	req.Header.Set("X-Device", "A81B6A536A98")

	err = r.Record("self", req, nil)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	expectedDir := filepath.Join(tmpDir, "interactions", r.SessionID, "self", "info", "{ip}", "{deviceId}")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("Expected directory %s does not exist", expectedDir)
	}

	files, _ := os.ReadDir(expectedDir)
	if len(files) == 0 {
		t.Fatalf("No files created in %s", expectedDir)
	}

	content, _ := os.ReadFile(filepath.Join(expectedDir, files[0].Name()))
	contentStr := string(content)

	if !strings.Contains(contentStr, "### GET /info/{{ip}}/{{deviceId}}") {
		t.Errorf("Expected sanitized comment in .http file, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "GET /info/{{ip}}/{{deviceId}}") {
		t.Errorf("Expected sanitized URL in .http file, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "X-Device: {{deviceId}}") {
		t.Errorf("Expected sanitized Header in .http file, got:\n%s", contentStr)
	}
}

func TestRecorder_Record_Sanitization_Account(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-sanitization-account-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)
	// Add AccountID pattern
	r.Patterns = append(r.Patterns, PathPattern{
		Name:        "AccountID",
		Regexp:      `^\d{1,10}$`,
		Replacement: "{accountId}",
	})
	// Re-compile
	for i := range r.Patterns {
		re, _ := regexp.Compile(r.Patterns[i].Regexp)
		r.Patterns[i].compiled = re
	}

	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path: "/marge/accounts/12345/full",
		},
		Header: make(http.Header),
	}

	err = r.Record("self", req, nil)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	expectedDir := filepath.Join(tmpDir, "interactions", r.SessionID, "self", "marge", "accounts", "{accountId}", "full")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("Expected directory %s does not exist", expectedDir)
	}

	files, _ := os.ReadDir(expectedDir)
	if len(files) == 0 {
		t.Fatalf("No files created in %s", expectedDir)
	}

	content, _ := os.ReadFile(filepath.Join(expectedDir, files[0].Name()))
	contentStr := string(content)

	if !strings.Contains(contentStr, "### GET /marge/accounts/{{accountId}}/full") {
		t.Errorf("Expected sanitized comment in .http file, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "GET /marge/accounts/{{accountId}}/full") {
		t.Errorf("Expected sanitized URL in .http file, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "// accountId: 12345") {
		t.Errorf("Expected accountId comment in .http file, got:\n%s", contentStr)
	}
}

func TestRecorder_Record_Redaction(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-redaction-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)
	r.Redact = true

	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path: "/test",
		},
		Header: make(http.Header),
	}
	req.Header.Set("Authorization", "Bearer sensitive-token")
	req.Header.Set("Cookie", "session=secret")
	req.Header.Set("X-Normal", "public-info")

	err = r.Record("self", req, nil)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	expectedDir := filepath.Join(tmpDir, "interactions", r.SessionID, "self", "test")
	files, _ := os.ReadDir(expectedDir)
	content, _ := os.ReadFile(filepath.Join(expectedDir, files[0].Name()))
	contentStr := string(content)

	if !strings.Contains(contentStr, "Authorization: [REDACTED]") {
		t.Errorf("Expected Authorization header to be redacted, got:\n%s", contentStr)
	}
	if strings.Contains(contentStr, "sensitive-token") {
		t.Errorf("Sensitive token still present in content:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "Cookie: [REDACTED]") {
		t.Errorf("Expected Cookie header to be redacted, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "X-Normal: public-info") {
		t.Errorf("Expected normal header to be present, got:\n%s", contentStr)
	}
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func TestRecorder_IncreasingPrefix(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-prefix-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)
	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path: "/test",
		},
		Header: make(http.Header),
	}

	for i := 1; i <= 3; i++ {
		err := r.Record("self", req, nil)
		if err != nil {
			t.Fatalf("Record failed: %v", err)
		}
	}

	expectedDir := filepath.Join(tmpDir, "interactions", r.SessionID, "self", "test")
	files, _ := os.ReadDir(expectedDir)
	if len(files) != 3 {
		t.Fatalf("Expected 3 files, got %d", len(files))
	}

	expectedPrefixes := []string{"0001-", "0002-", "0003-"}
	for i, f := range files {
		if !strings.HasPrefix(f.Name(), expectedPrefixes[i]) {
			t.Errorf("File %d: expected prefix %s, got %s", i, expectedPrefixes[i], f.Name())
		}
	}
}

func TestRecorder_EnvFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-env-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)
	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path: "/info/192.168.178.35",
		},
		Header: make(http.Header),
	}

	err = r.Record("self", req, nil)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	envFile := filepath.Join(tmpDir, "interactions", r.SessionID, "http-client.env.json")
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		t.Fatalf("Expected env file %s does not exist", envFile)
	}

	data, _ := os.ReadFile(envFile)
	var content map[string]map[string]string
	if err := json.Unmarshal(data, &content); err != nil {
		t.Fatalf("Failed to unmarshal env file: %v", err)
	}

	if content["session"]["ip"] != "192.168.178.35" {
		t.Errorf("Expected ip to be 192.168.178.35, got %s", content["session"]["ip"])
	}
}
