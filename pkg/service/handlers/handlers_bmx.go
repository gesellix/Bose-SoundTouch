// Package handlers provides HTTP handlers for the SoundTouch service.
package handlers

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
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

// HandleTuneInPlayback returns TuneIn playback information.
func (s *Server) HandleTuneInPlayback(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "" {
		s.writeBMXUnauthorized(w)
		return
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
	if r.Header.Get("Authorization") == "" {
		s.writeBMXUnauthorized(w)
		return
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
	if r.Header.Get("Authorization") == "" {
		s.writeBMXUnauthorized(w)
		return
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
	if r.Header.Get("Authorization") == "" {
		s.writeBMXUnauthorized(w)
		return
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

// HandleTuneInReport handles TuneIn playback reporting.
func (s *Server) HandleTuneInReport(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "" {
		s.writeBMXUnauthorized(w)
		return
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
	if r.Header.Get("Authorization") == "" {
		s.writeBMXUnauthorized(w)
		return
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
	if r.Header.Get("Authorization") == "" {
		s.writeBMXUnauthorized(w)
		return
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
