package handlers

import (
	"net/http"
)

// ShortcutMiddleware returns a middleware that shortcuts requests to specific paths.
func (s *Server) ShortcutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		shortcuts := s.shortcuts
		s.mu.RUnlock()

		if shortcuts != nil {
			if status, ok := shortcuts[r.URL.Path]; ok {
				w.WriteHeader(status)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
