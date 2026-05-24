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
	"encoding/json"
	"fmt"
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

// extractBMXService finds a single service entry in bmx_services.json by
// its `id.name` (e.g. "SIRIUSXM_EVEREST", "TUNEIN"). Returns the raw JSON
// segment for that service so callers can apply {BMX_SERVER} / {MEDIA_SERVER}
// substitution and write it back to the wire.
func extractBMXService(bmxJSON []byte, name string) (json.RawMessage, error) {
	var wrapper struct {
		BMXServices []json.RawMessage `json:"bmx_services"`
	}

	if err := json.Unmarshal(bmxJSON, &wrapper); err != nil {
		return nil, fmt.Errorf("parse bmx_services.json: %w", err)
	}

	for _, raw := range wrapper.BMXServices {
		var idOnly struct {
			ID struct {
				Name string `json:"name"`
			} `json:"id"`
		}

		if err := json.Unmarshal(raw, &idOnly); err != nil {
			continue
		}

		if idOnly.ID.Name == name {
			return raw, nil
		}
	}

	return nil, fmt.Errorf("service %q not found in bmx_services.json", name)
}

// applyBMXTemplate runs the same {BMX_SERVER} / {MEDIA_SERVER} substitution
// HandleBMXRegistry uses, so service-descriptor responses produced from
// sub-segments of bmx_services.json land at the same hostnames the
// registry advertises.
func (s *Server) applyBMXTemplate(content string) string {
	baseURL := s.serverURL

	s.mu.RLock()
	dnsEnabled := s.dnsEnabled
	s.mu.RUnlock()

	bmxServer := baseURL
	if dnsEnabled {
		bmxServer = "https://content.api.bose.io"
	}

	content = strings.ReplaceAll(content, "{BMX_SERVER}", bmxServer)
	content = strings.ReplaceAll(content, "{MEDIA_SERVER}", baseURL+"/media")

	return content
}

// writeBMXUnauthorized writes the canonical 401 used by every BMX adapter
// handler that requires an Authorization header (TuneIn variants, Orion
// playback). Currently unused because all gate sites are temporarily
// disabled (log-only); kept as the future-restore point — when we re-add
// the gate, callers will use this helper.
//
//lint:ignore U1000 intentional: future-restore point for the disabled BMX auth gate
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
