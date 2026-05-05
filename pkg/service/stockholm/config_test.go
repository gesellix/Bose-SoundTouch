package stockholm

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- ExtractVersionPrefix ----

func TestExtractVersionPrefix(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"27.0.13-release", "27.0.13"},
		{"27.0", "27.0"},
		{"1.2.3.4", "1.2.3.4"},
		{"v27.0", ""},
		{"", ""},
		{"release-27.0", ""},
	}

	for _, tc := range cases {
		if got := ExtractVersionPrefix(tc.input); got != tc.want {
			t.Errorf("ExtractVersionPrefix(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---- helper functions ----

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "third", "fourth"); got != "third" {
		t.Errorf("expected third, got %q", got)
	}

	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	if got := normalizeBaseURL("http://example.com"); got != "http://example.com/" {
		t.Errorf("expected trailing slash, got %q", got)
	}

	if got := normalizeBaseURL("http://example.com/"); got != "http://example.com/" {
		t.Errorf("expected no double slash, got %q", got)
	}

	if got := normalizeBaseURL(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestDecodeB64(t *testing.T) {
	original := "https://streaming.bose.com"
	encoded := base64.StdEncoding.EncodeToString([]byte(original))

	if got := decodeB64(encoded); got != original {
		t.Errorf("decodeB64 failed: got %q, want %q", got, original)
	}

	if got := decodeB64(""); got != "" {
		t.Errorf("expected empty for empty input, got %q", got)
	}

	// Not valid base64 → returned as-is
	if got := decodeB64("plain text"); got != "plain text" {
		t.Errorf("expected plain text returned as-is, got %q", got)
	}
}

// ---- LoadConfig / RewriteConfigURLs ----

func makeConfigJSON(t *testing.T, defaults map[string]string) string {
	t.Helper()

	// Encode each default value as base64
	encodedDefaults := make(map[string]interface{})
	for k, v := range defaults {
		encodedDefaults[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}

	raw := map[string]interface{}{
		"app_versions": map[string]interface{}{
			"bose_app":      "27.0.13-release",
			"bose_protocol": "1.0",
		},
		"api_versions": map[string]interface{}{
			"bose_streaming": "1.2",
			"bose_customer":  "1.3",
		},
		"default": encodedDefaults,
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	return string(data)
}

func writeStockholmConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	jsonDir := filepath.Join(dir, "json")

	if err := os.MkdirAll(jsonDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(jsonDir, "config.json"), []byte(content), 0644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}

	return dir
}

func TestLoadConfig_ParsesVersions(t *testing.T) {
	content := makeConfigJSON(t, map[string]string{
		"d0": "https://streaming.bose.com/marge/",
	})
	dir := writeStockholmConfig(t, content)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.AppVersion != "27.0.13-release" {
		t.Errorf("AppVersion = %q, want %q", cfg.AppVersion, "27.0.13-release")
	}

	if cfg.StreamingVersion != "1.2" {
		t.Errorf("StreamingVersion = %q, want %q", cfg.StreamingVersion, "1.2")
	}
}

func TestLoadConfig_DefaultsForMissingAPIVersions(t *testing.T) {
	// api_versions absent → defaults to "1.0"
	raw := map[string]interface{}{
		"app_versions": map[string]interface{}{
			"bose_app":      "27.0",
			"bose_protocol": "1.0",
		},
		"api_versions": map[string]interface{}{},
		"default":      map[string]interface{}{},
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	dir := writeStockholmConfig(t, string(data))

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.StreamingVersion != "1.0" {
		t.Errorf("expected default StreamingVersion=1.0, got %q", cfg.StreamingVersion)
	}
}

func TestRewriteConfigURLs_ReplacesHostnames(t *testing.T) {
	content := makeConfigJSON(t, map[string]string{
		"d0": "https://streaming.bose.com/marge/",
		"d1": "https://downloads.bose.com/updates/",
		"d3": "https://content.api.bose.io/registry",
	})
	dir := writeStockholmConfig(t, content)

	backendURL := "http://myserver:8000"
	if err := RewriteConfigURLs(dir, backendURL, backendURL, backendURL); err != nil {
		t.Fatalf("RewriteConfigURLs: %v", err)
	}

	// Reload and verify
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig after rewrite: %v", err)
	}

	if !strings.HasPrefix(cfg.DefaultMargeURL, backendURL) {
		t.Errorf("DefaultMargeURL = %q, expected prefix %q", cfg.DefaultMargeURL, backendURL)
	}

	if !strings.HasPrefix(cfg.DefaultUpdateURL, backendURL) {
		t.Errorf("DefaultUpdateURL = %q, expected prefix %q", cfg.DefaultUpdateURL, backendURL)
	}
}

func TestRewriteConfigURLs_MargeURLUsedForStreaming(t *testing.T) {
	content := makeConfigJSON(t, map[string]string{
		"d0": "https://streaming.bose.com/",
	})
	dir := writeStockholmConfig(t, content)

	backendURL := "http://backend:8000"
	margeURL := "http://backend:8000/marge"

	if err := RewriteConfigURLs(dir, backendURL, margeURL, backendURL); err != nil {
		t.Fatalf("RewriteConfigURLs: %v", err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig after rewrite: %v", err)
	}

	if !strings.HasPrefix(cfg.DefaultMargeURL, margeURL) {
		t.Errorf("DefaultMargeURL = %q, expected prefix %q", cfg.DefaultMargeURL, margeURL)
	}
}

func TestRewriteConfigURLs_AuthServiceURLHasTrailingSlash(t *testing.T) {
	content := makeConfigJSON(t, map[string]string{
		"d6": "oauth", // original Bose placeholder
	})
	dir := writeStockholmConfig(t, content)

	backendURL := "http://backend:8000"

	// Without trailing slash — function should add it.
	if err := RewriteConfigURLs(dir, backendURL, backendURL, backendURL); err != nil {
		t.Fatalf("RewriteConfigURLs: %v", err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig after rewrite: %v", err)
	}

	if !strings.HasSuffix(cfg.AuthServiceURL, "/") {
		t.Errorf("AuthServiceURL = %q, expected trailing slash", cfg.AuthServiceURL)
	}

	if !strings.HasPrefix(cfg.AuthServiceURL, backendURL) {
		t.Errorf("AuthServiceURL = %q, expected prefix %q", cfg.AuthServiceURL, backendURL)
	}
}

func TestRewriteConfigURLs_AuthServiceURL_ExplicitValue(t *testing.T) {
	content := makeConfigJSON(t, map[string]string{
		"d6": "oauth",
	})
	dir := writeStockholmConfig(t, content)

	backendURL := "http://backend:8000"
	authURL := "http://auth.backend:8001"

	if err := RewriteConfigURLs(dir, backendURL, backendURL, authURL); err != nil {
		t.Fatalf("RewriteConfigURLs: %v", err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig after rewrite: %v", err)
	}

	if !strings.HasPrefix(cfg.AuthServiceURL, authURL) {
		t.Errorf("AuthServiceURL = %q, expected prefix %q", cfg.AuthServiceURL, authURL)
	}

	if !strings.HasSuffix(cfg.AuthServiceURL, "/") {
		t.Errorf("AuthServiceURL = %q, expected trailing slash", cfg.AuthServiceURL)
	}
}

// ---- MediaType helpers ----

func TestStreamingMediaType(t *testing.T) {
	cfg := &Config{StreamingVersion: "1.2"}
	want := "application/vnd.bose.streaming-v1.2+xml"

	if got := cfg.StreamingMediaType(); got != want {
		t.Errorf("StreamingMediaType() = %q, want %q", got, want)
	}
}

func TestMediaTypeForPath(t *testing.T) {
	cfg := &Config{StreamingVersion: "1.2", CustomerVersion: "1.3"}

	cases := []struct {
		path string
		want string
	}{
		{"/customer/login", "application/vnd.bose.customer-v1.3+xml"},
		{"/streaming/content", "application/vnd.bose.streaming-v1.2+xml"},
		{"/info", "application/xml"},
	}

	for _, tc := range cases {
		if got := cfg.MediaTypeForPath(tc.path); got != tc.want {
			t.Errorf("MediaTypeForPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// ---- LoadBackendConfig ----

func TestLoadBackendConfig_MissingFile_ReturnsDefaults(t *testing.T) {
	cfg := LoadBackendConfig(t.TempDir())

	if cfg.FrontendLoggingLevel != 0 {
		t.Errorf("expected default FrontendLoggingLevel=0, got %d", cfg.FrontendLoggingLevel)
	}
}

func TestLoadBackendConfig_ParsesLevel(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "backend", "config")
	_ = os.MkdirAll(cfgDir, 0755)
	_ = os.WriteFile(filepath.Join(cfgDir, "backend-config.json"),
		[]byte(`{"frontendLoggingLevel": 3}`), 0644)

	cfg := LoadBackendConfig(dir)

	if cfg.FrontendLoggingLevel != 3 {
		t.Errorf("expected FrontendLoggingLevel=3, got %d", cfg.FrontendLoggingLevel)
	}

	if !cfg.ShouldEnableFrontendDebug() {
		t.Error("expected ShouldEnableFrontendDebug=true for level 3")
	}
}

func TestLoadBackendConfig_NegativeLevel_Clamped(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "backend", "config")
	_ = os.MkdirAll(cfgDir, 0755)
	_ = os.WriteFile(filepath.Join(cfgDir, "backend-config.json"),
		[]byte(`{"frontendLoggingLevel": -1}`), 0644)

	cfg := LoadBackendConfig(dir)

	if cfg.FrontendLoggingLevel != 0 {
		t.Errorf("expected clamped FrontendLoggingLevel=0, got %d", cfg.FrontendLoggingLevel)
	}
}
