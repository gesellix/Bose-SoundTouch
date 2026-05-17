// Package handlers — BMX registry / availability and shared helpers.
//
// Per-service handlers live in handlers_bmx_<service>.go:
//   - handlers_bmx_tunein.go    (TuneIn — playback / podcasts / navigate / search / favorites / report)
//   - handlers_bmx_orion.go     (Orion — LOCAL_INTERNET_RADIO token + station)
//   - handlers_bmx_custom.go    (our own custom-playback adapter)
//
// The split happened on 2026-05-17 as a pure refactor — no logic change.
// A future iteration may extract a common BMX-service interface (see
// memory project_bmx_service_interface.md) once enough services are
// fully implemented to make the common shape observable.
package handlers

import (
	"net/http"
	"strings"
)

// HandleBMXRegistry returns the BMX service registry.
func (s *Server) HandleBMXRegistry(w http.ResponseWriter, _ *http.Request) {
	baseURL := s.serverURL

	s.mu.RLock()
	dnsEnabled := s.dnsEnabled
	s.mu.RUnlock()

	bmxServer := baseURL
	if dnsEnabled {
		bmxServer = "https://content.api.bose.io"
	}

	content := string(bmxServicesJSON)
	content = strings.ReplaceAll(content, "{BMX_SERVER}", bmxServer)
	content = strings.ReplaceAll(content, "{MEDIA_SERVER}", baseURL+"/media")

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(content))
}

// HandleBMXServicesAvailability returns the BMX services availability.
func (s *Server) HandleBMXServicesAvailability(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(bmxServicesAvailabilityJSON)
}

// writeBMXUnauthorized writes the canonical 401 used by every BMX adapter
// handler that requires an Authorization header (TuneIn variants, Orion
// playback). Currently unused because all gate sites are temporarily
// disabled (log-only); kept as the future-restore point — when we re-add
// the gate, callers will use this helper.
//
//nolint:unused // intentional: future-restore point for the disabled auth gate.
func (s *Server) writeBMXUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`<!doctype html>
<html lang=en>
<title>401 Unauthorized</title>
<h1>Unauthorized</h1>
<p>Authorization not set. No access token found.</p>
`))
}
