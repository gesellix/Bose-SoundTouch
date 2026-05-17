// Package handlers — AfterTouch's own custom-playback adapter (not a
// Bose-official BMX service). Reached via /custom/v1/playback/{encodedURL}
// by speakers that follow our LOCAL_INTERNET_RADIO preset locations.
//
// Split out of handlers_bmx.go on 2026-05-17; pure file move, no logic
// change.
package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/gesellix/bose-soundtouch/pkg/service/bmx"
	"github.com/go-chi/chi/v5"
)

// HandleCustomPlayback returns custom playback information for a given stream URL.
func (s *Server) HandleCustomPlayback(w http.ResponseWriter, r *http.Request) {
	encodedURL := chi.URLParam(r, "encodedURL")
	imageUrl := r.URL.Query().Get("imageUrl")
	name := r.URL.Query().Get("name")

	// Decode URL if it's base64 encoded
	var streamUrl string

	decoded, err := base64.URLEncoding.DecodeString(encodedURL)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(encodedURL)
	}

	if err == nil {
		streamUrl = string(decoded)
	} else {
		// Try unescaping if it's not base64
		streamUrl, err = url.PathUnescape(encodedURL)
		if err != nil {
			streamUrl = encodedURL
		}
	}

	resp, err := bmx.BuildCustomStreamResponse(streamUrl, imageUrl, name)
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
