package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// OriginMiddleware returns a middleware that logs whether the request was handled "self" or "upstream".
func (s *Server) OriginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		origin := "self"
		if ww.Header().Get("X-Proxy-Origin") != "" {
			origin = "upstream"
		}

		log.Printf("[LOG] %s %s | %d | %s | %v", r.Method, r.URL.Path, ww.Status(), origin, time.Since(start))
	})
}
