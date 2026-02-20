package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// BasicAuthMgmt returns a Basic Auth middleware using the server's management credentials.
func (s *Server) BasicAuthMgmt() func(http.Handler) http.Handler {
	s.mu.RLock()
	username := s.mgmtUsername
	password := s.mgmtPassword
	s.mu.RUnlock()

	return middleware.BasicAuth("Management API", map[string]string{username: password})
}

// HandleMgmtListSpeakers returns discovered speakers for the given account.
func (s *Server) HandleMgmtListSpeakers(w http.ResponseWriter, r *http.Request) {
	_ = chi.URLParam(r, "accountId")

	allDevices, err := s.ds.ListAllDevices()
	if err != nil {
		log.Printf("[Mgmt] Failed to list devices: %v", err)

		allDevices = nil
	}

	type speaker struct {
		IPAddress string `json:"ipAddress"`
		Name      string `json:"name"`
		DeviceID  string `json:"deviceId"`
		Type      string `json:"type"`
	}

	speakers := make([]speaker, 0, len(allDevices))
	for i := range allDevices {
		d := &allDevices[i]
		speakers = append(speakers, speaker{
			IPAddress: d.IPAddress,
			Name:      d.Name,
			DeviceID:  d.DeviceID,
			Type:      d.ProductCode,
		})
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"speakers": speakers,
	}); err != nil {
		log.Printf("[Mgmt] Failed to encode speakers: %v", err)
	}
}

// HandleMgmtDeviceEvents returns events for a device (currently a placeholder).
func (s *Server) HandleMgmtDeviceEvents(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")

	events := s.ds.GetDeviceEvents(deviceID)
	if events == nil {
		events = nil // will marshal as empty array via wrapper
	}

	w.Header().Set("Content-Type", "application/json")
	// Return the events in the structure the Flutter app expects.
	// Use an explicit empty slice to ensure JSON "[]" instead of "null".
	type eventEntry struct {
		Type string                 `json:"type"`
		Time string                 `json:"time"`
		Data map[string]interface{} `json:"data"`
	}

	result := make([]eventEntry, 0, len(events))
	for _, e := range events {
		result = append(result, eventEntry{
			Type: e.Type,
			Time: e.Time,
			Data: e.Data,
		})
	}

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"events": result,
	}); err != nil {
		log.Printf("[Mgmt] Failed to encode events: %v", err)
	}
}

// HandleMgmtSpotifyInit starts the Spotify OAuth flow by returning an authorization URL.
func (s *Server) HandleMgmtSpotifyInit(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	svc := s.spotifyService
	s.mu.RUnlock()

	if svc == nil {
		http.Error(w, `{"error":"spotify not configured"}`, http.StatusServiceUnavailable)
		return
	}

	redirectURL := svc.BuildAuthorizeURL()

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(map[string]string{
		"redirectUrl": redirectURL,
	}); err != nil {
		log.Printf("[Mgmt] Failed to encode redirect URL: %v", err)
	}
}

// HandleMgmtSpotifyCallback is the browser OAuth callback from Spotify.
// Not protected by Basic Auth — Spotify redirects the user's browser here directly.
// Returns an HTML page the user can close.
func (s *Server) HandleMgmtSpotifyCallback(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	svc := s.spotifyService
	s.mu.RUnlock()

	if svc == nil {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<html><body><h1>Error</h1><p>Spotify integration not configured</p></body></html>`))

		return
	}

	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<html><body><h1>Spotify Authorization Failed</h1><p>Error: ` + errMsg + `</p></body></html>`))

		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<html><body><h1>Missing authorization code</h1></body></html>`))

		return
	}

	if err := svc.ExchangeCodeAndStore(code); err != nil {
		log.Printf("[Mgmt] Spotify callback failed: %v", err)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<html><body><h1>Error</h1><p>Token exchange failed</p></body></html>`))

		return
	}

	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<html><body><h1>Spotify Connected</h1><p>You can close this window.</p></body></html>`))
}

// HandleMgmtSpotifyConfirm exchanges an authorization code for tokens.
// Used by the ueberboese mobile app after the deep link callback delivers the code.
// Protected by Basic Auth.
func (s *Server) HandleMgmtSpotifyConfirm(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	svc := s.spotifyService
	s.mu.RUnlock()

	if svc == nil {
		http.Error(w, `{"error":"spotify not configured"}`, http.StatusServiceUnavailable)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, `{"error":"missing code parameter"}`, http.StatusBadRequest)
		return
	}

	if err := svc.ExchangeCodeAndStore(code); err != nil {
		log.Printf("[Mgmt] Spotify confirm failed: %v", err)
		http.Error(w, `{"error":"token exchange failed"}`, http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// HandleMgmtSpotifyAccounts returns linked Spotify accounts (tokens stripped).
func (s *Server) HandleMgmtSpotifyAccounts(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	svc := s.spotifyService
	s.mu.RUnlock()

	if svc == nil {
		http.Error(w, `{"error":"spotify not configured"}`, http.StatusServiceUnavailable)
		return
	}

	accounts := svc.GetAccounts()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"accounts": accounts,
	}); err != nil {
		log.Printf("[Mgmt] Failed to encode accounts: %v", err)
	}
}

// HandleMgmtSpotifyToken returns a fresh Spotify access token and username.
func (s *Server) HandleMgmtSpotifyToken(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	svc := s.spotifyService
	s.mu.RUnlock()

	if svc == nil {
		http.Error(w, `{"error":"spotify not configured"}`, http.StatusServiceUnavailable)
		return
	}

	accessToken, username, err := svc.GetFreshToken()
	if err != nil {
		log.Printf("[Mgmt] Spotify token error: %v", err)
		http.Error(w, `{"error":"no token available"}`, http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]string{
		"access_token": accessToken,
		"username":     username,
	}); err != nil {
		log.Printf("[Mgmt] Failed to encode token: %v", err)
	}
}

// HandleMgmtSpotifyEntity resolves a Spotify URI to name and image URL.
func (s *Server) HandleMgmtSpotifyEntity(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	svc := s.spotifyService
	s.mu.RUnlock()

	if svc == nil {
		http.Error(w, `{"error":"spotify not configured"}`, http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}

	var request struct {
		URI string `json:"uri"`
	}
	if unmarshalErr := json.Unmarshal(body, &request); unmarshalErr != nil || request.URI == "" {
		http.Error(w, `{"error":"missing or invalid uri"}`, http.StatusBadRequest)
		return
	}

	name, imageURL, err := svc.ResolveEntity(request.URI)
	if err != nil {
		log.Printf("[Mgmt] Spotify entity resolve error: %v", err)
		http.Error(w, `{"error":"entity resolution failed"}`, http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]string{
		"name":     name,
		"imageUrl": imageURL,
	}); err != nil {
		log.Printf("[Mgmt] Failed to encode entity: %v", err)
	}
}
