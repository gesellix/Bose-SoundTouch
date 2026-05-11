package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// peerProbeTimeout caps how long the passive observer waits for any
// inbound from the device IP after the :8090/swUpdateCheck nudge. 30s
// is comfortable for daemon wake-up latency on slow devices while still
// keeping the panel responsive; result.ElapsedMs surfaces the actual
// observed latency so the budget can be tuned from real data.
const peerProbeTimeout = 30 * time.Second

// peerProbeResponse is the body of POST /setup/peer-probe/{deviceId}.
type peerProbeResponse struct {
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// HandlePeerProbe runs the post-migration passive reachability check.
// Registers interest in the device's IP, nudges :8090/swUpdateCheck,
// and reports whether any inbound from that IP landed within
// peerProbeTimeout. Any inbound counts — on a migrated speaker, DNS
// interception routes the daemon's outbounds (update fan-out, marge,
// BMX) through this service regardless of which URL the daemon
// resolved internally, so the question reduces to "did the device
// dial us at all."
//
// Unlike the deprecated round-trip probe, this handler does not mutate
// device state. It presupposes the speaker is already migrated; the
// pre-flight orchestrator is responsible for only calling it in that
// state.
func (s *Server) HandlePeerProbe(w http.ResponseWriter, r *http.Request) {
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

	result, err := s.sm.RunPeerReachabilityProbe(deviceIP, s.peerObserver, peerProbeTimeout)

	w.Header().Set("Content-Type", "application/json")

	body := peerProbeResponse{
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
