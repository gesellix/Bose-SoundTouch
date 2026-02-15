package proxy

import (
	"bytes"
	"encoding/json"
	"io"
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
	// Compile default patterns
	for i := range r.Patterns {
		re, _ := regexp.Compile(r.Patterns[i].Regexp)
		r.Patterns[i].compiled = re
	}

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
		Name:        "CustomDeviceID",
		Regexp:      `^A81B\w{8}$`,
		Replacement: "{deviceId}",
	})
	// Compile all patterns
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

	expectedDir := filepath.Join(tmpDir, "interactions", r.SessionID, "self", "info", "{ip}", "{device_id}")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("Expected directory %s does not exist", expectedDir)
	}

	files, _ := os.ReadDir(expectedDir)
	if len(files) == 0 {
		t.Fatalf("No files created in %s", expectedDir)
	}

	content, _ := os.ReadFile(filepath.Join(expectedDir, files[0].Name()))
	contentStr := string(content)

	if !strings.Contains(contentStr, "### GET /info/{{ip}}/{{device_id}}") {
		t.Errorf("Expected sanitized comment in .http file, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "GET /info/{{ip}}/{{device_id}}") {
		t.Errorf("Expected sanitized URL in .http file, got:\n%s", contentStr)
	}
	if !strings.Contains(contentStr, "X-Device: {{device_id}}") {
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
	// Use default patterns which now include AccountID
	r.Patterns = DefaultPatterns()
	// Compile all patterns
	for i := range r.Patterns {
		re, _ := regexp.Compile(r.Patterns[i].Regexp)
		r.Patterns[i].compiled = re
	}

	accountID := "1234567"
	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Path: "/marge/accounts/" + accountID + "/full",
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
	if !strings.Contains(contentStr, "// accountId: "+accountID) {
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
	// Compile default patterns
	for i := range r.Patterns {
		re, _ := regexp.Compile(r.Patterns[i].Regexp)
		r.Patterns[i].compiled = re
	}
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
	// Compile default patterns
	for i := range r.Patterns {
		re, _ := regexp.Compile(r.Patterns[i].Regexp)
		r.Patterns[i].compiled = re
	}
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

func TestRecorder_GetInteractionStats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-stats-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)
	r.SessionID = "20260215-120000-12345"

	// Create some dummy interactions
	files := []string{
		"interactions/20260215-120000-12345/self/setup/0001-12-00-01.000-GET.http",
		"interactions/20260215-120000-12345/upstream/marge/0002-12-00-02.000-POST.http",
		"interactions/20260215-130000-67890/self/setup/0001-13-00-01.000-GET.http",
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f)
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte("test"), 0644)
	}

	stats, err := r.GetInteractionStats()
	if err != nil {
		t.Fatalf("GetInteractionStats failed: %v", err)
	}

	if stats.TotalRequests != 3 {
		t.Errorf("Expected 3 total requests, got %d", stats.TotalRequests)
	}

	if stats.ByService["self"] != 2 {
		t.Errorf("Expected 2 self requests, got %d", stats.ByService["self"])
	}

	if stats.ByService["upstream"] != 1 {
		t.Errorf("Expected 1 upstream request, got %d", stats.ByService["upstream"])
	}

	if stats.BySession["20260215-120000-12345"] != 2 {
		t.Errorf("Expected 2 requests for session 1, got %d", stats.BySession["20260215-120000-12345"])
	}
}

func TestRecorder_ListInteractions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-list-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)
	session1 := "20260215-120000-12345"
	session2 := "20260215-130000-67890"

	// Create some dummy interactions
	files := []struct {
		path    string
		content string
	}{
		{
			path:    filepath.Join("interactions", session1, "self", "setup", "0001-12-00-01.555-GET.http"),
			content: "### GET /setup\n\n> {% \n    // Response: 200 OK\n%}\n",
		},
		{
			path:    filepath.Join("interactions", session1, "upstream", "marge", "0002-12-00-02.000-POST.http"),
			content: "### POST /marge\n\n> {% \n    // Response: 201 Created\n%}\n",
		},
		{
			path:    filepath.Join("interactions", session2, "self", "info", "0001-13-00-05.000-GET.http"),
			content: "### GET /info\n\n> {% \n    // Response: 404 Not Found\n%}\n",
		},
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f.path)
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(f.content), 0644)
	}

	t.Run("List_all", func(t *testing.T) {
		list, err := r.ListInteractions("", "", "")
		if err != nil {
			t.Fatalf("ListInteractions failed: %v", err)
		}
		if len(list) != 3 {
			t.Errorf("Expected 3 interactions, got %d", len(list))
		}
	})

	t.Run("Filter_by_session", func(t *testing.T) {
		list, err := r.ListInteractions(session1, "", "")
		if err != nil {
			t.Fatalf("ListInteractions failed: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("Expected 2 interactions for session1, got %d", len(list))
		}
		for _, i := range list {
			if i.Session != session1 {
				t.Errorf("Expected session %s, got %s", session1, i.Session)
			}
		}
	})

	t.Run("Filter_by_category", func(t *testing.T) {
		list, err := r.ListInteractions("", "upstream", "")
		if err != nil {
			t.Fatalf("ListInteractions failed: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("Expected 1 upstream interaction, got %d", len(list))
		}
		if list[0].Category != "upstream" {
			t.Errorf("Expected category upstream, got %s", list[0].Category)
		}
	})

	t.Run("Check_enhanced_fields", func(t *testing.T) {
		list, err := r.ListInteractions(session1, "self", "")
		if err != nil {
			t.Fatalf("ListInteractions failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("Expected at least one interaction")
		}
		i := list[0]
		if i.Counter != 1 {
			t.Errorf("Expected counter 1, got %d", i.Counter)
		}
		if i.Status != 200 {
			t.Errorf("Expected status 200, got %d", i.Status)
		}
		if i.Method != "GET" {
			t.Errorf("Expected method GET, got %s", i.Method)
		}
		if i.Timestamp != "2026-02-15 12:00:01.555" {
			t.Errorf("Expected timestamp 2026-02-15 12:00:01.555, got %s", i.Timestamp)
		}
		if i.Path != "/setup" {
			t.Errorf("Expected path /setup, got %s", i.Path)
		}
	})

	t.Run("Filter_by_since", func(t *testing.T) {
		// session1 has 2026-02-15 12:00:01.555 and 12:00:02.000
		// session2 has 2026-02-15 13:00:05.000
		list, err := r.ListInteractions("", "", "2026-02-15 12:30:00")
		if err != nil {
			t.Fatalf("ListInteractions failed: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("Expected 1 interaction since 12:30:00, got %d", len(list))
		}
		if list[0].Session != session2 {
			t.Errorf("Expected session2, got %s", list[0].Session)
		}

		list, err = r.ListInteractions("", "", "2026-02-15 12:00:01.600")
		if err != nil {
			t.Fatalf("ListInteractions failed: %v", err)
		}
		// Should include 12:00:02.000 and 13:00:05.000
		if len(list) != 2 {
			t.Errorf("Expected 2 interactions since 12:00:01.600, got %d", len(list))
		}
	})
}

func TestRecorder_GetInteractionContent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-content-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)
	relPath := filepath.Join(r.SessionID, "self", "test", "0001-GET.http")
	fullPath := filepath.Join(tmpDir, "interactions", relPath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)

	expectedContent := "test content"
	os.WriteFile(fullPath, []byte(expectedContent), 0644)

	content, err := r.GetInteractionContent(relPath)
	if err != nil {
		t.Fatalf("GetInteractionContent failed: %v", err)
	}

	if string(content) != expectedContent {
		t.Errorf("Expected %s, got %s", expectedContent, string(content))
	}

	_, err = r.GetInteractionContent("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestRecorder_Record_FullExchange(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-full-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)

	req := &http.Request{
		Method: "POST",
		URL: &url.URL{
			Path: "/test",
		},
		Header: make(http.Header),
		Body:   io.NopCloser(strings.NewReader("request body")),
	}
	req.Header.Set("Content-Type", "text/plain")

	res := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("response body")),
		Request:    req,
	}
	res.Header.Set("Content-Type", "application/json")

	err = r.Record("self", req, res)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	// Verify file content
	interactionsDir := filepath.Join(tmpDir, "interactions", r.SessionID, "self", "test")
	files, _ := os.ReadDir(interactionsDir)
	if len(files) == 0 {
		t.Fatal("No recording file found")
	}

	content, _ := os.ReadFile(filepath.Join(interactionsDir, files[0].Name()))
	contentStr := string(content)

	if !strings.Contains(contentStr, "request body") {
		t.Error("Recording does not contain request body")
	}
	if !strings.Contains(contentStr, "Response: 200 OK") {
		t.Error("Recording does not contain response status")
	}
	if !strings.Contains(contentStr, "response body") {
		t.Error("Recording does not contain response body")
	}
}

func TestRecorder_Record_BinaryResponse(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-binary-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)

	req := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/image"},
	}

	res := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBuffer([]byte{0x00, 0x01, 0x02, 0x03})),
		Request:    req,
	}
	res.Header.Set("Content-Type", "image/png")

	err = r.Record("self", req, res)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	interactionsDir := filepath.Join(tmpDir, "interactions", r.SessionID, "self", "image")
	files, _ := os.ReadDir(interactionsDir)
	content, _ := os.ReadFile(filepath.Join(interactionsDir, files[0].Name()))
	contentStr := string(content)

	if !strings.Contains(contentStr, "[Binary response body: 4 bytes]") {
		t.Error("Recording does not correctly report binary response")
	}
}

func TestRecorder_ListInteractions_FullTimestamp(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder-full-ts-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	r := NewRecorder(tmpDir)
	sessionID := "20260215-100000-12345"
	r.SessionID = sessionID

	// Create some dummy recordings
	basePath := filepath.Join(tmpDir, "interactions", sessionID, "self", "test")
	os.MkdirAll(basePath, 0755)

	files := []string{
		"0001-10-00-01.000-GET.http",
		"0002-11-00-00.000-GET.http",
	}

	for _, f := range files {
		os.WriteFile(filepath.Join(basePath, f), []byte("test"), 0644)
	}

	t.Run("Check_Full_Timestamp_Display", func(t *testing.T) {
		interactions, err := r.ListInteractions(sessionID, "", "")
		if err != nil {
			t.Fatalf("ListInteractions failed: %v", err)
		}

		if len(interactions) != 2 {
			t.Fatalf("Expected 2 interactions, got %d", len(interactions))
		}

		expectedTS := "2026-02-15 10:00:01.000"
		if interactions[0].Timestamp != expectedTS {
			t.Errorf("Expected timestamp %s, got %s", expectedTS, interactions[0].Timestamp)
		}
	})

	t.Run("Filter_By_Full_Date_Time", func(t *testing.T) {
		// Filter for interactions since 10:30:00 on that day
		interactions, err := r.ListInteractions(sessionID, "", "2026-02-15 10:30:00")
		if err != nil {
			t.Fatalf("ListInteractions failed: %v", err)
		}

		if len(interactions) != 1 {
			t.Fatalf("Expected 1 interaction, got %d", len(interactions))
		}

		if interactions[0].ID != "0002-11-00-00.000-GET.http" {
			t.Errorf("Expected 0002-..., got %s", interactions[0].ID)
		}
	})

	t.Run("Filter_By_Date_Only", func(t *testing.T) {
		// Filter for interactions since the day before
		interactions, err := r.ListInteractions(sessionID, "", "2026-02-14")
		if err != nil {
			t.Fatalf("ListInteractions failed: %v", err)
		}

		if len(interactions) != 2 {
			t.Fatalf("Expected 2 interactions, got %d", len(interactions))
		}
	})
}
