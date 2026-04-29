// Package amazon provides Amazon Music (Login with Amazon) OAuth integration
// and token management for the SoundTouch service.
package amazon

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
	// AmazonAuthorizeURL is the Login with Amazon (LWA) authorization endpoint.
	AmazonAuthorizeURL = "https://www.amazon.com/ap/oa"
	// AmazonTokenURL is the LWA token endpoint.
	AmazonTokenURL = "https://api.amazon.com/auth/o2/token"
	// AmazonProfileURL is the LWA user profile endpoint.
	AmazonProfileURL = "https://api.amazon.com/user/profile"
	// AmazonScopes are the OAuth scopes for account linking.
	// Expand to "music::*" scopes once Amazon Music API access is available.
	AmazonScopes = "profile"
)

// Account represents a stored Amazon account with tokens.
type Account struct {
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	BoseSecret   string `json:"bose_secret,omitempty"`
}

// Service manages Amazon OAuth flow and token lifecycle.
type Service struct {
	clientID     string
	clientSecret string
	redirectURI  string
	dataDir      string
	mu           sync.RWMutex
	accounts     map[string]*Account

	// Overridable URLs for testing
	tokenURL   string
	profileURL string
}

// NewAmazonService creates a new Service and loads any persisted accounts.
func NewAmazonService(clientID, clientSecret, redirectURI, dataDir string) *Service {
	return &Service{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		dataDir:      dataDir,
		accounts:     make(map[string]*Account),
		tokenURL:     AmazonTokenURL,
		profileURL:   AmazonProfileURL,
	}
}

// Load loads persisted accounts from disk.
func (s *Service) Load() error {
	return s.load()
}

// SetEndpoints allows overriding default Amazon API endpoints (for testing).
func (s *Service) SetEndpoints(tokenURL, profileURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokenURL = tokenURL
	s.profileURL = profileURL
}

// BuildAuthorizeURL constructs the LWA OAuth authorization URL.
func (s *Service) BuildAuthorizeURL(state string) string {
	params := url.Values{
		"client_id":     {s.clientID},
		"response_type": {"code"},
		"redirect_uri":  {s.redirectURI},
		"scope":         {AmazonScopes},
	}
	if state != "" {
		params.Set("state", state)
	}

	return AmazonAuthorizeURL + "?" + params.Encode()
}

// ExchangeCodeAndStore exchanges an authorization code for tokens,
// fetches the user profile, and stores the account.
func (s *Service) ExchangeCodeAndStore(code string) error {
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

	profile, err := s.getUserProfile(accessToken)
	if err != nil {
		return fmt.Errorf("fetch profile: %w", err)
	}

	// LWA profile uses "user_id" and "name" (not "id" and "display_name" like Spotify).
	userID, _ := profile["user_id"].(string)
	displayName, _ := profile["name"].(string)
	email, _ := profile["email"].(string)

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

	log.Printf("[Amazon] Account linked: %s (%s)", displayName, userID)

	return nil
}

// exchangeCode exchanges an authorization code for tokens.
// Amazon LWA requires client_id and client_secret as POST body fields,
// not as HTTP Basic Auth (unlike Spotify).
func (s *Service) exchangeCode(code string) (map[string]interface{}, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {s.redirectURI},
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
	}

	req, err := http.NewRequest(http.MethodPost, s.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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
	req, err := http.NewRequest(http.MethodGet, s.profileURL, nil)
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
// Amazon LWA requires client credentials as POST body fields.
func (s *Service) RefreshAccessToken(account *Account) error {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {account.RefreshToken},
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
	}

	req, err := http.NewRequest(http.MethodPost, s.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

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
		return "", "", fmt.Errorf("no Amazon accounts linked")
	}

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

// GetAccountBySecret retrieves an Amazon account by its Bose surrogate secret.
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

// GetAccountByRefreshToken retrieves an Amazon account by its current refresh token.
// Used by the token handler because the speaker sends back the actual LWA refresh token
// (extracted from the AmazonSecret JSON in Sources.xml), not a surrogate.
func (s *Service) GetAccountByRefreshToken(refreshToken string) (*Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, a := range s.accounts {
		if a.RefreshToken == refreshToken {
			return a, true
		}
	}

	return nil, false
}

func (s *Service) generateBoseSecret() string {
	prefix := "ba-"

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}

	return prefix + hex.EncodeToString(b)
}

// save persists accounts to disk as JSON.
func (s *Service) save() error {
	s.mu.RLock()

	data := make(map[string]*Account, len(s.accounts))
	for k, v := range s.accounts {
		data[k] = v
	}

	s.mu.RUnlock()

	dir := filepath.Join(s.dataDir, "amazon")
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
	path := filepath.Join(s.dataDir, "amazon", "accounts.json")

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

	log.Printf("[Amazon] Loaded %d account(s)", len(accounts))

	return nil
}
