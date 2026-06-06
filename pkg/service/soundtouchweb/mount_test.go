package soundtouchweb

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// walkRoutes returns the set of route patterns registered on r.
func walkRoutes(t *testing.T, r chi.Router) map[string]bool {
	t.Helper()

	routes := map[string]bool{}

	err := chi.Walk(r, func(_, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes[route] = true

		return nil
	})
	if err != nil {
		t.Fatalf("walk routes: %v", err)
	}

	return routes
}

// TestMountWebControlAPIShape verifies the issue #451 web API restructure on
// the portable surface (MountWeb): building must not panic (catches any chi
// route-registration ambiguity), and every web /api/* route must live under
// /api/control/* (the post-merge canonical namespace).
func TestMountWebControlAPIShape(t *testing.T) {
	app := NewWebApp()

	r := chi.NewRouter()
	app.MountWeb(r, nil) // must not panic while registering routes

	registered := walkRoutes(t, r)

	var apiRoutes []string

	for route := range registered {
		if strings.HasPrefix(route, "/api/") {
			apiRoutes = append(apiRoutes, route)
		}
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
	// app-wide socket moved from top-level /ws to /api/control/ws, and assets
	// live under /app/static.
	mustExist := []string{
		"/app/static/*",
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
			t.Errorf("expected route %q to be registered", want)
		}
	}

	// The pre-infix flat paths are gone, the app-wide socket no longer sits at
	// top-level /ws, and the portable surface owns nothing outside
	// /api/control + /app (no /, no /health, no top-level /static).
	mustNotExist := []string{
		"/",
		"/ws",
		"/health",
		"/static/*",
		"/api/control/tunein/search",
		"/api/control/radiobrowser/search",
		"/api/control/devices/{id}/play-url",
		"/api/control/devices/{id}/speak",
		"/api/control/devices/{id}/tunein/play",
		"/api/control/devices/{id}/radiobrowser/play",
	}
	for _, gone := range mustNotExist {
		if registered[gone] {
			t.Errorf("portable surface should not register %q", gone)
		}
	}
}

// TestMountWebSPARoutes verifies the issue #451 SPA move: the web UI is served
// under /app/* and the old top-level page paths are gone. The portable surface
// does not register /.
func TestMountWebSPARoutes(t *testing.T) {
	app := NewWebApp()

	r := chi.NewRouter()
	app.MountWeb(r, nil)

	routes := walkRoutes(t, r)

	for _, want := range []string{"/app", "/app/devices", "/app/tunein"} {
		if !routes[want] {
			t.Errorf("expected SPA route %q under /app to be registered", want)
		}
	}

	for _, gone := range []string{"/", "/devices", "/device/*", "/tunein", "/radiobrowser", "/playurl", "/tts"} {
		if routes[gone] {
			t.Errorf("top-level route %q should not be registered by the portable surface", gone)
		}
	}
}

// TestMountStandalone verifies that the standalone entry point (Mount) adds the
// / redirect and /health liveness endpoint on top of the portable surface.
func TestMountStandalone(t *testing.T) {
	app := NewWebApp()

	r := chi.NewRouter()
	app.Mount(r, nil)

	routes := walkRoutes(t, r)

	for _, want := range []string{"/", "/health", "/app", "/api/control/version"} {
		if !routes[want] {
			t.Errorf("expected standalone Mount to register %q", want)
		}
	}
}
