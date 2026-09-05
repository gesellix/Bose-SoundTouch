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
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
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
			sanitizeLog(r.URL.Path), sanitizeLog(r.UserAgent()))
	}

	stationID := strings.TrimSpace(chi.URLParam(r, "stationID"))

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
			sanitizeLog(r.URL.Path), sanitizeLog(r.UserAgent()))
	}

	podcastID := strings.TrimSpace(chi.URLParam(r, "podcastID"))
	encodedName := strings.TrimSpace(r.URL.Query().Get("encoded_name"))

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
			sanitizeLog(r.URL.Path), sanitizeLog(r.UserAgent()))
	}

	podcastID := strings.TrimSpace(chi.URLParam(r, "podcastID"))

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

// HandleTuneInToken returns an anonymous TuneIn access token.
//
// The registry advertises TUNEIN with authenticationModel.anonymousAccount
// (autoCreate: true) — see bmx_services.json — so the speaker's very first
// call here is a bootstrap request with no prior refresh_token to present.
// This handler used to echo back whatever refresh_token the speaker sent
// (mirroring an authenticated-refresh recording), which meant that very
// first bootstrap call round-tripped an empty token. The speaker never
// obtained a usable TuneIn account and subsequently rejected every TUNEIN
// ContentItem selection with INVALID_SOURCE, even though /sources reported
// TUNEIN as READY (READY only reflects registry presence, not a live
// account). Match HandleOrionToken's unconditional-generation shape
// instead: always mint a token, regardless of what the speaker sent.
//
// The token itself is a stable, constant value (datastore.GenerateSerialSecret
// is a pure function of the hardcoded "tunein" literal), not a fresh or
// per-device secret — it's the same value for every device and every call.
// That's fine today only because the Authorization gate is disabled for all
// TuneIn handlers (see HandleTuneInReport below) and nothing validates the
// token's uniqueness; if either of those ever changes, this would need a
// real per-device/per-session token instead.
func (s *Server) HandleTuneInToken(w http.ResponseWriter, r *http.Request) {
	// The unconditional mint above means we never use the decoded values,
	// but we still decode the body so a genuinely malformed request (not a
	// normal bootstrap call, which is valid JSON with an empty/absent
	// refresh_token) gets a 400 instead of silently succeeding.
	var req struct {
		GrantType    string `json:"grant_type"`
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	token := datastore.GenerateSerialSecret("tunein")

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

// HandleTuneInReport handles TuneIn playback reporting.
func (s *Server) HandleTuneInReport(w http.ResponseWriter, r *http.Request) {
	// Authorization gate temporarily disabled (was: 401 if header missing).
	// The Stockholm browser proxy doesn't inject Authorization for requests
	// that target our own service. Logged so we can spot callers that would
	// have been rejected; do NOT 401.
	if r.Header.Get("Authorization") == "" {
		log.Printf("[BMX] Authorization missing (gate temporarily disabled, see handlers_bmx.go); path=%q ua=%q",
			sanitizeLog(r.URL.Path), sanitizeLog(r.UserAgent()))
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
//   - profiles/{encodedURI}                 → artist/program profile page
func (s *Server) HandleTuneInNavigate(w http.ResponseWriter, r *http.Request) {
	// Authorization gate temporarily disabled (was: 401 if header missing).
	// The Stockholm browser proxy doesn't inject Authorization for requests
	// that target our own service. Logged so we can spot callers that would
	// have been rejected; do NOT 401.
	if r.Header.Get("Authorization") == "" {
		log.Printf("[BMX] Authorization missing (gate temporarily disabled, see handlers_bmx.go); path=%q ua=%q",
			sanitizeLog(r.URL.Path), sanitizeLog(r.UserAgent()))
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
		// Hrefs are generated as a single path segment:
		// /v1/navigate/profiles/{encodedURI} (see tuneInSearchProfile in
		// pkg/service/bmx/tunein.go). Requiring a profiles/{type}/{id}/{encodedURI}
		// shape here caused every profile link to fall through to
		// bmx.TuneInNavigate with the literal "profiles/..." prefix still
		// attached, which is not valid base64 and produced a 500 ("illegal
		// base64 data..."). Take the last path segment as the encoded URI so
		// both the current single-segment hrefs and any legacy
		// multi-segment ones decode correctly.
		parts := strings.Split(rest, "/")

		encodedURI := parts[len(parts)-1]
		if encodedURI == "" {
			return bmx.TuneInNavigate(wildcard, nil)
		}

		return bmx.TuneInNavigateProfile(encodedURI)

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
			sanitizeLog(r.URL.Path), sanitizeLog(r.UserAgent()))
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
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
			sanitizeLog(r.URL.Path), sanitizeLog(r.UserAgent()))
	}

	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
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
	stationID := strings.TrimSpace(chi.URLParam(r, "stationID"))
	if err := s.ds.SaveTuneInFavorite(stationID); err != nil {
		log.Printf("Failed to persist TuneIn favorite %s: %s", sanitizeLog(stationID), sanitizeErr(err))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("{}"))
}

// HandleTuneInDeleteFavorite handles DELETE /bmx/tunein/v1/favorite/{stationID}.
func (s *Server) HandleTuneInDeleteFavorite(w http.ResponseWriter, r *http.Request) {
	stationID := strings.TrimSpace(chi.URLParam(r, "stationID"))
	if err := s.ds.DeleteTuneInFavorite(stationID); err != nil {
		log.Printf("Failed to delete TuneIn favorite %s: %s", sanitizeLog(stationID), sanitizeErr(err))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("{}"))
}

// HandleTuneInService returns the TuneIn service descriptor (the bare
// GET /bmx/tunein endpoint the registry advertises as the service's `self`
// link). It is the TUNEIN entry of bmx_services.json with the same
// {BMX_SERVER} / {MEDIA_SERVER} substitution the registry applies.
func (s *Server) HandleTuneInService(w http.ResponseWriter, _ *http.Request) {
	svc, err := extractBMXService(bmxServicesJSON, "TUNEIN")
	if err != nil {
		log.Printf("[BMX TuneIn] failed to extract service descriptor: %v", sanitizeErr(err))
		http.Error(w, "service descriptor unavailable", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(s.applyBMXTemplate(string(svc))))
}
