package handlers

import (
	"net/http"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

// PeerObserverMiddleware records every incoming request's source IP and
// path in the peerObserver registry. It fires on every request before
// the handler runs, so passive reachability probes can register a device
// IP and learn whether any inbound landed in their wait window.
//
// Placement: after ClientIPMiddleware (so the resolved client IP is available
// via middleware.GetClientIP) and after Recoverer (so any panic inside this
// middleware is contained). Before any short-circuiting middleware
// would be unnecessary — Signal runs before next.ServeHTTP, so the
// observation lands regardless of how later middleware handles the
// request.
func (s *Server) PeerObserverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if host := clientHost(r); host != "" {
			s.peerObserver.Signal(host, setup.PeerHit{Path: r.URL.Path, At: time.Now()})
		}

		next.ServeHTTP(w, r)
	})
}
