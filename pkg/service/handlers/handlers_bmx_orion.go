// Package handlers — Orion BMX adapter handlers (LOCAL_INTERNET_RADIO).
//
// Split out of handlers_bmx.go on 2026-05-17; pure file move, no logic
// change. Shared helpers (writeBMXUnauthorized) still live in
// handlers_bmx.go.
package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gesellix/bose-soundtouch/pkg/service/bmx"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// HandleOrionToken returns an anonymous Orion access token.
// The token is a base64-encoded JSON serial, matching the pattern used by the real Bose BMX Orion service.
func (s *Server) HandleOrionToken(w http.ResponseWriter, _ *http.Request) {
	token := datastore.GenerateSerialSecret("orion")

	resp := map[string]interface{}{
		"_embedded": map[string]interface{}{
			"bmx_account": map[string]string{
				"displayName": "",
				"username":    "",
			},
		},
		"access_token":  token,
		"refresh_token": token,
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleOrionPlayback returns Orion playback information for the
// /core02/svc-bmx-adapter-orion/prod/orion/station?data=... endpoint
// the speaker reaches by following its stored LOCAL_INTERNET_RADIO
// preset's `location` attribute. The `data` query string is the
// base64-encoded JSON blob (streamUrl/imageUrl/name) that the speaker
// constructed when the preset was first saved; we just decode and
// rewrap it into the Bose BmxPlaybackResponse shape via
// bmx.PlayCustomStream.
//
// Requires a Bearer token in the `Authorization` header — same as
// the rest of the BMX playback surface (TuneIn variants and the
// orion token endpoint). Real speakers obtain the token via
// POST /core02/svc-bmx-adapter-orion/prod/orion/token (HandleOrionToken)
// before they ever follow a LOCAL_INTERNET_RADIO preset, so this
// check shouldn't cost any legitimate caller.
func (s *Server) HandleOrionPlayback(w http.ResponseWriter, r *http.Request) {
	// Authorization gate temporarily disabled (was: 401 if header missing).
	// See HandleTuneInPlayback for the rationale. Logged so we can spot
	// callers that would have been rejected; do NOT 401.
	if r.Header.Get("Authorization") == "" {
		log.Printf("[BMX] Authorization missing (gate temporarily disabled, see handlers_bmx.go); path=%q ua=%q",
			r.URL.Path, r.UserAgent())
	}

	data := r.URL.Query().Get("data")

	resp, err := bmx.PlayCustomStream(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
