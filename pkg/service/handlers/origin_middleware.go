package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// quietLogPaths are polled by the admin Logs tab every 1.5 s while it is open.
// Logging a successful poll would make the page show an endless stream of its
// own requests (issue 728), so only failures are logged for these.
var quietLogPaths = map[string]bool{
	"/api/setup/logs": true,
	"/setup/logs":     true,
}

// OriginMiddleware returns a middleware that logs whether the request was handled "self" or "upstream".
func (s *Server) OriginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		if r.Method == http.MethodGet && quietLogPaths[r.URL.Path] && ww.Status() < http.StatusBadRequest {
			return
		}

		origin := "self"
		if ww.Header().Get("X-Proxy-Origin") != "" {
			origin = "upstream"
		}

		log.Printf("[LOG] %s %s | %d | %s | %v", r.Method, sanitizeLog(r.URL.Path), ww.Status(), origin, time.Since(start))
	})
}
