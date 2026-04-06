// Package spotify provides Spotify OAuth integration and token management
// for the SoundTouch service, ported from soundcork's Python implementation.
package spotify

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// SpotifyAuthorizeURL is the Spotify OAuth authorization endpoint.
	SpotifyAuthorizeURL = "https://accounts.spotify.com/authorize"
	// SpotifyTokenURL is the Spotify OAuth token endpoint.
	SpotifyTokenURL = "https://accounts.spotify.com/api/token"
	// SpotifyAPIBase is the base URL for the Spotify Web API.
	SpotifyAPIBase = "https://api.spotify.com/v1"
	// SpotifyScopes are the OAuth scopes required for speaker playback and user info.
	SpotifyScopes = "streaming user-read-private user-read-email user-read-playback-state user-modify-playback-state"
)

// Account represents a stored Spotify account with tokens.
type Account struct {
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	BoseSecret   string `json:"bose_secret,omitempty"`
}

// Service manages Spotify OAuth flow and token lifecycle.
type Service struct {
	clientID     string
	clientSecret string
	redirectURI  string
	dataDir      string
	mu           sync.RWMutex
	accounts     map[string]*Account

	// Overridable URLs for testing
	tokenURL string
	apiBase  string
}

// NewSpotifyService creates a new Service and loads any persisted accounts.
func NewSpotifyService(clientID, clientSecret, redirectURI, dataDir string) *Service {
	return &Service{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		dataDir:      dataDir,
		accounts:     make(map[string]*Account),
		tokenURL:     SpotifyTokenURL,
		apiBase:      SpotifyAPIBase,
	}
}

// Load loads persisted accounts from disk.
func (s *Service) Load() error {
	if err := s.load(); err != nil {
		return err
	}

	return nil
}

// SetEndpoints allows overriding default Spotify API endpoints (for testing).
func (s *Service) SetEndpoints(tokenURL, apiBase string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokenURL = tokenURL
	s.apiBase = apiBase
}

// BuildAuthorizeURL constructs the Spotify OAuth authorization URL.
func (s *Service) BuildAuthorizeURL(state string) string {
	params := url.Values{
		"client_id":     {s.clientID},
		"response_type": {"code"},
		"redirect_uri":  {s.redirectURI},
		"scope":         {SpotifyScopes},
	}
	if state != "" {
		params.Set("state", state)
	}

	return SpotifyAuthorizeURL + "?" + params.Encode()
}

// ExchangeCodeAndStore exchanges an authorization code for tokens,
// fetches the user profile, and stores the account.
func (s *Service) ExchangeCodeAndStore(code string) error {
	// Exchange code for tokens
	tokenResp, err := s.exchangeCode(code)
	if err != nil {
		return fmt.Errorf("token exchange: %w", err)
	}

	accessToken, _ := tokenResp["access_token"].(string)
	refreshToken, _ := tokenResp["refresh_token"].(string)

	expiresIn, _ := tokenResp["expires_in"].(float64)
	if expiresIn == 0 {
		expiresIn = 3600
	}

	// Fetch user profile
	profile, err := s.getUserProfile(accessToken)
	if err != nil {
		return fmt.Errorf("fetch profile: %w", err)
	}

	userID, _ := profile["id"].(string)
	displayName, _ := profile["display_name"].(string)
	email, _ := profile["email"].(string)

	// Generate a Bose surrogate secret (represented as a 32-char hex string)
	boseSecret := s.generateBoseSecret()

	account := &Account{
		UserID:       userID,
		DisplayName:  displayName,
		Email:        email,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Unix() + int64(expiresIn),
		BoseSecret:   boseSecret,
	}

	s.mu.Lock()
	s.accounts[userID] = account
	s.mu.Unlock()

	if err := s.save(); err != nil {
		return fmt.Errorf("save accounts: %w", err)
	}

	log.Printf("[Spotify] Account linked: %s (%s)", displayName, userID)

	return nil
}

func (s *Service) exchangeCode(code string) (map[string]interface{}, error) {
	data := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {s.redirectURI},
	}

	req, err := http.NewRequest(http.MethodPost, s.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.clientID, s.clientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return result, nil
}

func (s *Service) getUserProfile(accessToken string) (map[string]interface{}, error) {
	req, err := http.NewRequest(http.MethodGet, s.apiBase+"/me", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("profile request: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("profile fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}

	return result, nil
}

// RefreshAccessToken refreshes the access token for the given account.
func (s *Service) RefreshAccessToken(account *Account) error {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {account.RefreshToken},
	}

	req, err := http.NewRequest(http.MethodPost, s.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.clientID, s.clientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	s.mu.Lock()
	account.AccessToken, _ = result["access_token"].(string)

	expiresIn, _ := result["expires_in"].(float64)
	if expiresIn == 0 {
		expiresIn = 3600
	}

	account.ExpiresAt = time.Now().Unix() + int64(expiresIn)
	if newRefresh, ok := result["refresh_token"].(string); ok && newRefresh != "" {
		account.RefreshToken = newRefresh
	}
	s.mu.Unlock()

	if err := s.save(); err != nil {
		return fmt.Errorf("save accounts: %w", err)
	}

	return nil
}

// GetFreshToken returns a valid access token and username, refreshing if needed.
func (s *Service) GetFreshToken() (accessToken, username string, err error) {
	s.mu.RLock()

	if len(s.accounts) == 0 {
		s.mu.RUnlock()
		return "", "", fmt.Errorf("no Spotify accounts linked")
	}

	// Get the first account
	var account *Account
	for _, a := range s.accounts {
		account = a
		break
	}

	s.mu.RUnlock()

	// Check if token needs refresh (expired or within 60s of expiry)
	if account.ExpiresAt < time.Now().Unix()+60 {
		if err := s.RefreshAccessToken(account); err != nil {
			return "", "", fmt.Errorf("refresh token: %w", err)
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return account.AccessToken, account.UserID, nil
}

// GetAccounts returns a copy of all accounts with tokens stripped for API responses.
func (s *Service) GetAccounts() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Account, 0, len(s.accounts))
	for _, a := range s.accounts {
		result = append(result, Account{
			UserID:      a.UserID,
			DisplayName: a.DisplayName,
			Email:       a.Email,
			ExpiresAt:   a.ExpiresAt,
			BoseSecret:  a.BoseSecret,
			// AccessToken and RefreshToken deliberately omitted
		})
	}

	return result
}

// GetAccountBySecret retrieves a Spotify account by its Bose surrogate secret.
func (s *Service) GetAccountBySecret(secret string) (*Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, a := range s.accounts {
		if a.BoseSecret == secret {
			return a, true
		}
	}

	return nil, false
}

func (s *Service) generateBoseSecret() string {
	prefix := "bs-"

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based if RNG fails
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}

	return prefix + hex.EncodeToString(b)
}

// ResolveEntity resolves a Spotify URI to a name and image URL.
func (s *Service) ResolveEntity(uri string) (name, imageURL string, err error) {
	entityType, entityID, err := parseSpotifyURI(uri)
	if err != nil {
		return "", "", err
	}

	accessToken, _, err := s.GetFreshToken()
	if err != nil {
		return "", "", fmt.Errorf("get token: %w", err)
	}

	apiURL := fmt.Sprintf("%s/%s/%s", s.apiBase, entityType, entityID)

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("API request: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return "", "", fmt.Errorf("spotify entity not found")
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("spotify API error (%d): %s", resp.StatusCode, string(body))
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", "", fmt.Errorf("parse response: %w", err)
	}

	name, _ = data["name"].(string)
	if name == "" {
		name = "Unknown"
	}

	// Extract image URL — location varies by entity type
	imageURL = extractImageURL(data, entityType)

	return name, imageURL, nil
}

// extractImageURL extracts the first image URL from a Spotify API response.
// For tracks, images are stored on the album object.
func extractImageURL(data map[string]interface{}, entityType string) string {
	images, _ := data["images"].([]interface{})
	if len(images) == 0 && entityType == "tracks" {
		// Tracks store images on the album
		album, _ := data["album"].(map[string]interface{})
		if album != nil {
			images, _ = album["images"].([]interface{})
		}
	}

	if len(images) > 0 {
		if img, ok := images[0].(map[string]interface{}); ok {
			url, _ := img["url"].(string)
			return url
		}
	}

	return ""
}

// parseSpotifyURI parses a Spotify URI like "spotify:track:abc" into
// the pluralized API type ("tracks") and ID ("abc").
func parseSpotifyURI(uri string) (entityType, entityID string, err error) {
	parts := strings.Split(uri, ":")
	if len(parts) != 3 || parts[0] != "spotify" {
		return "", "", fmt.Errorf("invalid Spotify URI format: %s", uri)
	}

	typ := parts[1]
	id := parts[2]

	validTypes := map[string]string{
		"track":    "tracks",
		"album":    "albums",
		"playlist": "playlists",
		"artist":   "artists",
	}

	plural, ok := validTypes[typ]
	if !ok {
		return "", "", fmt.Errorf("unsupported Spotify entity type: %s", typ)
	}

	return plural, id, nil
}

// save persists accounts to disk as JSON.
func (s *Service) save() error {
	s.mu.RLock()

	data := make(map[string]*Account, len(s.accounts))
	for k, v := range s.accounts {
		data[k] = v
	}

	s.mu.RUnlock()

	dir := filepath.Join(s.dataDir, "spotify")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal accounts: %w", err)
	}

	path := filepath.Join(dir, "accounts.json")
	if err := os.WriteFile(path, jsonData, 0600); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// load reads persisted accounts from disk.
func (s *Service) load() error {
	path := filepath.Join(s.dataDir, "spotify", "accounts.json")

	jsonData, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No accounts file yet, not an error
		}

		return fmt.Errorf("read file: %w", err)
	}

	var accounts map[string]*Account
	if err := json.Unmarshal(jsonData, &accounts); err != nil {
		return fmt.Errorf("unmarshal accounts: %w", err)
	}

	s.mu.Lock()
	s.accounts = accounts
	s.mu.Unlock()

	log.Printf("[Spotify] Loaded %d account(s)", len(accounts))

	return nil
}
