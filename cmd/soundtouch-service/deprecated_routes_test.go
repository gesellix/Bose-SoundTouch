package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
)

// TestDeprecatedRouteSignal verifies the legacy admin paths are counted (and the
// new /api/* twins are not), so the diagnostic export can show whether the old
// paths are still in use before they are removed in a future major release.
func TestDeprecatedRouteSignal(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	server := handlers.NewServer(ds, nil, "http://localhost:8000", true, false, false)
	r := setupRouter(server, nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	hit := func(path string) {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}

		_ = resp.Body.Close()
	}

	hit("/setup/version")     // legacy — counted
	hit("/setup/version")     // legacy again — count increments
	hit("/api/setup/version") // new canonical — must NOT be counted

	hits := server.DeprecatedRouteHits()

	if got := hits["GET /setup/version"]; got != 2 {
		t.Errorf("legacy GET /setup/version hits = %d, want 2", got)
	}

	if _, tracked := hits["GET /api/setup/version"]; tracked {
		t.Errorf("/api/setup/version must not be tracked as deprecated; hits=%v", hits)
	}
}
