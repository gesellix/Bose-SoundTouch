package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// DEPRECATED: handlers_telnet_probe.go (HandleTelnetProbe,
// HandleProbeInbound, telnetProbeTimeout, telnetProbeResponse) is
// scheduled for removal. The swUpdate daemon caches its target URL
// at boot; the active flip in RunTelnetRoundTripProbe never reaches
// the running daemon. See docs/analysis/TELNET-MIGRATION-METHOD.md
// §9.8. Replaced by HandlePeerProbe (handlers_peer_probe.go).

// telnetProbeTimeout caps how long the orchestrator waits for the
// device's outbound swUpdateCheck fan-out to land on /probe/{token}.
// 6s lines up with the existing telnet preflight budgets and is well
// above the median observed round-trip (<1s on FW 27.0.6).
const telnetProbeTimeout = 6 * time.Second

// DEPRECATED: see the file-level note above.
//
// HandleProbeInbound is the catch-all for /probe/{token}/* — the path
// the round-trip orchestrator sets as the speaker's swUpdateUrl. Any
// hit signals the registered channel; the response body is a minimal
// XML stub so the speaker's swUpdateCheck doesn't error out on a
// missing structure.
func (s *Server) HandleProbeInbound(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token != "" {
		s.probes.Signal(token)
	}

	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><swUpdateIndex/>`))
}

// telnetProbeResponse is the body of POST /setup/telnet-probe/{deviceId}.
type telnetProbeResponse struct {
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// DEPRECATED: see the file-level note above.
//
// HandleTelnetProbe runs the SSH-less round-trip reachability check.
// Generates a token, temporarily points the speaker's swUpdateUrl at
// /probe/{token} via telnet, triggers :8090/swUpdateCheck, and reports
// whether the device's outbound landed on our service within
// telnetProbeTimeout.
//
// Query params:
//   - target_url (optional) — defaults to the configured server URL.
func (s *Server) HandleTelnetProbe(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		writeJSONError(w, http.StatusBadRequest, "Device ID is required")
		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	targetURL := r.URL.Query().Get("target_url")
	if targetURL == "" {
		targetURL = s.sm.ServerURL
	}

	result, err := s.sm.RunTelnetRoundTripProbe(deviceIP, targetURL, s.probes, telnetProbeTimeout)

	w.Header().Set("Content-Type", "application/json")

	body := telnetProbeResponse{
		OK:     err == nil && result != nil && result.Reached,
		Result: result,
	}
	if err != nil {
		body.Error = err.Error()
	}

	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
