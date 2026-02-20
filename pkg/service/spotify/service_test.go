package spotify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildAuthorizeURL(t *testing.T) {
	svc := NewSpotifyService("test-client-id", "test-secret", "http://localhost/callback", t.TempDir())

	url := svc.BuildAuthorizeURL()

	if !strings.Contains(url, "client_id=test-client-id") {
		t.Errorf("URL should contain client_id, got: %s", url)
	}
	if !strings.Contains(url, "redirect_uri=") {
		t.Errorf("URL should contain redirect_uri, got: %s", url)
	}
	if !strings.Contains(url, "scope=") {
		t.Errorf("URL should contain scope, got: %s", url)
	}
	if !strings.Contains(url, "response_type=code") {
		t.Errorf("URL should contain response_type=code, got: %s", url)
	}
	if !strings.HasPrefix(url, SpotifyAuthorizeURL) {
		t.Errorf("URL should start with %s, got: %s", SpotifyAuthorizeURL, url)
	}
}

func TestGetAccountsStripsTokens(t *testing.T) {
	svc := NewSpotifyService("cid", "csecret", "http://localhost/cb", t.TempDir())

	// Manually add an account with tokens
	svc.mu.Lock()
	svc.accounts["user1"] = &Account{
		UserID:       "user1",
		DisplayName:  "Test User",
		Email:        "test@example.com",
		AccessToken:  "secret-access-token",
		RefreshToken: "secret-refresh-token",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Unix(),
	}
	svc.mu.Unlock()

	accounts := svc.GetAccounts()

	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}

	if accounts[0].AccessToken != "" {
		t.Errorf("AccessToken should be stripped, got: %s", accounts[0].AccessToken)
	}
	if accounts[0].RefreshToken != "" {
		t.Errorf("RefreshToken should be stripped, got: %s", accounts[0].RefreshToken)
	}
	if accounts[0].UserID != "user1" {
		t.Errorf("UserID should be preserved, got: %s", accounts[0].UserID)
	}
	if accounts[0].DisplayName != "Test User" {
		t.Errorf("DisplayName should be preserved, got: %s", accounts[0].DisplayName)
	}
}

func TestGetFreshTokenRefreshesExpired(t *testing.T) {
	// Set up a mock Spotify token endpoint
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("expected grant_type=refresh_token, got %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("refresh_token") != "my-refresh-token" {
			t.Errorf("expected refresh_token=my-refresh-token, got %s", r.Form.Get("refresh_token"))
		}

		// Verify Basic Auth
		user, pass, ok := r.BasicAuth()
		if !ok || user != "cid" || pass != "csecret" {
			t.Errorf("expected Basic Auth cid:csecret, got %s:%s (ok=%v)", user, pass, ok)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-access-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "new-refresh-token",
		})
	}))
	defer tokenServer.Close()

	svc := NewSpotifyService("cid", "csecret", "http://localhost/cb", t.TempDir())

	// Override the token URL for testing
	svc.tokenURL = tokenServer.URL

	// Add an account with an expired token
	svc.mu.Lock()
	svc.accounts["user1"] = &Account{
		UserID:       "user1",
		DisplayName:  "Test User",
		AccessToken:  "old-expired-token",
		RefreshToken: "my-refresh-token",
		ExpiresAt:    time.Now().Add(-1 * time.Hour).Unix(), // expired
	}
	svc.mu.Unlock()

	accessToken, username, err := svc.GetFreshToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if accessToken != "new-access-token" {
		t.Errorf("expected new-access-token, got %s", accessToken)
	}
	if username != "user1" {
		t.Errorf("expected user1, got %s", username)
	}

	// Verify the account was updated
	svc.mu.RLock()
	account := svc.accounts["user1"]
	svc.mu.RUnlock()

	if account.RefreshToken != "new-refresh-token" {
		t.Errorf("refresh token should be updated, got %s", account.RefreshToken)
	}
}

func TestResolveEntityParsesURI(t *testing.T) {
	tests := []struct {
		uri          string
		expectedType string
		expectedID   string
		shouldErr    bool
	}{
		{"spotify:track:abc123", "tracks", "abc123", false},
		{"spotify:album:xyz789", "albums", "xyz789", false},
		{"spotify:playlist:pl1", "playlists", "pl1", false},
		{"spotify:artist:ar1", "artists", "ar1", false},
		{"invalid-uri", "", "", true},
		{"spotify:invalid_type:id", "", "", true},
		{"spotify:track", "", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.uri, func(t *testing.T) {
			entityType, entityID, err := parseSpotifyURI(tc.uri)
			if tc.shouldErr {
				if err == nil {
					t.Errorf("expected error for URI %s", tc.uri)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for URI %s: %v", tc.uri, err)
			}
			if entityType != tc.expectedType {
				t.Errorf("expected type %s, got %s", tc.expectedType, entityType)
			}
			if entityID != tc.expectedID {
				t.Errorf("expected id %s, got %s", tc.expectedID, entityID)
			}
		})
	}
}

func TestResolveEntityFetchesFromAPI(t *testing.T) {
	// Mock Spotify API
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer fresh-token" {
			t.Errorf("expected Bearer fresh-token, got %s", auth)
		}

		switch r.URL.Path {
		case "/tracks/abc123":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "Test Track",
				"album": map[string]interface{}{
					"images": []map[string]interface{}{
						{"url": "http://img.example.com/track.jpg"},
					},
				},
			})
		case "/albums/xyz789":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "Test Album",
				"images": []map[string]interface{}{
					{"url": "http://img.example.com/album.jpg"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()

	svc := NewSpotifyService("cid", "csecret", "http://localhost/cb", t.TempDir())
	svc.apiBase = apiServer.URL

	// Add a non-expired account
	svc.mu.Lock()
	svc.accounts["user1"] = &Account{
		UserID:       "user1",
		AccessToken:  "fresh-token",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Unix(),
	}
	svc.mu.Unlock()

	// Test track (images come from album)
	name, imageURL, err := svc.ResolveEntity("spotify:track:abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "Test Track" {
		t.Errorf("expected Test Track, got %s", name)
	}
	if imageURL != "http://img.example.com/track.jpg" {
		t.Errorf("expected track image URL, got %s", imageURL)
	}

	// Test album (images at top level)
	name, imageURL, err = svc.ResolveEntity("spotify:album:xyz789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "Test Album" {
		t.Errorf("expected Test Album, got %s", name)
	}
	if imageURL != "http://img.example.com/album.jpg" {
		t.Errorf("expected album image URL, got %s", imageURL)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	// Create and populate
	svc := NewSpotifyService("cid", "csecret", "http://localhost/cb", dir)
	svc.mu.Lock()
	svc.accounts["user1"] = &Account{
		UserID:       "user1",
		DisplayName:  "Test User",
		Email:        "test@example.com",
		AccessToken:  "at",
		RefreshToken: "rt",
		ExpiresAt:    1234567890,
	}
	svc.accounts["user2"] = &Account{
		UserID:       "user2",
		DisplayName:  "User Two",
		Email:        "two@example.com",
		AccessToken:  "at2",
		RefreshToken: "rt2",
		ExpiresAt:    9876543210,
	}
	svc.mu.Unlock()

	// Save
	if err := svc.save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	accountsFile := filepath.Join(dir, "spotify", "accounts.json")
	if _, err := os.Stat(accountsFile); os.IsNotExist(err) {
		t.Fatal("accounts.json was not created")
	}

	// Load into new service
	svc2 := NewSpotifyService("cid", "csecret", "http://localhost/cb", dir)

	svc2.mu.RLock()
	defer svc2.mu.RUnlock()

	if len(svc2.accounts) != 2 {
		t.Fatalf("expected 2 accounts after load, got %d", len(svc2.accounts))
	}

	u1, ok := svc2.accounts["user1"]
	if !ok {
		t.Fatal("user1 not found after load")
	}
	if u1.DisplayName != "Test User" {
		t.Errorf("expected Test User, got %s", u1.DisplayName)
	}
	if u1.AccessToken != "at" {
		t.Errorf("expected at, got %s", u1.AccessToken)
	}
	if u1.ExpiresAt != 1234567890 {
		t.Errorf("expected ExpiresAt 1234567890, got %d", u1.ExpiresAt)
	}
}

func TestExchangeCodeAndStore(t *testing.T) {
	// Mock token endpoint
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}

		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "test-auth-code" {
				t.Errorf("expected code=test-auth-code, got %s", r.Form.Get("code"))
			}
			user, pass, ok := r.BasicAuth()
			if !ok || user != "cid" || pass != "csecret" {
				t.Errorf("bad Basic Auth: %s:%s ok=%v", user, pass, ok)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  "new-at",
				"refresh_token": "new-rt",
				"expires_in":    3600,
			})
		default:
			t.Errorf("unexpected grant_type: %s", r.Form.Get("grant_type"))
			http.Error(w, "bad request", 400)
		}
	}))
	defer tokenServer.Close()

	// Mock profile endpoint
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer new-at" {
			t.Errorf("expected Bearer new-at, got %s", auth)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           "spotify-user-123",
			"display_name": "Spotify User",
			"email":        "user@spotify.com",
		})
	}))
	defer profileServer.Close()

	dir := t.TempDir()
	svc := NewSpotifyService("cid", "csecret", "http://localhost/cb", dir)
	svc.tokenURL = tokenServer.URL
	svc.apiBase = profileServer.URL

	err := svc.ExchangeCodeAndStore("test-auth-code")
	if err != nil {
		t.Fatalf("ExchangeCodeAndStore failed: %v", err)
	}

	// Verify account stored
	svc.mu.RLock()
	account, ok := svc.accounts["spotify-user-123"]
	svc.mu.RUnlock()

	if !ok {
		t.Fatal("account not found after exchange")
	}
	if account.DisplayName != "Spotify User" {
		t.Errorf("expected Spotify User, got %s", account.DisplayName)
	}
	if account.Email != "user@spotify.com" {
		t.Errorf("expected user@spotify.com, got %s", account.Email)
	}
	if account.AccessToken != "new-at" {
		t.Errorf("expected new-at, got %s", account.AccessToken)
	}
	if account.RefreshToken != "new-rt" {
		t.Errorf("expected new-rt, got %s", account.RefreshToken)
	}

	// Verify saved to disk
	accountsFile := filepath.Join(dir, "spotify", "accounts.json")
	data, err := os.ReadFile(accountsFile)
	if err != nil {
		t.Fatalf("failed to read accounts file: %v", err)
	}
	if !strings.Contains(string(data), "spotify-user-123") {
		t.Error("accounts file should contain the user ID")
	}
}

func TestGetFreshTokenNoAccounts(t *testing.T) {
	svc := NewSpotifyService("cid", "csecret", "http://localhost/cb", t.TempDir())

	_, _, err := svc.GetFreshToken()
	if err == nil {
		t.Error("expected error when no accounts exist")
	}
}

func TestGetFreshTokenNotExpired(t *testing.T) {
	svc := NewSpotifyService("cid", "csecret", "http://localhost/cb", t.TempDir())

	svc.mu.Lock()
	svc.accounts["user1"] = &Account{
		UserID:       "user1",
		AccessToken:  "valid-token",
		RefreshToken: "rt",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Unix(),
	}
	svc.mu.Unlock()

	token, username, err := svc.GetFreshToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "valid-token" {
		t.Errorf("expected valid-token, got %s", token)
	}
	if username != "user1" {
		t.Errorf("expected user1, got %s", username)
	}
}
