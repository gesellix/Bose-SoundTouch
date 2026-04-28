package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"strconv"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/spotify"
	"github.com/go-chi/chi/v5"
)

// HandleBoseToken handles the Bose-specific token refresh request from the speaker.
// POST /oauth/device/{deviceID}/music/musicprovider/{sourceID}/token/cs1
// POST /oauth/device/{deviceID}/music/musicprovider/{sourceID}/token/cs3
func (s *Server) HandleBoseToken(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceID")

	for _, provider := range constants.StaticProviders {
		if strconv.Itoa(provider.ID) != sourceID {
			continue
		}

		switch provider.Name {
		case constants.ProviderSpotify:
			s.HandleBoseSpotifyToken(w, r)
			return
		case constants.ProviderAmazon:
			s.HandleBoseAmazonToken(w, r)
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

// HandleBoseAccountToken handles the Bose-specific token refresh/exchange request from the app.
// POST /oauth/account/{account}/music/musicprovider/{sourceID}/token/cs
func (s *Server) HandleBoseAccountToken(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceID")

	// If it's Spotify, handle it.
	if sourceID == strconv.Itoa(constants.SpotifyProviderID) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[OAuth Proxy] Failed to read body: %v", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)

			return
		}

		_ = r.Body.Close()

		var tokenReq struct {
			GrantType   string `json:"grant_type"`
			Code        string `json:"code"`
			RedirectURI string `json:"redirect_uri"`
		}

		if err := json.Unmarshal(body, &tokenReq); err == nil && tokenReq.GrantType == "authorization_code" {
			log.Printf("[Spotify Proxy] Handling authorization_code grant for account addition")

			s.mu.RLock()
			svc := s.spotifyService
			s.mu.RUnlock()

			if svc == nil {
				log.Printf("[Spotify Proxy] Spotify service not configured")
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)

				return
			}

			if err := svc.ExchangeCodeAndStore(tokenReq.Code); err != nil {
				log.Printf("[Spotify Proxy] Failed to exchange code: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)

				return
			}

			// After successful exchange, we can return the token for the newly added account.
			// HandleBoseSpotifyToken will pick the first account, which is fine if this is the only one.
			s.HandleBoseSpotifyToken(w, r)

			return
		}
	}

	s.HandleBoseSpotifyToken(w, r)
}

// HandleBoseAmazonToken is a stub for Amazon Music OAuth token handling.
// POST /oauth/device/{deviceID}/music/musicprovider/20/token/cs1
// Amazon Music API integration is not yet implemented.
func (s *Server) HandleBoseAmazonToken(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceID")
	log.Printf("[Amazon] Token request for device %s — Amazon Music not yet supported", deviceID)
	http.Error(w, "Amazon Music integration not yet supported", http.StatusNotImplemented)
}

// HandleBoseSpotifyToken handles the Bose-specific Spotify token refresh request.
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
	// However, if the request provides a "secret" (which we use as our Bose surrogate token),
	// we should use that to find the specific account.
	var (
		account     *spotify.Account
		accessToken string
		userID      string
	)

	// Spotify registration/refresh often passes the secret in the body as "refresh_token"
	// or in the registration flow as "code".
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	var tokenReq struct {
		RefreshToken string `json:"refresh_token"`
		GrantType    string `json:"grant_type"`
		Code         string `json:"code"`
	}

	_ = json.Unmarshal(body, &tokenReq)

	secret := tokenReq.RefreshToken
	if secret == "" {
		secret = tokenReq.Code
	}

	if secret != "" {
		if acc, ok := svc.GetAccountBySecret(secret); ok {
			account = acc
			log.Printf("[Spotify Proxy] Found account for secret %s: %s", secret, acc.UserID)
		}
	}

	if account != nil {
		if err := svc.RefreshAccessToken(account); err != nil {
			log.Printf("[Spotify Proxy] Failed to refresh token for %s: %v. Falling back to upstream", account.UserID, err)
			s.HandleBoseProxy(w, r)

			return
		}

		accessToken = account.AccessToken
	} else {
		// Fallback to first account for backward compatibility or when secret is missing
		var err error

		accessToken, userID, err = svc.GetFreshToken()
		if err != nil {
			log.Printf("[Spotify Proxy] Failed to get fresh token: %v. Falling back to upstream", err)
			s.HandleBoseProxy(w, r)

			return
		}

		log.Printf("[Spotify Proxy] Using default account %s", userID)
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
