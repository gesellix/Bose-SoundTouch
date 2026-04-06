// Package spotify provides shared handlers for mocking the Spotify API.
package spotify

import (
	"encoding/json"
	"log"
	"net/http"
)

// NewSpotifyHandler returns a new http.Handler configured with Spotify mock endpoints.
func NewSpotifyHandler() http.Handler {
	mux := http.NewServeMux()

	// OAuth Token Endpoint
	mux.HandleFunc("/api/token", HandleToken)

	// User Profile Endpoint
	mux.HandleFunc("/v1/me", HandleMe)
	mux.HandleFunc("/me", HandleMe)

	return mux
}

// HandleToken simulates the Spotify OAuth token endpoint.
func HandleToken(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Spotify Mock] Token request: %s", r.Method)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")
	log.Printf("[Spotify Mock] Grant type: %s", grantType)

	resp := map[string]interface{}{
		"access_token":  "spotify-access-token",
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": "spotify-refresh-token",
		"scope":         "user-read-private user-read-email",
	}

	switch grantType {
	case "authorization_code":
		code := r.FormValue("code")
		if code == "" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
	case "refresh_token":
		refreshToken := r.FormValue("refresh_token")
		if refreshToken == "" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, `{"error":"unsupported_grant_type"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding token response: %v", err)
	}
}

// HandleMe simulates the Spotify user profile endpoint.
func HandleMe(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Spotify Mock] Profile request: %s", r.Method)

	auth := r.Header.Get("Authorization")
	if auth != "Bearer spotify-access-token" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	resp := map[string]interface{}{
		"id":           "spotify-user-id",
		"display_name": "Spotify Test User",
		"email":        "spotify-test@example.com",
		"uri":          "spotify:user:spotify-user-id",
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding profile response: %v", err)
	}
}
