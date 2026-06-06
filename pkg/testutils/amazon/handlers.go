// Package amazon provides shared handlers for mocking the Amazon LWA API.
package amazon

import (
	"encoding/json"
	"log"
	"net/http"
)

// NewAmazonHandler returns a new http.Handler configured with Amazon LWA mock endpoints.
func NewAmazonHandler() http.Handler {
	mux := http.NewServeMux()

	// LWA Token Endpoint (POST body credentials, not Basic Auth)
	mux.HandleFunc("/auth/o2/token", HandleToken)

	// LWA User Profile Endpoint
	mux.HandleFunc("/user/profile", HandleProfile)

	// Readiness probe (used by the CI compose healthcheck)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	return mux
}

// HandleToken simulates the Amazon LWA token endpoint.
// Amazon requires client_id and client_secret as POST body fields, not HTTP Basic Auth.
func HandleToken(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Amazon Mock] Token request: %s", sanitizeLog(r.Method))

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")
	log.Printf("[Amazon Mock] Grant type: %s", sanitizeLog(grantType))

	resp := map[string]interface{}{
		"access_token":  "Atza|amazon-access-token",
		"token_type":    "bearer",
		"expires_in":    3600,
		"refresh_token": "Atzr|amazon-refresh-token",
	}

	switch grantType {
	case "authorization_code":
		if r.FormValue("code") == "" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
	case "refresh_token":
		if r.FormValue("refresh_token") == "" {
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

// HandleProfile simulates the Amazon LWA user profile endpoint.
// LWA returns "user_id" and "name" (not "id" / "display_name" like Spotify).
func HandleProfile(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Amazon Mock] Profile request: %s", sanitizeLog(r.Method))

	auth := r.Header.Get("Authorization")
	if auth != "Bearer Atza|amazon-access-token" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	resp := map[string]interface{}{
		"user_id": "amzn1.account.TESTUSER",
		"name":    "Amazon Test User",
		"email":   "amazon-test@example.com",
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding profile response: %v", err)
	}
}
