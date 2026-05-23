// Package handlers — TuneIn BMX adapter handlers.
//
// Split out of handlers_bmx.go on 2026-05-17; pure file move, no logic
// change. Shared helpers (writeBMXUnauthorized, bmxServicesJSON) still
// live in handlers_bmx.go.
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/service/bmx"
	"github.com/go-chi/chi/v5"
)

// tuneInStreamFormats returns the formats= list AfterTouch should send
// to TuneIn's Tune.ashx, honouring Settings.TuneInStreamFormats when
// set. Empty (the default) lets bmx.TuneInStream fall back to
// bmx.DefaultTuneInStreamFormats — the SoundTouch-line-compatible
// "mp3,aac,ogg" shape. Operators with HLS-capable speakers can set
// the field to "mp3,aac,ogg,hls" (or any other comma-separated list)
// in settings.json.
func (s *Server) tuneInStreamFormats() string {
	if s == nil || s.ds == nil {
		return ""
	}

	settings, err := s.ds.GetSettings()
	if err != nil {
		return ""
	}

	return settings.TuneInStreamFormats
}

// HandleTuneInPlayback returns TuneIn playback information.
func (s *Server) HandleTuneInPlayback(w http.ResponseWriter, r *http.Request) {
	// Authorization gate temporarily disabled (was: 401 if header missing).
	// The Stockholm browser proxy doesn't inject Authorization for requests
	// that target our own service. Logged so we can spot callers that would
	// have been rejected; do NOT 401.
	if r.Header.Get("Authorization") == "" {
		log.Printf("[BMX] Authorization missing (gate temporarily disabled, see handlers_bmx.go); path=%q ua=%q",
			r.URL.Path, r.UserAgent())
	}

	stationID := chi.URLParam(r, "stationID")

	resp, err := bmx.TuneInPlayback(stationID, s.tuneInStreamFormats())
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

// HandleTuneInPodcastInfo returns TuneIn podcast information.
func (s *Server) HandleTuneInPodcastInfo(w http.ResponseWriter, r *http.Request) {
	// Authorization gate temporarily disabled (was: 401 if header missing).
	// The Stockholm browser proxy doesn't inject Authorization for requests
	// that target our own service. Logged so we can spot callers that would
	// have been rejected; do NOT 401.
	if r.Header.Get("Authorization") == "" {
		log.Printf("[BMX] Authorization missing (gate temporarily disabled, see handlers_bmx.go); path=%q ua=%q",
			r.URL.Path, r.UserAgent())
	}

	podcastID := chi.URLParam(r, "podcastID")
	encodedName := r.URL.Query().Get("encoded_name")

	resp, err := bmx.TuneInPodcastInfo(podcastID, encodedName)
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

// HandleTuneInPlaybackPodcast returns TuneIn podcast playback information.
func (s *Server) HandleTuneInPlaybackPodcast(w http.ResponseWriter, r *http.Request) {
	// Authorization gate temporarily disabled (was: 401 if header missing).
	// The Stockholm browser proxy doesn't inject Authorization for requests
	// that target our own service. Logged so we can spot callers that would
	// have been rejected; do NOT 401.
	if r.Header.Get("Authorization") == "" {
		log.Printf("[BMX] Authorization missing (gate temporarily disabled, see handlers_bmx.go); path=%q ua=%q",
			r.URL.Path, r.UserAgent())
	}

	podcastID := chi.URLParam(r, "podcastID")

	resp, err := bmx.TuneInPlaybackPodcast(podcastID, s.tuneInStreamFormats())
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

// HandleTuneInToken returns a TuneIn access token.
func (s *Server) HandleTuneInToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GrantType    string `json:"grant_type"`
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// For now, we return the provided refresh_token as access_token and refresh_token,
	// mirroring the behavior seen in the recordings.
	resp := map[string]string{
		"access_token":  req.RefreshToken,
		"refresh_token": req.RefreshToken,
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleTuneInReport handles TuneIn playback reporting.
func (s *Server) HandleTuneInReport(w http.ResponseWriter, r *http.Request) {
	// Authorization gate temporarily disabled (was: 401 if header missing).
	// The Stockholm browser proxy doesn't inject Authorization for requests
	// that target our own service. Logged so we can spot callers that would
	// have been rejected; do NOT 401.
	if r.Header.Get("Authorization") == "" {
		log.Printf("[BMX] Authorization missing (gate temporarily disabled, see handlers_bmx.go); path=%q ua=%q",
			r.URL.Path, r.UserAgent())
	}

	var req struct {
		EventType string `json:"eventType"`
	}

	// We don't strictly need the body to determine the response,
	// but we decode it to see the eventType.
	_ = json.NewDecoder(r.Body).Decode(&req)

	w.Header().Set("Content-Type", "application/json")

	if req.EventType == "START" {
		// Mirroring the response from 0196-20260329-233306.072-POST.http
		resp := map[string]interface{}{
			"_links": map[string]interface{}{
				"self": map[string]interface{}{
					"href": "/v1/report?" + r.URL.RawQuery,
				},
			},
			"nextReportIn": 1800,
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	// For STOP and other events, return an empty object
	_, _ = w.Write([]byte("{}"))
}

// HandleTuneInNavigate returns live TuneIn navigation results.
// Path variants handled via chi wildcard:
//   - (empty)                               → top-level browse
//   - {encodedURI}                          → browse the given TuneIn URI
//   - sub/{n}/{encodedURI}                  → single subsection of a browse page
//   - profiles/{type}/{id}/{encodedURI}     → artist/program profile page
func (s *Server) HandleTuneInNavigate(w http.ResponseWriter, r *http.Request) {
	// Authorization gate temporarily disabled (was: 401 if header missing).
	// The Stockholm browser proxy doesn't inject Authorization for requests
	// that target our own service. Logged so we can spot callers that would
	// have been rejected; do NOT 401.
	if r.Header.Get("Authorization") == "" {
		log.Printf("[BMX] Authorization missing (gate temporarily disabled, see handlers_bmx.go); path=%q ua=%q",
			r.URL.Path, r.UserAgent())
	}

	wildcard := chi.URLParam(r, "*")

	resp, err := parseTuneInNavigatePath(wildcard)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func parseTuneInNavigatePath(wildcard string) (interface{}, error) {
	if wildcard == "" {
		return bmx.TuneInNavigate("", nil)
	}

	firstSlash := strings.Index(wildcard, "/")
	if firstSlash == -1 {
		return bmx.TuneInNavigate(wildcard, nil)
	}

	prefix := wildcard[:firstSlash]
	rest := wildcard[firstSlash+1:]

	switch prefix {
	case "sub":
		secondSlash := strings.Index(rest, "/")
		if secondSlash == -1 {
			return bmx.TuneInNavigate(rest, nil)
		}

		n, err := strconv.Atoi(rest[:secondSlash])
		if err != nil {
			return bmx.TuneInNavigate(wildcard, nil)
		}

		return bmx.TuneInNavigate(rest[secondSlash+1:], &n)

	case "profiles":
		// profiles/{type}/{id}/{encodedURI}
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) < 3 {
			return bmx.TuneInNavigate(wildcard, nil)
		}

		return bmx.TuneInNavigateProfile(parts[2])

	default:
		return bmx.TuneInNavigate(wildcard, nil)
	}
}

// HandleTuneInSearch returns live TuneIn search results for the given query.
func (s *Server) HandleTuneInSearch(w http.ResponseWriter, r *http.Request) {
	// Authorization gate temporarily disabled (was: 401 if header missing).
	// The Stockholm browser proxy doesn't inject Authorization for requests
	// that target our own service. Logged so we can spot callers that would
	// have been rejected; do NOT 401.
	if r.Header.Get("Authorization") == "" {
		log.Printf("[BMX] Authorization missing (gate temporarily disabled, see handlers_bmx.go); path=%q ua=%q",
			r.URL.Path, r.UserAgent())
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	resp, err := bmx.TuneInSearch(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleTuneInSearchNext returns the next page of TuneIn search results using
// an opaque cursor produced by HandleTuneInSearch.
func (s *Server) HandleTuneInSearchNext(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "" {
		log.Printf("[BMX] Authorization missing (gate temporarily disabled, see handlers_bmx.go); path=%q ua=%q",
			r.URL.Path, r.UserAgent())
	}

	cursor := r.URL.Query().Get("cursor")
	if cursor == "" {
		http.Error(w, "cursor parameter required", http.StatusBadRequest)
		return
	}

	resp, err := bmx.TuneInSearchNext(cursor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleTuneInFavorite handles POST /bmx/tunein/v1/favorite/{stationID}.
func (s *Server) HandleTuneInFavorite(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationID")
	if err := s.ds.SaveTuneInFavorite(stationID); err != nil {
		log.Printf("Failed to persist TuneIn favorite %s: %v", stationID, err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("{}"))
}

// HandleTuneInDeleteFavorite handles DELETE /bmx/tunein/v1/favorite/{stationID}.
func (s *Server) HandleTuneInDeleteFavorite(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationID")
	if err := s.ds.DeleteTuneInFavorite(stationID); err != nil {
		log.Printf("Failed to delete TuneIn favorite %s: %v", stationID, err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("{}"))
}
