package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/marge"
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
func (s *Server) HandleMgmtSpotifyInit(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	svc := s.spotifyService
	s.mu.RUnlock()

	if svc == nil {
		http.Error(w, `{"error":"spotify not configured"}`, http.StatusServiceUnavailable)
		return
	}

	state := r.URL.Query().Get("account")
	redirectURL := svc.BuildAuthorizeURL(state)

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
		// html.EscapeString neutralises any HTML metacharacters in the
		// caller-supplied error string before it lands in the response.
		_, _ = w.Write([]byte(`<html><body><h1>Spotify Authorization Failed</h1><p>Error: ` + html.EscapeString(errMsg) + `</p></body></html>`))

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

	// Register account in Marge and notify speakers
	accountID := r.URL.Query().Get("account")
	if accountID == "" {
		accountID = r.URL.Query().Get("state")
	}

	s.bridgeSpotifyToMarge(accountID)

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

	// Register account in Marge and notify speakers
	accountID := r.URL.Query().Get("account")
	if accountID == "" {
		accountID = r.URL.Query().Get("state")
	}

	s.bridgeSpotifyToMarge(accountID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Server) bridgeSpotifyToMarge(accountID string) {
	if accountID == "" {
		accountID = "default"
	}

	s.mu.RLock()
	svc := s.spotifyService
	s.mu.RUnlock()

	if svc == nil {
		return
	}

	accounts := svc.GetAccounts()
	if len(accounts) == 0 {
		return
	}

	// For now, we use the first account found or match by ID if possible.
	// In this bridge, we'll ensure all linked Spotify accounts are registered in Marge.
	for _, acc := range accounts {
		log.Printf("[Spotify Bridge] Registering Spotify user %s in Marge for account %s", sanitizeLog(acc.UserID), sanitizeLog(accountID))

		// 1. Register in Marge (updates configuredsources.xml for all devices in the account)
		// We use the BoseSecret as the credential instead of the AccessToken
		credential := acc.BoseSecret
		if credential == "" {
			// Fallback to AccessToken if BoseSecret is not available (for old accounts)
			credential = acc.AccessToken
		}

		_, err := marge.AddSource(s.ds, accountID, acc.UserID, strconv.Itoa(constants.SpotifyProviderID), credential, "token_version_3", acc.DisplayName)
		if err != nil {
			log.Printf("[Spotify Bridge] Failed to register source in Marge: %v", err)
			continue
		}

		// 2. Notify discovered speakers via LISA API (/setMusicServiceOAuthAccount)
		allDevices, err := s.ds.ListAllDevices()
		if err != nil {
			log.Printf("[Spotify Bridge] Failed to list devices: %v", err)
			continue
		}

		for i := range allDevices {
			dev := &allDevices[i]
			if dev.AccountID != accountID && accountID != "default" {
				continue
			}

			if dev.IPAddress == "" {
				continue
			}

			go func(d models.ServiceDeviceInfo) {
				log.Printf("[Spotify Bridge] Notifying speaker %s (%s) about new Spotify account", sanitizeLog(d.Name), sanitizeLog(d.IPAddress))

				c := client.NewClientFromHost(d.IPAddress)
				creds := models.NewSpotifyOAuthCredentials(acc.UserID, credential, acc.DisplayName)

				if err := c.SetMusicServiceOAuthAccount(creds); err != nil {
					log.Printf("[Spotify Bridge] Failed to notify speaker %s via OAuth: %v", sanitizeLog(d.Name), err)

					// Fallback if OAuth is not supported (Error 1029)
					errs := &models.ErrorsResponse{}
					if errors.As(err, &errs) {
						isUnsupported := false

						for _, e := range errs.Errors {
							if e.Value == 1029 {
								isUnsupported = true
								break
							}
						}

						if isUnsupported {
							log.Printf("[Spotify Bridge] Speaker %s doesn't support OAuth, falling back to Marge sync notification", sanitizeLog(d.Name))

							// Some speakers (especially Stockholm-based) don't support /setMusicServiceOAuthAccount
							// via LISA but will pick up the new source from Marge if notified.
							if err := c.NotifySourcesUpdated(d.DeviceID); err != nil {
								log.Printf("[Spotify Bridge] Sync notification failed for speaker %s: %v", sanitizeLog(d.Name), err)

								// Final fallback to legacy account creation
								log.Printf("[Spotify Bridge] Falling back to legacy account creation for speaker %s", sanitizeLog(d.Name))

								legacyCreds := models.NewSpotifyCredentials(acc.UserID, credential)
								if err := c.SetMusicServiceAccount(legacyCreds); err != nil {
									log.Printf("[Spotify Bridge] Legacy fallback failed for speaker %s: %v", sanitizeLog(d.Name), err)
								} else {
									log.Printf("[Spotify Bridge] Legacy fallback successful for speaker %s", sanitizeLog(d.Name))
								}
							} else {
								log.Printf("[Spotify Bridge] Sync notification successful for speaker %s", sanitizeLog(d.Name))
							}

							return
						}
					}
				} else {
					log.Printf("[Spotify Bridge] Successfully notified speaker %s", sanitizeLog(d.Name))
				}
			}(*dev)
		}
	}
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

// HandleMgmtSpotifyToken returns a fresh Spotify access token for the linked account.
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
		log.Printf("[Mgmt] Spotify entity resolve error: %s", sanitizeErr(err))
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

// HandleMgmtPrimeDevice triggers a Spotify priming for a specific device.
func (s *Server) HandleMgmtPrimeDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceId")

	if deviceID == "" {
		http.Error(w, `{"error":"missing deviceId"}`, http.StatusBadRequest)
		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		log.Printf("[Mgmt] Prime failed: %s", sanitizeErr(err))
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusNotFound)

		return
	}

	// Trigger priming
	go s.PrimeDeviceWithSpotify(deviceIP)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"Priming triggered"}`))
}

// HandleMgmtAmazonInit starts the Amazon OAuth flow by returning an authorization URL.
func (s *Server) HandleMgmtAmazonInit(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	svc := s.amazonService
	s.mu.RUnlock()

	if svc == nil {
		http.Error(w, `{"error":"amazon not configured"}`, http.StatusServiceUnavailable)
		return
	}

	state := r.URL.Query().Get("account")
	redirectURL := svc.BuildAuthorizeURL(state)

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(map[string]string{
		"redirectUrl": redirectURL,
	}); err != nil {
		log.Printf("[Mgmt] Failed to encode Amazon redirect URL: %v", err)
	}
}

// HandleMgmtAmazonCallback is the browser OAuth callback from Amazon LWA.
// Not protected by Basic Auth — Amazon redirects the user's browser here directly.
// Returns an HTML page the user can close.
func (s *Server) HandleMgmtAmazonCallback(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	svc := s.amazonService
	s.mu.RUnlock()

	if svc == nil {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<html><body><h1>Error</h1><p>Amazon Music integration not configured</p></body></html>`))

		return
	}

	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		// html.EscapeString neutralises any HTML metacharacters in the
		// caller-supplied error string before it lands in the response.
		_, _ = w.Write([]byte(`<html><body><h1>Amazon Authorization Failed</h1><p>Error: ` + html.EscapeString(errMsg) + `</p></body></html>`))

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
		log.Printf("[Mgmt] Amazon callback failed: %v", err)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<html><body><h1>Error</h1><p>Token exchange failed</p></body></html>`))

		return
	}

	accountID := r.URL.Query().Get("account")
	if accountID == "" {
		accountID = r.URL.Query().Get("state")
	}

	s.bridgeAmazonToMarge(accountID)

	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<html><body><h1>Amazon Music Connected</h1><p>You can close this window.</p></body></html>`))
}

// HandleMgmtAmazonConfirm exchanges an authorization code for tokens.
// Used by the ueberboese mobile app after the deep link callback delivers the code.
// Protected by Basic Auth.
func (s *Server) HandleMgmtAmazonConfirm(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	svc := s.amazonService
	s.mu.RUnlock()

	if svc == nil {
		http.Error(w, `{"error":"amazon not configured"}`, http.StatusServiceUnavailable)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, `{"error":"missing code parameter"}`, http.StatusBadRequest)
		return
	}

	if err := svc.ExchangeCodeAndStore(code); err != nil {
		log.Printf("[Mgmt] Amazon confirm failed: %v", err)
		http.Error(w, `{"error":"token exchange failed"}`, http.StatusInternalServerError)

		return
	}

	accountID := r.URL.Query().Get("account")
	if accountID == "" {
		accountID = r.URL.Query().Get("state")
	}

	s.bridgeAmazonToMarge(accountID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Server) bridgeAmazonToMarge(accountID string) {
	if accountID == "" {
		accountID = "default"
	}

	s.mu.RLock()
	svc := s.amazonService
	s.mu.RUnlock()

	if svc == nil {
		return
	}

	accounts := svc.GetAllAccounts()
	if len(accounts) == 0 {
		return
	}

	for _, acc := range accounts {
		log.Printf("[Amazon Bridge] Registering Amazon user %s in Marge for account %s", sanitizeLog(acc.Email), sanitizeLog(accountID))

		// Build the AmazonSecret credential envelope expected by the speaker firmware.
		credMap := map[string]interface{}{
			"AmazonSecret": map[string]string{
				"refresh_token": acc.RefreshToken,
				"site_id":       acc.SiteID,
			},
		}

		credJSON, err := json.Marshal(credMap)
		if err != nil {
			log.Printf("[Amazon Bridge] Failed to marshal credential: %v", err)
			continue
		}

		_, err = marge.AddSource(s.ds, accountID, acc.Email, strconv.Itoa(constants.AmazonProviderID), string(credJSON), constants.CredentialTypeToken, acc.DisplayName)
		if err != nil {
			log.Printf("[Amazon Bridge] Failed to register source in Marge: %v", err)
			continue
		}

		allDevices, err := s.ds.ListAllDevices()
		if err != nil {
			log.Printf("[Amazon Bridge] Failed to list devices: %v", err)
			continue
		}

		for i := range allDevices {
			dev := &allDevices[i]
			if dev.AccountID != accountID && accountID != "default" {
				continue
			}

			if dev.IPAddress == "" {
				continue
			}

			go func(d models.ServiceDeviceInfo) {
				log.Printf("[Amazon Bridge] Notifying speaker %s (%s) about new Amazon account", sanitizeLog(d.Name), sanitizeLog(d.IPAddress))

				cfg := client.DefaultConfig()
				cfg.Host = d.IPAddress
				cfg.Timeout = 5 * time.Second
				c := client.NewClient(cfg)
				creds := models.NewAmazonOAuthCredentials(acc.Email, string(credJSON), acc.DisplayName)

				if err := c.SetMusicServiceOAuthAccount(creds); err != nil {
					log.Printf("[Amazon Bridge] Failed to notify speaker %s via OAuth: %v", sanitizeLog(d.Name), err)
					log.Printf("[Amazon Bridge] Speaker %s doesn't support OAuth or is unreachable, falling back to Marge sync notification", sanitizeLog(d.Name))

					if err := c.NotifySourcesUpdated(d.DeviceID); err != nil {
						log.Printf("[Amazon Bridge] Sync notification failed for speaker %s: %v", sanitizeLog(d.Name), err)
						log.Printf("[Amazon Bridge] Falling back to legacy account creation for speaker %s", sanitizeLog(d.Name))

						legacyCreds := models.NewAmazonMusicCredentials(acc.Email, string(credJSON))
						if err := c.SetMusicServiceAccount(legacyCreds); err != nil {
							log.Printf("[Amazon Bridge] Legacy fallback failed for speaker %s: %v", sanitizeLog(d.Name), err)
						} else {
							log.Printf("[Amazon Bridge] Legacy fallback successful for speaker %s", sanitizeLog(d.Name))
						}
					} else {
						log.Printf("[Amazon Bridge] Sync notification successful for speaker %s", sanitizeLog(d.Name))
					}
				} else {
					log.Printf("[Amazon Bridge] Successfully notified speaker %s", sanitizeLog(d.Name))
				}
			}(*dev)
		}
	}
}

// HandleMgmtAmazonAccounts returns linked Amazon accounts (tokens stripped).
func (s *Server) HandleMgmtAmazonAccounts(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	svc := s.amazonService
	s.mu.RUnlock()

	if svc == nil {
		http.Error(w, `{"error":"amazon not configured"}`, http.StatusServiceUnavailable)
		return
	}

	accounts := svc.GetAccounts()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"accounts": accounts,
	}); err != nil {
		log.Printf("[Mgmt] Failed to encode Amazon accounts: %v", err)
	}
}

// HandleMgmtAmazonToken returns a fresh Amazon access token for the linked account.
func (s *Server) HandleMgmtAmazonToken(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	svc := s.amazonService
	s.mu.RUnlock()

	if svc == nil {
		http.Error(w, `{"error":"amazon not configured"}`, http.StatusServiceUnavailable)
		return
	}

	accessToken, username, err := svc.GetFreshToken()
	if err != nil {
		log.Printf("[Mgmt] Amazon token error: %v", err)
		http.Error(w, `{"error":"no token available"}`, http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]string{
		"access_token": accessToken,
		"username":     username,
	}); err != nil {
		log.Printf("[Mgmt] Failed to encode Amazon token: %v", err)
	}
}

// HandleMgmtPrimeDeviceAmazon triggers Amazon Music priming for a specific device.
func (s *Server) HandleMgmtPrimeDeviceAmazon(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceId")

	if deviceID == "" {
		http.Error(w, `{"error":"missing deviceId"}`, http.StatusBadRequest)
		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		log.Printf("[Mgmt] Amazon prime failed: %s", sanitizeErr(err))
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusNotFound)

		return
	}

	go s.PrimeDeviceWithAmazon(deviceIP)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"Priming triggered"}`))
}
