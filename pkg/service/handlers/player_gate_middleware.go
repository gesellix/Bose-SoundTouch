package handlers

import (
	"net/http"
)

// playerDisabledMessage is returned verbatim (plain text) whenever the
// embedded player is switched off in Settings and a request hits its routes
// (/app, /api/control/*) anyway. Kept as a named constant so tests can assert
// on it without duplicating the literal.
const playerDisabledMessage = "the player is disabled in AfterTouch settings"

// PlayerGateMiddleware refuses requests to the embedded player's routes with
// a clear message when the player is disabled, instead of leaving them to
// fall through to whatever the router would otherwise do. Reads
// s.PlayerEnabled() live on every request, so a Settings-page toggle applies
// to the very next request -- no restart needed (issue 762).
func (s *Server) PlayerGateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.PlayerEnabled() {
			http.Error(w, playerDisabledMessage, http.StatusNotFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}
