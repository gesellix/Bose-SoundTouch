package stockholm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Config holds the parsed stockholm/json/config.json values.
type Config struct {
	AppVersion           string
	ProtocolVersion      string
	StreamingVersion     string
	CustomerVersion      string
	DefaultMargeURL      string
	DefaultUpdateURL     string
	BmxRegistryURL       string
	AuthServiceURL       string
	EncryptedBmxToken    string
	MargeServerKey       string
	MargeServerKeyHeader string
	// BasePath is an optional URL prefix under which the Stockholm frontend is
	// served (e.g. "/stockholm"). Empty means served at "/".
	BasePath string
}

// BackendConfig holds the parsed backend/config/backend-config.json values.
type BackendConfig struct {
	FrontendLoggingLevel int `json:"frontendLoggingLevel"`
}

var versionPrefix = regexp.MustCompile(`^(\d+(?:\.\d+)+)`)

// LoadConfig reads and parses stockholm/json/config.json from stockholmDir.
func LoadConfig(stockholmDir string) (*Config, error) {
	path := filepath.Join(stockholmDir, "json", "config.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config.json: %w", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config.json: %w", err)
	}

	appVersions := jsonObject(raw["app_versions"])
	apiVersions := jsonObject(raw["api_versions"])
	defaults := jsonObject(raw["default"])

	cfg := &Config{
		AppVersion:           jsonString(appVersions["bose_app"]),
		ProtocolVersion:      jsonString(appVersions["bose_protocol"]),
		StreamingVersion:     firstNonEmpty(jsonString(apiVersions["bose_streaming"]), "1.0"),
		CustomerVersion:      firstNonEmpty(jsonString(apiVersions["bose_customer"]), "1.0"),
		DefaultMargeURL:      normalizeBaseURL(decodeB64(jsonString(defaults["d0"]))),
		DefaultUpdateURL:     normalizeBaseURL(decodeB64(jsonString(defaults["d1"]))),
		BmxRegistryURL:       decodeB64(jsonString(defaults["d3"])),
		AuthServiceURL:       decodeB64(jsonString(defaults["d6"])),
		EncryptedBmxToken:    decodeB64(jsonString(defaults["d7"])),
		MargeServerKey:       decodeB64(jsonString(defaults["d10"])),
		MargeServerKeyHeader: decodeB64(jsonString(defaults["d13"])),
	}

	return cfg, nil
}

// RewriteConfigURLs updates the base64-encoded URL fields in stockholm/json/config.json
// to point at backendURL. margeURL is used for streaming.bose.com rewrites; if empty it
// defaults to backendURL. Set margeURL to backendURL+"/marge" when using soundcork.
// authServiceURL is written into d6 (the auth endpoint); if empty it defaults to
// backendURL. A trailing slash is always ensured because the JS concatenates paths like
// "oauth/account/..." directly onto this value.
func RewriteConfigURLs(stockholmDir, backendURL, margeURL, authServiceURL string) error {
	if margeURL == "" {
		margeURL = backendURL
	}

	if authServiceURL == "" {
		authServiceURL = backendURL
	}

	// Ensure trailing slash so JS path concatenation (e.g. d6 + "oauth/account/...")
	// produces a valid URL.
	if authServiceURL != "" && !strings.HasSuffix(authServiceURL, "/") {
		authServiceURL += "/"
	}

	path := filepath.Join(stockholmDir, "json", "config.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config.json: %w", err)
	}

	var raw map[string]json.RawMessage

	if unmarshalErr := json.Unmarshal(data, &raw); unmarshalErr != nil {
		return fmt.Errorf("parse config.json: %w", unmarshalErr)
	}

	defaults := jsonObject(raw["default"])

	// Decode each field, substitute known hostnames, re-encode
	replacements := map[string]string{
		"https://streaming.bose.com":    margeURL,
		"https://events.api.bosecm.com": backendURL,
		"https://content.api.bose.io":   backendURL,
		"https://worldwide.bose.com":    backendURL,
		"https://downloads.bose.com":    backendURL,
	}

	for key, rawVal := range defaults {
		decoded := decodeB64(jsonString(rawVal))

		for old, newVal := range replacements {
			decoded = strings.ReplaceAll(decoded, old, newVal)
		}

		defaults[key] = jsonRawString(base64.StdEncoding.EncodeToString([]byte(decoded)))
	}

	// d6 = auth service base URL; always overwrite with a full URL so the JS
	// does not fall back to treating it as a subdomain prefix.
	defaults["d6"] = jsonRawString(base64.StdEncoding.EncodeToString([]byte(authServiceURL)))

	// Re-serialize defaults back into the raw map
	encoded, err := json.Marshal(defaults)
	if err != nil {
		return fmt.Errorf("marshal defaults: %w", err)
	}

	raw["default"] = json.RawMessage(encoded)

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config.json: %w", err)
	}

	return os.WriteFile(path, append(out, '\n'), 0644)
}

// LoadBackendConfig reads backend/config/backend-config.json from workspaceRoot.
// Returns defaults if the file is absent.
func LoadBackendConfig(workspaceRoot string) *BackendConfig {
	path := filepath.Join(workspaceRoot, "backend", "config", "backend-config.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return &BackendConfig{}
	}

	var cfg BackendConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &BackendConfig{}
	}

	if cfg.FrontendLoggingLevel < 0 {
		cfg.FrontendLoggingLevel = 0
	}

	return &cfg
}

// StreamingMediaType returns the Accept/Content-Type for streaming API calls.
func (c *Config) StreamingMediaType() string {
	return "application/vnd.bose.streaming-v" + c.StreamingVersion + "+xml"
}

// CustomerMediaType returns the Accept/Content-Type for customer API calls.
func (c *Config) CustomerMediaType() string {
	return "application/vnd.bose.customer-v" + c.CustomerVersion + "+xml"
}

// MediaTypeForPath returns the appropriate media type based on path.
func (c *Config) MediaTypeForPath(path string) string {
	p := strings.ToLower(path)
	if strings.Contains(p, "/customer/") {
		return c.CustomerMediaType()
	}

	if strings.Contains(p, "/streaming/") {
		return c.StreamingMediaType()
	}

	return "application/xml"
}

// IsBmxTarget returns true if host is a BMX API target.
func (c *Config) IsBmxTarget(host string) bool {
	h := strings.ToLower(host)

	return h == "content.api.bose.io" ||
		h == "test.content.api.bose.io" ||
		h == "bose-prod.apigee.net" ||
		strings.HasSuffix(h, ".apigee.net")
}

// IsMargeTarget returns true if host+path is a Marge streaming/customer endpoint.
func (c *Config) IsMargeTarget(host, path string) bool {
	h := strings.ToLower(host)

	p := strings.ToLower(path)
	if !strings.Contains(p, "/streaming/") && !strings.Contains(p, "/customer/") {
		return false
	}

	return strings.HasSuffix(h, ".bose.com") || strings.HasSuffix(h, ".apigee.net")
}

// ExtractVersionPrefix returns the leading version number (e.g. "27.0" from "27.0.13-xyz").
func ExtractVersionPrefix(v string) string {
	m := versionPrefix.FindStringSubmatch(v)
	if len(m) >= 2 {
		return m[1]
	}

	return ""
}

// ShouldEnableFrontendDebug returns true when logging level > 0.
func (b *BackendConfig) ShouldEnableFrontendDebug() bool {
	return b.FrontendLoggingLevel > 0
}

// helpers

func decodeB64(s string) string {
	if s == "" {
		return ""
	}

	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// Some values may not be base64 (already plain), return as-is
		return s
	}

	return string(b)
}

func normalizeBaseURL(s string) string {
	if s == "" {
		return ""
	}

	if !strings.HasSuffix(s, "/") {
		return s + "/"
	}

	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}

	return ""
}

func jsonObject(raw json.RawMessage) map[string]json.RawMessage {
	if raw == nil {
		return nil
	}

	var obj map[string]json.RawMessage

	_ = json.Unmarshal(raw, &obj)

	return obj
}

func jsonString(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}

	return s
}

func jsonRawString(s string) json.RawMessage {
	// json.Marshal on a string never fails
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}

	return json.RawMessage(b)
}
