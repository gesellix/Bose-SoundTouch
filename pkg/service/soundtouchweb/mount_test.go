package soundtouchweb

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestMountControlAPIShape verifies the issue #451 web API restructure:
// building the router must not panic (catches any chi route-registration
// ambiguity), and every web `/api/*` route must live under `/api/control/*`
// (the post-merge canonical namespace). This is the only test that exercises
// Mount itself; the handler tests call handlers directly with injected params.
func TestMountControlAPIShape(t *testing.T) {
	app := NewWebApp()

	r := chi.NewRouter()
	app.Mount(r, nil) // must not panic while registering routes

	var apiRoutes []string

	registered := map[string]bool{}

	walkErr := chi.Walk(r, func(_, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		registered[route] = true

		if strings.HasPrefix(route, "/api/") {
			apiRoutes = append(apiRoutes, route)
		}

		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk routes: %v", walkErr)
	}

	if len(apiRoutes) == 0 {
		t.Fatal("no /api/* routes registered")
	}

	// Invariant: the whole web API is under /api/control/* (no flat /api/devices,
	// /api/zone, /api/control/{id}/{action}, ... left behind).
	for _, route := range apiRoutes {
		if !strings.HasPrefix(route, "/api/control/") {
			t.Errorf("web API route %q is not under /api/control/ after the #451 restructure", route)
		}
	}

	// The provider infix (#451): browsable providers expose global browse
	// routes; every provider play nests under devices/{id}/providers/. The
	// app-wide socket moved from top-level /ws to /api/control/ws.
	mustExist := []string{
		"/api/control/version",
		"/api/control/ws",
		"/api/control/providers/tunein/search",
		"/api/control/providers/radiobrowser/search",
		"/api/control/devices/{id}/providers/tunein/play",
		"/api/control/devices/{id}/providers/radiobrowser/play",
		"/api/control/devices/{id}/providers/url/play",
		"/api/control/devices/{id}/providers/tts/play",
	}
	for _, want := range mustExist {
		if !registered[want] {
			t.Errorf("expected route %q to be registered; got %v", want, apiRoutes)
		}
	}

	// The pre-infix flat paths are gone, and the app-wide socket no longer
	// sits at top-level /ws.
	mustNotExist := []string{
		"/ws",
		"/api/control/tunein/search",
		"/api/control/radiobrowser/search",
		"/api/control/devices/{id}/play-url",
		"/api/control/devices/{id}/speak",
		"/api/control/devices/{id}/tunein/play",
		"/api/control/devices/{id}/radiobrowser/play",
	}
	for _, gone := range mustNotExist {
		if registered[gone] {
			t.Errorf("pre-infix route %q should have moved under /providers/", gone)
		}
	}
}

// TestMountSPARoutes verifies the issue #451 SPA move: the web UI is served
// under /app/* and the old top-level page paths are gone, with / kept only as
// a redirect into the app.
func TestMountSPARoutes(t *testing.T) {
	app := NewWebApp()

	r := chi.NewRouter()
	app.Mount(r, nil)

	routes := map[string]bool{}

	walkErr := chi.Walk(r, func(_, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes[route] = true

		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk routes: %v", walkErr)
	}

	for _, want := range []string{"/app", "/app/devices", "/app/tunein"} {
		if !routes[want] {
			t.Errorf("expected SPA route %q under /app to be registered", want)
		}
	}

	// The old top-level page paths moved under /app.
	for _, gone := range []string{"/devices", "/device/*", "/tunein", "/radiobrowser", "/playurl", "/tts"} {
		if routes[gone] {
			t.Errorf("top-level SPA route %q should have moved under /app", gone)
		}
	}

	// / stays registered, but only as the redirect into the app.
	if !routes["/"] {
		t.Error("expected / to remain registered (redirect into the app)")
	}
}
