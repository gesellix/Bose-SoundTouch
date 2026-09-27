package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb"
)

// TestPlayerGateRouter is the wiring-level regression test for issue 762: it
// exercises the real production router (setupRouter) with a real webApp
// mounted, not just PlayerGateMiddleware in isolation, to pin that /app and
// /api/control/* are served when the player is enabled (the default) and
// refused with the disabled message -- not a bare 404 -- once it is turned
// off, live, with no restart.
func TestPlayerGateRouter(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	server := handlers.NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)
	webApp := soundtouchweb.NewWebApp()

	r := setupRouter(server, nil, webApp)
	ts := httptest.NewServer(r)

	defer ts.Close()

	playerPaths := []string{"/app", "/api/control/devices"}

	t.Run("enabled (default): player routes are served", func(t *testing.T) {
		for _, path := range playerPaths {
			res, err := http.Get(ts.URL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			_ = res.Body.Close()

			if res.StatusCode == http.StatusNotFound {
				t.Errorf("GET %s: got 404 while enabled, want the route served", path)
			}
		}
	})

	t.Run("disabled: player routes refuse with the disabled message", func(t *testing.T) {
		server.SetPlayerEnabled(false)
		defer server.SetPlayerEnabled(true)

		for _, path := range playerPaths {
			res, err := http.Get(ts.URL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}

			body, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()

			if res.StatusCode != http.StatusNotFound {
				t.Errorf("GET %s: status = %d, want 404", path, res.StatusCode)
			}

			if !strings.Contains(string(body), "the player is disabled in AfterTouch settings") {
				t.Errorf("GET %s: body = %q, want the disabled message", path, body)
			}
		}
	})

	t.Run("re-enabled live: player routes are served again, no restart", func(t *testing.T) {
		server.SetPlayerEnabled(false)
		server.SetPlayerEnabled(true)

		res, err := http.Get(ts.URL + "/app")
		if err != nil {
			t.Fatalf("GET /app: %v", err)
		}
		_ = res.Body.Close()

		if res.StatusCode == http.StatusNotFound {
			t.Errorf("GET /app: got 404 after re-enabling, want the route served")
		}
	})
}
