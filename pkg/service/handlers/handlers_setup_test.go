package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/certmanager"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/health"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

func TestProxySettingsAPI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging-settings-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	r, server := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Initial State
	server.redactLogs = true
	server.logBodies = false

	// 1. Test GET
	res, err := http.Get(ts.URL + "/setup/logging-settings")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("GET: Expected status OK, got %v", res.Status)
	}

	var settings map[string]bool
	if decodeErr := json.NewDecoder(res.Body).Decode(&settings); decodeErr != nil {
		t.Fatalf("GET: Failed to decode response: %v", decodeErr)
	}

	if settings["redact"] != true || settings["log_body"] != false {
		t.Errorf("GET: Unexpected settings: %+v", settings)
	}

	// 2. Test POST
	update := map[string]bool{
		"redact":   false,
		"log_body": true,
	}

	body, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("Failed to marshal update data: %v", err)
	}

	res, err = http.Post(ts.URL+"/setup/logging-settings", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("POST: Expected status OK, got %v", res.Status)
	}

	// Verify server state
	if server.redactLogs != false || server.logBodies != true {
		t.Errorf("POST: Server state did not update: redact=%v, logBody=%v", server.redactLogs, server.logBodies)
	}

	res, err = http.Get(ts.URL + "/setup/logging-settings")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if err := json.NewDecoder(res.Body).Decode(&settings); err != nil {
		t.Fatalf("GET (after update): Failed to decode response: %v", err)
	}

	if settings["redact"] != false || settings["log_body"] != true {
		t.Errorf("GET (after update): Unexpected settings: %+v", settings)
	}

	// 3. Test System Settings POST
	sysUpdate := map[string]string{
		"server_url": "http://127.0.0.1:8000",
	}

	sysBody, err := json.Marshal(sysUpdate)
	if err != nil {
		t.Fatalf("Failed to marshal system settings data: %v", err)
	}

	res, err = http.Post(ts.URL+"/setup/settings", "application/json", bytes.NewBuffer(sysBody))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("POST /setup/settings: Expected status OK, got %v", res.Status)
	}

	// Verify server state
	sURL, _ := server.GetSettings()
	if sURL != "http://127.0.0.1:8000" {
		t.Errorf("POST /setup/settings: Server state did not update: serverURL=%s", sURL)
	}

	// 4. Test internal paths persistence
	pathUpdate := map[string]interface{}{
		"server_url":     "http://127.0.0.1:8000",
		"internal_paths": []string{"/setup/*"},
	}

	pathBody, err := json.Marshal(pathUpdate)
	if err != nil {
		t.Fatalf("Failed to marshal path settings: %v", err)
	}
	res, err = http.Post(ts.URL+"/setup/settings", "application/json", bytes.NewBuffer(pathBody))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("POST /setup/settings (paths): Expected status OK, got %v", res.Status)
	}

	// Verify server state
	server.mu.RLock()
	iPaths := server.internalPaths
	server.mu.RUnlock()

	if len(iPaths) != 1 || iPaths[0] != "/setup/*" {
		t.Errorf("POST /setup/settings (paths): Internal paths did not update: %v", iPaths)
	}

	// Verify persistence in datastore
	persisted, _ := ds.GetSettings()
	if len(persisted.InternalPaths) != 1 || persisted.InternalPaths[0] != "/setup/*" {
		t.Errorf("POST /setup/settings (paths): Datastore internal paths did not update: %+v", persisted)
	}
}

// TestSettingsSavePreservesUnmanagedFields is the regression test for
// issue #589: saving settings via either the main settings form or the
// logging/proxy panel must not drop fields that have no counterpart in
// their respective request DTOs (e.g. hand-edited trust_forwarded_headers /
// trusted_proxy_cidrs, or the other handler's owned fields).
func TestSettingsSavePreservesUnmanagedFields(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "settings-preserve-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	// Seed settings.json with fields neither handler's DTO exposes.
	seeded := datastore.Settings{
		ServerURL:             "http://127.0.0.1:8000",
		TrustForwardedHeaders: true,
		TrustedProxyCIDRs:     []string{"10.42.0.0/16"},
	}
	if err := ds.SaveSettings(seeded); err != nil {
		t.Fatalf("Failed to seed settings: %v", err)
	}

	r, server := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Simulate a credential already loaded into the running server (as
	// main.go's startup wiring does) but not managed by the logging panel's
	// DTO, to catch HandleUpdateLoggingSettings resetting fields it doesn't
	// own back to their zero value.
	server.spotifyClientID = "seeded-spotify-client-id"

	// Saving the main settings form (which knows nothing about
	// trust_forwarded_headers / trusted_proxy_cidrs) must not drop them.
	sysUpdate := map[string]string{"server_url": "http://127.0.0.1:8000"}
	sysBody, err := json.Marshal(sysUpdate)
	if err != nil {
		t.Fatalf("Failed to marshal update: %v", err)
	}

	res, err := http.Post(ts.URL+"/setup/settings", "application/json", bytes.NewBuffer(sysBody))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/settings: expected status OK, got %v", res.Status)
	}

	persisted, err := ds.GetSettings()
	if err != nil {
		t.Fatalf("Failed to reload settings: %v", err)
	}
	if !persisted.TrustForwardedHeaders {
		t.Errorf("POST /setup/settings dropped TrustForwardedHeaders: %+v", persisted)
	}
	if len(persisted.TrustedProxyCIDRs) != 1 || persisted.TrustedProxyCIDRs[0] != "10.42.0.0/16" {
		t.Errorf("POST /setup/settings dropped TrustedProxyCIDRs: %+v", persisted)
	}

	// Saving the logging/proxy panel (which only knows redact/log_body/record)
	// must not drop these fields, or the SpotifyClientID it also doesn't manage.
	logUpdate := map[string]bool{"redact": true, "log_body": true, "record": false}
	logBody, err := json.Marshal(logUpdate)
	if err != nil {
		t.Fatalf("Failed to marshal logging update: %v", err)
	}

	res, err = http.Post(ts.URL+"/setup/logging-settings", "application/json", bytes.NewBuffer(logBody))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/logging-settings: expected status OK, got %v", res.Status)
	}

	persisted, err = ds.GetSettings()
	if err != nil {
		t.Fatalf("Failed to reload settings: %v", err)
	}
	if !persisted.TrustForwardedHeaders {
		t.Errorf("POST /setup/logging-settings dropped TrustForwardedHeaders: %+v", persisted)
	}
	if len(persisted.TrustedProxyCIDRs) != 1 || persisted.TrustedProxyCIDRs[0] != "10.42.0.0/16" {
		t.Errorf("POST /setup/logging-settings dropped TrustedProxyCIDRs: %+v", persisted)
	}
	if persisted.SpotifyClientID != "seeded-spotify-client-id" {
		t.Errorf("POST /setup/logging-settings dropped SpotifyClientID: %+v", persisted)
	}
}

// TestAdminAreaAuthInvalidValue is a regression test for #419: an
// unrecognised admin_area_auth value must be rejected outright, not
// silently coerced to the unset default.
func TestAdminAreaAuthInvalidValue(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "admin-area-auth-invalid-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	r, _ := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	body, err := json.Marshal(map[string]string{
		"server_url":      "http://127.0.0.1:8000",
		"admin_area_auth": "sometimes",
	})
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	res, err := http.Post(ts.URL+"/setup/settings", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid admin_area_auth, got %v", res.Status)
	}

	persisted, err := ds.GetSettings()
	if err != nil {
		t.Fatalf("Failed to reload settings: %v", err)
	}
	if persisted.AdminAreaAuth != "" {
		t.Errorf("Invalid admin_area_auth must not be persisted, got %q", persisted.AdminAreaAuth)
	}
}

// TestAdminAreaAuthGuardRailBlocksDefaultCreds is a regression test for
// #419: enabling the admin-area gate while the Management API credentials
// are still the published default (admin/change_me!) must be rejected —
// otherwise the gate would give a false sense of security.
func TestAdminAreaAuthGuardRailBlocksDefaultCreds(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "admin-area-auth-guard-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	r, server := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	server.mgmtUsername = health.DefaultMgmtUsername
	server.mgmtPassword = health.DefaultMgmtPassword

	body, err := json.Marshal(map[string]string{
		"server_url":      "http://127.0.0.1:8000",
		"admin_area_auth": "enabled",
	})
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	res, err := http.Post(ts.URL+"/setup/settings", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400 when enabling admin_area_auth with default creds, got %v", res.Status)
	}

	if server.AdminAreaAuthMode() != "" {
		t.Errorf("Guard rail must not flip the live mode, got %q", server.AdminAreaAuthMode())
	}

	persisted, err := ds.GetSettings()
	if err != nil {
		t.Fatalf("Failed to reload settings: %v", err)
	}
	if persisted.AdminAreaAuth != "" {
		t.Errorf("Guard rail must not persist the change, got %q", persisted.AdminAreaAuth)
	}
}

// TestAdminAreaAuthRoundTrip verifies enabling (with non-default creds) and
// later disabling admin_area_auth updates both the live server field and
// the persisted settings.json, and is reflected back by GET /setup/settings.
func TestAdminAreaAuthRoundTrip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "admin-area-auth-roundtrip-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	r, server := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	server.mgmtUsername = "custom-admin"
	server.mgmtPassword = "custom-password"

	enableBody, err := json.Marshal(map[string]string{
		"server_url":      "http://127.0.0.1:8000",
		"admin_area_auth": "Enabled", // mixed case must normalise
	})
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	res, err := http.Post(ts.URL+"/setup/settings", "application/json", bytes.NewBuffer(enableBody))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/settings (enable): expected 200, got %v", res.Status)
	}

	if server.AdminAreaAuthMode() != "enabled" {
		t.Errorf("Expected live mode \"enabled\", got %q", server.AdminAreaAuthMode())
	}

	persisted, err := ds.GetSettings()
	if err != nil {
		t.Fatalf("Failed to reload settings: %v", err)
	}
	if persisted.AdminAreaAuth != "enabled" {
		t.Errorf("Expected persisted admin_area_auth \"enabled\", got %q", persisted.AdminAreaAuth)
	}

	res, err = http.Get(ts.URL + "/setup/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	var got map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("Failed to decode GET /setup/settings: %v", err)
	}
	if got["admin_area_auth"] != "enabled" {
		t.Errorf("GET /setup/settings: expected admin_area_auth \"enabled\", got %+v", got["admin_area_auth"])
	}

	disableBody, err := json.Marshal(map[string]string{
		"server_url":      "http://127.0.0.1:8000",
		"admin_area_auth": "disabled",
	})
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	res, err = http.Post(ts.URL+"/setup/settings", "application/json", bytes.NewBuffer(disableBody))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/settings (disable): expected 200, got %v", res.Status)
	}

	if server.AdminAreaAuthMode() != "disabled" {
		t.Errorf("Expected live mode \"disabled\" after explicit opt-out, got %q", server.AdminAreaAuthMode())
	}
}

// TestResolvePeriodicSetting covers the shared interval/enabled resolution
// used by both background pollers (device discovery, update check).
func TestResolvePeriodicSetting(t *testing.T) {
	cases := []struct {
		name             string
		current          time.Duration
		requested        time.Duration
		provided         bool
		enabled          bool
		wantInterval     time.Duration
		wantEnabledState bool
	}{
		{"interval omitted keeps the current one", 24 * time.Hour, 0, false, true, 24 * time.Hour, true},
		{"interval supplied replaces the current one", 24 * time.Hour, 6 * time.Hour, true, true, 6 * time.Hour, true},
		{"disabling keeps the interval", 24 * time.Hour, 0, false, false, 24 * time.Hour, false},
		{"a zero interval forces it off", 24 * time.Hour, 0, true, true, 0, false},
		{"a zero current interval forces it off too", 0, 0, false, true, 0, false},
	}

	for _, tc := range cases {
		gotInterval, gotEnabled := resolvePeriodicSetting(tc.current, tc.requested, tc.provided, tc.enabled)
		if gotInterval != tc.wantInterval || gotEnabled != tc.wantEnabledState {
			t.Errorf("%s: resolvePeriodicSetting() = %v/%v, want %v/%v",
				tc.name, gotInterval, gotEnabled, tc.wantInterval, tc.wantEnabledState)
		}
	}
}

// TestParseOptionalDuration verifies an omitted duration is not an error,
// while a supplied-but-invalid one is.
func TestParseOptionalDuration(t *testing.T) {
	if d, err := parseOptionalDuration(""); err != nil || d != 0 {
		t.Errorf("parseOptionalDuration(\"\") = %v/%v, want 0/nil", d, err)
	}

	if d, err := parseOptionalDuration("90m"); err != nil || d != 90*time.Minute {
		t.Errorf("parseOptionalDuration(\"90m\") = %v/%v, want 1h30m0s/nil", d, err)
	}

	if _, err := parseOptionalDuration("nope"); err == nil {
		t.Error("parseOptionalDuration(\"nope\") = nil error, want a parse error")
	}
}

// TestUpdateCheckSettingsRoundTrip covers the Settings-page control for the
// opt-in update check (#591 follow-up): POST /setup/settings must update the
// live values the background poller reads, persist them, and hand them back
// on GET so the UI reflects what was saved.
func TestUpdateCheckSettingsRoundTrip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-check-settings-roundtrip-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	r, server := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Default state: opted out, with a nonzero interval so enabling it later
	// doesn't need an interval to be supplied.
	if interval, enabled := server.GetUpdateCheckSettings(); enabled || interval == 0 {
		t.Fatalf("Expected the check to default to disabled with a nonzero interval, got %v/%v", interval, enabled)
	}

	enableBody, err := json.Marshal(map[string]interface{}{
		"server_url":            "http://127.0.0.1:8000",
		"update_check_enabled":  true,
		"update_check_interval": "6h",
	})
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	res, err := http.Post(ts.URL+"/setup/settings", "application/json", bytes.NewBuffer(enableBody))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/settings (enable): expected 200, got %v", res.Status)
	}

	interval, enabled := server.GetUpdateCheckSettings()
	if !enabled || interval != 6*time.Hour {
		t.Errorf("Expected live settings 6h/true, got %v/%v", interval, enabled)
	}

	persisted, err := ds.GetSettings()
	if err != nil {
		t.Fatalf("Failed to reload settings: %v", err)
	}
	if !persisted.UpdateCheckEnabled || persisted.UpdateCheckInterval != "6h0m0s" {
		t.Errorf("Expected persisted 6h0m0s/true, got %q/%v",
			persisted.UpdateCheckInterval, persisted.UpdateCheckEnabled)
	}

	res, err = http.Get(ts.URL + "/setup/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	var got map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("Failed to decode GET /setup/settings: %v", err)
	}
	if got["update_check_enabled"] != true {
		t.Errorf("GET /setup/settings: expected update_check_enabled true, got %+v", got["update_check_enabled"])
	}
	if got["update_check_interval"] != "6h0m0s" {
		t.Errorf("GET /setup/settings: expected update_check_interval 6h0m0s, got %+v", got["update_check_interval"])
	}

	// An unparseable interval must be rejected before anything is applied.
	badBody, err := json.Marshal(map[string]interface{}{
		"server_url":            "http://127.0.0.1:8000",
		"update_check_enabled":  true,
		"update_check_interval": "not-a-duration",
	})
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	res, err = http.Post(ts.URL+"/setup/settings", "application/json", bytes.NewBuffer(badBody))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("POST /setup/settings (bad interval): expected 400, got %v", res.Status)
	}

	// A zero interval must force the check off rather than leave the poller
	// hitting GitHub on every tick.
	zeroBody, err := json.Marshal(map[string]interface{}{
		"server_url":            "http://127.0.0.1:8000",
		"update_check_enabled":  true,
		"update_check_interval": "0s",
	})
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	res, err = http.Post(ts.URL+"/setup/settings", "application/json", bytes.NewBuffer(zeroBody))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /setup/settings (zero interval): expected 200, got %v", res.Status)
	}

	if _, enabled := server.GetUpdateCheckSettings(); enabled {
		t.Error("Expected a zero interval to disable the update check")
	}
}

// TestHandleGetVersionInfo_IncludesAbsoluteDataDir verifies /api/setup/version
// reports the actual data directory in use, resolved to an absolute path —
// added so operators running the service locally (not in Docker, where the
// path is obvious from the bind mount) can find it without having to
// inspect the running process. See NEXT.md/#419 session notes.
func TestHandleGetVersionInfo_IncludesAbsoluteDataDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "version-info-datadir-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	r, _ := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/setup/version")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	var got map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	wantAbs, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatalf("Failed to resolve expected absolute path: %v", err)
	}

	if got["data_dir"] != wantAbs {
		t.Errorf("Expected data_dir %q, got %q", wantAbs, got["data_dir"])
	}
}

// TestHandleGetVersionInfo_UpdateCheckFields verifies the #591 fields are
// present and reflect a nil-checker default (Available: false) when the
// update check was never enabled — the common case.
func TestHandleGetVersionInfo_UpdateCheckFields(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "version-info-updatecheck-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	r, _ := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/setup/version")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	var got map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if got["update_available"] != false {
		t.Errorf("Expected update_available=false by default, got %v", got["update_available"])
	}
	if _, ok := got["latest_version"]; !ok {
		t.Error("Expected a latest_version key in the response")
	}
	if _, ok := got["latest_release_url"]; !ok {
		t.Error("Expected a latest_release_url key in the response")
	}
}

func TestMigrationAndCA(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "handlers-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()
	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	_ = cm.EnsureCA()

	sm := setup.NewManager("http://localhost:8000", ds, cm)
	// Mock SSH to avoid real connections
	sm.NewSSH = func(host string) setup.SSHClient {
		return &mockSSH{host: host}
	}

	// Mock HTTPGet to avoid real network timeouts
	sm.HTTPGet = func(url string) (*http.Response, error) {
		if strings.HasSuffix(url, "/info") {
			xml := `<?xml version="1.0" encoding="UTF-8" ?><info deviceID="192.0.2.10"><name>Test Speaker</name><type>SoundTouch 10</type><margeAccountUUID>default</margeAccountUUID></info>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(xml)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("Not Found")),
		}, nil
	}

	r, server := setupRouter("http://localhost:8001", ds)
	server.sm = sm // Inject our manager with mock SSH

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Add device to datastore for resolution
	_ = ds.SaveDeviceInfo("default", "192.0.2.10", &models.ServiceDeviceInfo{
		DeviceID:  "192.0.2.10",
		IPAddress: "192.0.2.10",
		AccountID: "default",
	})

	// 1. Test GET /setup/ca.crt
	res, err := http.Get(ts.URL + "/setup/ca.crt")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("CA: Expected status OK, got %v", res.Status)
	}
	if res.Header.Get("Content-Type") != "application/x-x509-ca-cert" {
		t.Errorf("CA: Unexpected content type: %s", res.Header.Get("Content-Type"))
	}

	// 2. Test POST /setup/migrate/{deviceIP}?method=hosts
	res, err = http.Post(ts.URL+"/setup/migrate/192.0.2.10?method=hosts&target_url=http://192.0.2.100:8000", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Migrate: Expected status OK, got %v", res.Status)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("Migrate: Failed to decode response: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("Migrate: Expected ok=true, got %v", result["ok"])
	}
	if _, ok := result["output"]; !ok {
		t.Errorf("Migrate: Expected output field in response")
	}

	// 3. Test POST /setup/trust-ca/{deviceIP}
	res, err = http.Post(ts.URL+"/setup/trust-ca/192.0.2.10", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("TrustCA: Expected status OK, got %v", res.Status)
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("TrustCA: Failed to decode response: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("TrustCA: Expected ok=true, got %v", result["ok"])
	}
	if _, ok := result["output"]; !ok {
		t.Errorf("TrustCA: Expected output field in response")
	}

	// 4. Test POST /setup/reboot/{deviceIP}
	res, err = http.Post(ts.URL+"/setup/reboot/192.0.2.10", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Reboot: Expected status OK, got %v", res.Status)
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("Reboot: Failed to decode response: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("Reboot: Expected ok=true, got %v", result["ok"])
	}
	if _, ok := result["output"]; !ok {
		t.Errorf("Reboot: Expected output field in response")
	}

	// 5. Test POST /setup/remove-remote-services/{deviceIP}
	res, err = http.Post(ts.URL+"/setup/remove-remote-services/192.0.2.10", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("RemoveRemote: Expected status OK, got %v", res.Status)
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("RemoveRemote: Failed to decode response: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("RemoveRemote: Expected ok=true, got %v", result["ok"])
	}
	if _, ok := result["output"]; !ok {
		t.Errorf("RemoveRemote: Expected output field in response")
	}
}

func TestRemoveDevice(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "remove-device-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	// Setup a dummy device in the datastore
	account := "test-account"
	deviceID := "TEST-DEVICE-ID"
	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", deviceID)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("Failed to create device dir: %v", err)
	}

	infoFile := filepath.Join(deviceDir, "DeviceInfo.xml")
	infoXML := `<?xml version="1.0" encoding="UTF-8" ?><info deviceID="TEST-DEVICE-ID"><name>Test Device</name><type>SoundTouch 10</type></info>`
	if err := os.WriteFile(infoFile, []byte(infoXML), 0644); err != nil {
		t.Fatalf("Failed to create device info file: %v", err)
	}

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	// 1. Verify device exists
	res, err := http.Get(ts.URL + "/setup/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	var devices []map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&devices); err != nil {
		t.Fatalf("Failed to decode devices: %v", err)
	}

	found := false
	for _, d := range devices {
		if d["device_id"] == deviceID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Device not found in list before removal")
	}

	// 2. Remove device
	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/setup/devices/"+deviceID, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	// 3. Verify device is gone
	res, err = http.Get(ts.URL + "/setup/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if err := json.NewDecoder(res.Body).Decode(&devices); err != nil {
		t.Fatalf("Failed to decode devices after removal: %v", err)
	}

	for _, d := range devices {
		if d["device_id"] == deviceID {
			t.Errorf("Device still exists in list after removal")
		}
	}

	// 4. Verify directory is gone
	if _, err := os.Stat(deviceDir); !os.IsNotExist(err) {
		t.Errorf("Device directory still exists after removal")
	}
}

type mockSSH struct {
	host     string
	runCount int

	// uploaded mirrors UploadContent calls so that a subsequent
	// `cat <path>` (notably the tmp-readback step in
	// TrustCACertFromBytes) returns what we just wrote there.
	uploaded map[string][]byte
}

func (m *mockSSH) Run(command string) (string, error) {
	if strings.Contains(command, "cat /etc/hosts") {
		m.runCount++
		if m.runCount > 1 {
			// Return updated hosts for verification
			return "127.0.0.1 localhost\n192.0.2.100\tstreaming.bose.com\n192.0.2.100\tupdates.bose.com\n192.0.2.100\tstats.bose.com\n192.0.2.100\tbmx.bose.com\n192.0.2.100\tcontent.api.bose.io\n192.0.2.100\tevents.api.bosecm.com\n192.0.2.100\taudionotification.api.bosecm.com\n192.0.2.100\taudionotificationdev.api.bosecm.com\n192.0.2.100\tbose-prod.apigee.net\n192.0.2.100\tworldwide.bose.com\n192.0.2.100\tmedia.bose.io\n192.0.2.100\tdownloads.bose.com\n192.0.2.100\tvoice.api.bose.io", nil
		}
		return "127.0.0.1 localhost", nil
	}
	if strings.HasPrefix(command, "[ -f") {
		return "", nil // Pretend file exists for backups
	}
	if strings.HasPrefix(command, "grep -F") {
		return "matched", nil // CA trusted
	}
	if strings.HasPrefix(command, "cat ") {
		path := strings.TrimPrefix(command, "cat ")
		if body, ok := m.uploaded[path]; ok {
			return string(body), nil
		}
	}
	return "", nil
}

func (m *mockSSH) UploadContent(content []byte, remotePath string) error {
	if m.uploaded == nil {
		m.uploaded = make(map[string][]byte)
	}

	m.uploaded[remotePath] = append([]byte(nil), content...)

	return nil
}

// Connect/Close are no-ops here — the mock has no real connection to
// reuse, and every test call already goes through Run/UploadContent above
// regardless of whether Connect was called first.
func (m *mockSSH) Connect() error { return nil }
func (m *mockSSH) Close() error   { return nil }
