package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"strconv"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/go-chi/chi/v5"
)

// HandleBoseToken handles the Bose-specific token refresh request from the speaker.
// POST /oauth/device/{deviceID}/music/musicprovider/{sourceID}/token/cs3
func (s *Server) HandleBoseToken(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceID")

	for _, provider := range constants.StaticProviders {
		if strconv.Itoa(provider.ID) == sourceID && provider.Name == "SPOTIFY" {
			s.HandleBoseSpotifyToken(w, r)
			return
		}
	}

	s.HandleBoseProxy(w, r)
}

// HandleBoseLegacyToken handles the Bose-specific token refresh request (legacy or variant).
// POST /oauth/device/{deviceID}/music/musicprovider/{sourceID}/token
func (s *Server) HandleBoseLegacyToken(w http.ResponseWriter, r *http.Request) {
	s.HandleBoseToken(w, r)
}

// HandleBoseSpotifyToken handles the Bose-specific Spotify token refresh request from the speaker.
// POST /oauth/device/{deviceID}/music/musicprovider/15/token/cs3
func (s *Server) HandleBoseSpotifyToken(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceID")
	log.Printf("[Spotify Proxy] Intercepted token request for device %s", deviceID)

	s.mu.RLock()
	svc := s.spotifyService
	s.mu.RUnlock()

	if svc == nil {
		log.Printf("[Spotify Proxy] Spotify service not configured, falling back to upstream")
		s.HandleBoseProxy(w, r)

		return
	}

	accounts := svc.GetAccounts()
	if len(accounts) == 0 {
		log.Printf("[Spotify Proxy] No Spotify accounts linked, falling back to upstream")
		s.HandleBoseProxy(w, r)

		return
	}

	// We use the first linked account.
	accessToken, _, err := svc.GetFreshToken()
	if err != nil {
		log.Printf("[Spotify Proxy] Failed to get fresh token: %v. Falling back to upstream", err)
		s.HandleBoseProxy(w, r)

		return
	}

	// Format response as expected by Bose firmware.
	// Based on observed interactions, it's a JSON object with access_token.
	// The "scope" and other fields might be needed by some firmware versions.
	response := map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
		// These scopes are typical for what Bose requests.
		"scope": "playlist-read-private playlist-read-collaborative streaming user-library-read user-library-modify playlist-modify-private playlist-modify-public user-read-email user-read-private user-top-read",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Proxy-Origin", "self")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[Spotify Proxy] Failed to encode response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
