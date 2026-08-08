package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/certmanager"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

// TestAdminAreaAuthGate is the wiring-level regression test for #419: it
// exercises the real production router (setupRouter), not just the
// BasicAuthAdmin middleware in isolation, to pin two things at once:
//  1. /admin and /api/setup/* (and their /setup/* legacy aliases) are open
//     by default and become gated once AdminAreaAuth is "enabled".
//  2. A handful of routes deliberately stay reachable WITHOUT credentials
//     regardless of the gate: ca.crt/tts/speak/tts/config because
//     soundtouch-cli/soundtouch-player call them directly (the whole reason
//     mountSetupAPI was split into mountSetupAPIShared/mountSetupAPIAdmin),
//     and /api/announcements because it specifically needs to reach
//     operators who haven't set up credentials yet.
func TestAdminAreaAuthGate(t *testing.T) {
	tempDir := t.TempDir()

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	// A real setup.Manager (with an actual CA) so /setup/ca.crt genuinely
	// succeeds instead of failing on a nil dependency for an unrelated
	// reason, which would make the "stays reachable" assertion meaningless.
	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	_ = cm.EnsureCA()
	sm := setup.NewManager("http://localhost:8000", ds, cm)

	server := handlers.NewServer(ds, sm, "http://localhost:8000", true, false, false)
	server.SetMgmtConfig("custom-admin", "custom-password")

	r := setupRouter(server, nil, nil)
	ts := httptest.NewServer(r)
	defer ts.Close()

	adminGatedPaths := []string{
		"/admin",
		"/setup/settings",
		"/api/setup/settings",
	}
	alwaysUngatedPaths := []string{
		"/setup/ca.crt",
		"/api/setup/ca.crt",
		"/setup/tts/config",
		"/api/setup/tts/config",
		"/api/announcements?target=admin",
	}

	t.Run("open by default (AdminAreaAuth unset)", func(t *testing.T) {
		for _, path := range adminGatedPaths {
			status := getStatus(t, ts.URL, path, "", "")
			if status == http.StatusUnauthorized {
				t.Errorf("%s: expected open access by default, got 401", path)
			}
		}
	})

	server.SetAdminAreaAuth("enabled")
	defer server.SetAdminAreaAuth("")

	t.Run("gated paths reject without credentials once enabled", func(t *testing.T) {
		for _, path := range adminGatedPaths {
			status := getStatus(t, ts.URL, path, "", "")
			if status != http.StatusUnauthorized {
				t.Errorf("%s: expected 401 without credentials once enabled, got %d", path, status)
			}
		}
	})

	t.Run("gated paths accept correct credentials once enabled", func(t *testing.T) {
		for _, path := range adminGatedPaths {
			status := getStatus(t, ts.URL, path, "custom-admin", "custom-password")
			if status == http.StatusUnauthorized {
				t.Errorf("%s: expected access with correct credentials, got 401", path)
			}
		}
	})

	t.Run("routes intentionally left outside the gate stay reachable without credentials", func(t *testing.T) {
		for _, path := range alwaysUngatedPaths {
			status := getStatus(t, ts.URL, path, "", "")
			if status != http.StatusOK {
				t.Errorf("%s: expected 200 without credentials even with the gate enabled, got %d", path, status)
			}
		}
	})
}

func getStatus(t *testing.T, base, path, user, pass string) int {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatalf("Failed to build request for %s: %v", path, err)
	}
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request to %s failed: %v", path, err)
	}
	defer res.Body.Close()

	return res.StatusCode
}
