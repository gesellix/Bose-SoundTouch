package updatecheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestNormalizeVersion(t *testing.T) {
	cases := []struct {
		in      string
		wantOK  bool
		wantOut string
	}{
		{"v1.2.3", true, "v1.2.3"},
		{"1.2.3", true, "v1.2.3"}, // missing "v" prefix gets added
		{"dev", false, ""},
		{"(devel)", false, ""},
		{"v0.120.1-0.20260808211626-abcdef123456+dirty", false, ""},
		{"", false, ""},
		{"not-a-version", false, ""},
	}

	for _, tc := range cases {
		out, ok := normalizeVersion(tc.in)
		if ok != tc.wantOK {
			t.Errorf("normalizeVersion(%q): ok = %v, want %v", tc.in, ok, tc.wantOK)
		}
		if ok && out != tc.wantOut {
			t.Errorf("normalizeVersion(%q) = %q, want %q", tc.in, out, tc.wantOut)
		}
	}
}

func newTestServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Error("expected a User-Agent header to be set")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func newTestDataStore(t *testing.T) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "updatecheck-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	return datastore.NewDataStore(tempDir)
}

func TestCheckNow_NewerVersionAvailable(t *testing.T) {
	server := newTestServer(t, http.StatusOK, `{"tag_name":"v1.1.0","prerelease":false,"html_url":"https://example.invalid/v1.1.0"}`)
	defer server.Close()

	ds := newTestDataStore(t)
	c := NewChecker(ds, "owner/repo", "v1.0.0")
	c.SetBaseURL(server.URL)

	result, err := c.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow failed: %v", err)
	}

	if !result.Available {
		t.Error("Expected Available=true for a newer release")
	}
	if result.LatestVersion != "v1.1.0" {
		t.Errorf("Expected LatestVersion v1.1.0, got %q", result.LatestVersion)
	}
	if result.ReleaseURL != "https://example.invalid/v1.1.0" {
		t.Errorf("Expected ReleaseURL to be set, got %q", result.ReleaseURL)
	}

	if got := c.LastResult(); got != result {
		t.Errorf("LastResult() = %+v, want %+v", got, result)
	}

	persisted, err := ds.GetUpdateCheckState()
	if err != nil {
		t.Fatalf("GetUpdateCheckState failed: %v", err)
	}
	if persisted.LastSeenVersion != "v1.1.0" {
		t.Errorf("Expected persisted LastSeenVersion v1.1.0, got %q", persisted.LastSeenVersion)
	}
}

func TestCheckNow_CurrentVersionIsUpToDate(t *testing.T) {
	server := newTestServer(t, http.StatusOK, `{"tag_name":"v1.0.0","prerelease":false,"html_url":"https://example.invalid/v1.0.0"}`)
	defer server.Close()

	c := NewChecker(newTestDataStore(t), "owner/repo", "v1.0.0")
	c.SetBaseURL(server.URL)

	result, err := c.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow failed: %v", err)
	}

	if result.Available {
		t.Error("Expected Available=false when already on the latest version")
	}
}

func TestCheckNow_OlderReleaseThanCurrent(t *testing.T) {
	// e.g. a beta/main build ahead of the last tagged release.
	server := newTestServer(t, http.StatusOK, `{"tag_name":"v0.9.0","prerelease":false}`)
	defer server.Close()

	c := NewChecker(newTestDataStore(t), "owner/repo", "v1.0.0")
	c.SetBaseURL(server.URL)

	result, err := c.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow failed: %v", err)
	}

	if result.Available {
		t.Error("Expected Available=false when the release is older than current")
	}
}

func TestCheckNow_PrereleaseExcluded(t *testing.T) {
	server := newTestServer(t, http.StatusOK, `{"tag_name":"v2.0.0","prerelease":true,"html_url":"https://example.invalid/v2.0.0"}`)
	defer server.Close()

	c := NewChecker(newTestDataStore(t), "owner/repo", "v1.0.0")
	c.SetBaseURL(server.URL)

	result, err := c.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow failed: %v", err)
	}

	if result.Available {
		t.Error("Expected Available=false for a prerelease, even though it's semver-newer")
	}
	if result.LatestVersion != "" {
		t.Errorf("Expected no LatestVersion recorded for a prerelease-only response, got %q", result.LatestVersion)
	}
}

func TestCheckNow_DirtyCurrentVersionSkipsWithoutError(t *testing.T) {
	// Server would answer, but must never be called for a non-release build.
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","prerelease":false}`))
	}))
	defer server.Close()

	c := NewChecker(newTestDataStore(t), "owner/repo", "v1.0.0-0.20260101000000-abcdef+dirty")
	c.SetBaseURL(server.URL)

	result, err := c.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("Expected no error for a dirty current version, got: %v", err)
	}
	if result.Available {
		t.Error("Expected Available=false, dirty builds must skip the comparison")
	}
	if called {
		t.Error("Expected no HTTP call for a dirty current version")
	}
}

func TestCheckNow_TimeoutReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v1.1.0","prerelease":false}`))
	}))
	defer server.Close()

	c := NewChecker(newTestDataStore(t), "owner/repo", "v1.0.0")
	c.SetBaseURL(server.URL)
	c.SetTimeout(20 * time.Millisecond)

	if _, err := c.CheckNow(context.Background()); err == nil {
		t.Error("Expected a timeout error")
	}
}

func TestCheckNow_MalformedJSONReturnsError(t *testing.T) {
	server := newTestServer(t, http.StatusOK, `not json`)
	defer server.Close()

	c := NewChecker(newTestDataStore(t), "owner/repo", "v1.0.0")
	c.SetBaseURL(server.URL)

	if _, err := c.CheckNow(context.Background()); err == nil {
		t.Error("Expected an error for a malformed JSON response")
	}
}

func TestCheckNow_NonOKStatusReturnsError(t *testing.T) {
	server := newTestServer(t, http.StatusInternalServerError, `oops`)
	defer server.Close()

	c := NewChecker(newTestDataStore(t), "owner/repo", "v1.0.0")
	c.SetBaseURL(server.URL)

	if _, err := c.CheckNow(context.Background()); err == nil {
		t.Error("Expected an error for a non-200 response")
	}
}

func TestCheckNow_FailureDoesNotOverwritePersistedState(t *testing.T) {
	ds := newTestDataStore(t)
	if err := ds.SaveUpdateCheckState(datastore.UpdateCheckState{
		LastCheckedAt:   "2026-08-01T00:00:00Z",
		LastSeenVersion: "v1.1.0",
	}); err != nil {
		t.Fatalf("Failed to seed state: %v", err)
	}

	server := newTestServer(t, http.StatusInternalServerError, `oops`)
	defer server.Close()

	c := NewChecker(ds, "owner/repo", "v1.0.0")
	c.SetBaseURL(server.URL)

	if _, err := c.CheckNow(context.Background()); err == nil {
		t.Fatal("Expected an error from the failing server")
	}

	persisted, err := ds.GetUpdateCheckState()
	if err != nil {
		t.Fatalf("GetUpdateCheckState failed: %v", err)
	}
	if persisted.LastSeenVersion != "v1.1.0" {
		t.Errorf("Expected previously-persisted state to survive a failed check, got %+v", persisted)
	}
}

// TestNewChecker_SeedsFromPersistedState verifies a restarted process picks
// up "already knew about vX.Y.Z" from a previous run without needing to
// call CheckNow first.
func TestNewChecker_SeedsFromPersistedState(t *testing.T) {
	ds := newTestDataStore(t)
	checkedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	if err := ds.SaveUpdateCheckState(datastore.UpdateCheckState{
		LastCheckedAt:   checkedAt.Format(time.RFC3339),
		LastSeenVersion: "v1.1.0",
	}); err != nil {
		t.Fatalf("Failed to seed state: %v", err)
	}

	c := NewChecker(ds, "owner/repo", "v1.0.0")
	result := c.LastResult()

	if !result.Available {
		t.Error("Expected a seeded Checker to report Available=true")
	}
	if result.LatestVersion != "v1.1.0" {
		t.Errorf("Expected seeded LatestVersion v1.1.0, got %q", result.LatestVersion)
	}
	if !result.CheckedAt.Equal(checkedAt) {
		t.Errorf("Expected seeded CheckedAt %v, got %v", checkedAt, result.CheckedAt)
	}
}

func TestNewChecker_NilDataStoreIsSafe(t *testing.T) {
	c := NewChecker(nil, "owner/repo", "v1.0.0")

	result := c.LastResult()
	if result.Available {
		t.Error("Expected a fresh Checker with no datastore to report Available=false")
	}
}

// Sanity check that the JSON tags on Result round-trip as expected — a
// contract worth pinning if this ever gets exposed via an HTTP handler.
func TestResult_JSONShape(t *testing.T) {
	r := Result{Available: true, CurrentVersion: "v1.0.0", LatestVersion: "v1.1.0", ReleaseURL: "https://example.invalid"}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	for _, key := range []string{"available", "current_version", "latest_version", "release_url", "checked_at"} {
		if _, ok := got[key]; !ok {
			t.Errorf("Expected JSON key %q in marshaled Result", key)
		}
	}
}
