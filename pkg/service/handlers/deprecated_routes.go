package handlers

import (
	"log"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

// deprecatedRouteTracker counts hits on legacy admin routes (the /setup and
// /mgmt paths that now also live under /api/*). The counts let the eventual 1.x
// removal be data-driven — a route is only cut once it has gone quiet across
// real deployments — and are surfaced in the diagnostic export.
type deprecatedRouteTracker struct {
	mu     sync.Mutex
	counts map[string]int64
	logged map[string]bool
}

func newDeprecatedRouteTracker() *deprecatedRouteTracker {
	return &deprecatedRouteTracker{
		counts: map[string]int64{},
		logged: map[string]bool{},
	}
}

// record increments the counter for key ("METHOD pattern") and reports whether
// this is the first time the key was seen, so the caller can log once per route
// per process rather than on every hit.
func (t *deprecatedRouteTracker) record(key string) (firstHit bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.counts[key]++

	if !t.logged[key] {
		t.logged[key] = true

		return true
	}

	return false
}

func (t *deprecatedRouteTracker) snapshot() map[string]int64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make(map[string]int64, len(t.counts))
	for k, v := range t.counts {
		out[k] = v
	}

	return out
}

// DeprecatedRouteMiddleware records a hit on a legacy admin route and logs a
// one-time warning pointing at the /api equivalent. It never alters the
// response — the legacy paths keep working; this only produces the observability
// the 1.x removal needs. Wire it onto the legacy /setup and /mgmt mounts only,
// not their /api/* twins.
func (s *Server) DeprecatedRouteMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		rctx := chi.RouteContext(r.Context())
		if rctx == nil {
			return
		}

		pattern := rctx.RoutePattern()
		if pattern == "" {
			return
		}

		key := r.Method + " " + pattern
		if s.deprecatedRoutes.record(key) {
			log.Printf("[deprecated-route] %s used by client=%s — use /api%s instead; "+
				"the legacy path still works but is slated for removal in a future major release",
				sanitizeLog(key), sanitizeLog(clientHost(r)), sanitizeLog(pattern))
		}
	})
}

// DeprecatedRouteHits returns a snapshot of legacy-route hit counts keyed by
// "METHOD pattern", for the diagnostic export.
func (s *Server) DeprecatedRouteHits() map[string]int64 {
	return s.deprecatedRoutes.snapshot()
}
