package amazon

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
	svc := NewAmazonService("test-client-id", "test-secret", "ueberboese-login://amazon", t.TempDir())

	state := "test-state"
	gotURL := svc.BuildAuthorizeURL(state)

	if !strings.Contains(gotURL, "client_id=test-client-id") {
		t.Errorf("URL should contain client_id, got: %s", gotURL)
	}
	if !strings.Contains(gotURL, "redirect_uri=") {
		t.Errorf("URL should contain redirect_uri, got: %s", gotURL)
	}
	if !strings.Contains(gotURL, "scope=") {
		t.Errorf("URL should contain scope, got: %s", gotURL)
	}
	if !strings.Contains(gotURL, "response_type=code") {
		t.Errorf("URL should contain response_type=code, got: %s", gotURL)
	}
	if !strings.Contains(gotURL, "state=test-state") {
		t.Errorf("URL should contain state=test-state, got: %s", gotURL)
	}
	if !strings.HasPrefix(gotURL, AmazonAuthorizeURL) {
		t.Errorf("URL should start with %s, got: %s", AmazonAuthorizeURL, gotURL)
	}
}

func TestGetAccountsStripsTokens(t *testing.T) {
	svc := NewAmazonService("cid", "csecret", "ueberboese-login://amazon", t.TempDir())

	svc.mu.Lock()
	svc.accounts["amzn1.account.EXAMPLE"] = &Account{
		UserID:       "amzn1.account.EXAMPLE",
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
	if accounts[0].UserID != "amzn1.account.EXAMPLE" {
		t.Errorf("UserID should be preserved, got: %s", accounts[0].UserID)
	}
	if accounts[0].DisplayName != "Test User" {
		t.Errorf("DisplayName should be preserved, got: %s", accounts[0].DisplayName)
	}
}

func TestExchangeCodeAndStore(t *testing.T) {
	// Mock token endpoint — Amazon uses POST body credentials, not Basic Auth.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}

		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "test-auth-code" {
				t.Errorf("expected code=test-auth-code, got %s", r.Form.Get("code"))
			}

			// Amazon uses POST body credentials, not HTTP Basic Auth.
			if r.Form.Get("client_id") != "cid" {
				t.Errorf("expected client_id=cid in POST body, got %q", r.Form.Get("client_id"))
			}
			if r.Form.Get("client_secret") != "csecret" {
				t.Errorf("expected client_secret=csecret in POST body, got %q", r.Form.Get("client_secret"))
			}
			_, _, hasBasicAuth := r.BasicAuth()
			if hasBasicAuth {
				t.Error("Amazon token endpoint must NOT use HTTP Basic Auth")
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

	// Mock profile endpoint — LWA returns "user_id" and "name" (not "id" / "display_name").
	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer new-at" {
			t.Errorf("expected Bearer new-at, got %s", auth)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user_id": "amzn1.account.TESTUSER123",
			"name":    "Amazon User",
			"email":   "user@amazon.com",
		})
	}))
	defer profileServer.Close()

	dir := t.TempDir()
	svc := NewAmazonService("cid", "csecret", "ueberboese-login://amazon", dir)
	svc.SetEndpoints(tokenServer.URL, profileServer.URL)

	err := svc.ExchangeCodeAndStore("test-auth-code")
	if err != nil {
		t.Fatalf("ExchangeCodeAndStore failed: %v", err)
	}

	svc.mu.RLock()
	account, ok := svc.accounts["amzn1.account.TESTUSER123"]
	svc.mu.RUnlock()

	if !ok {
		t.Fatal("account not found after exchange")
	}
	if account.DisplayName != "Amazon User" {
		t.Errorf("expected Amazon User, got %s", account.DisplayName)
	}
	if account.Email != "user@amazon.com" {
		t.Errorf("expected user@amazon.com, got %s", account.Email)
	}
	if account.AccessToken != "new-at" {
		t.Errorf("expected new-at, got %s", account.AccessToken)
	}
	if account.RefreshToken != "new-rt" {
		t.Errorf("expected new-rt, got %s", account.RefreshToken)
	}

	// Verify saved to disk under amazon/ (not spotify/).
	accountsFile := filepath.Join(dir, "amazon", "accounts.json")
	data, err := os.ReadFile(accountsFile)
	if err != nil {
		t.Fatalf("failed to read accounts file: %v", err)
	}
	if !strings.Contains(string(data), "amzn1.account.TESTUSER123") {
		t.Error("accounts file should contain the user ID")
	}
}

func TestRefreshAccessToken(t *testing.T) {
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

		// Amazon uses POST body credentials.
		if r.Form.Get("client_id") != "cid" {
			t.Errorf("expected client_id=cid in POST body, got %q", r.Form.Get("client_id"))
		}
		if r.Form.Get("client_secret") != "csecret" {
			t.Errorf("expected client_secret=csecret in POST body, got %q", r.Form.Get("client_secret"))
		}
		_, _, hasBasicAuth := r.BasicAuth()
		if hasBasicAuth {
			t.Error("Amazon token endpoint must NOT use HTTP Basic Auth")
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

	svc := NewAmazonService("cid", "csecret", "ueberboese-login://amazon", t.TempDir())
	svc.tokenURL = tokenServer.URL

	account := &Account{
		UserID:       "amzn1.account.USER",
		AccessToken:  "old-expired-token",
		RefreshToken: "my-refresh-token",
		ExpiresAt:    time.Now().Add(-1 * time.Hour).Unix(),
	}

	svc.mu.Lock()
	svc.accounts[account.UserID] = account
	svc.mu.Unlock()

	if err := svc.RefreshAccessToken(account); err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}

	if account.AccessToken != "new-access-token" {
		t.Errorf("expected new-access-token, got %s", account.AccessToken)
	}
	if account.RefreshToken != "new-refresh-token" {
		t.Errorf("expected new-refresh-token, got %s", account.RefreshToken)
	}
}

func TestGetFreshTokenRefreshesExpired(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
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

	svc := NewAmazonService("cid", "csecret", "ueberboese-login://amazon", t.TempDir())
	svc.tokenURL = tokenServer.URL

	svc.mu.Lock()
	svc.accounts["amzn1.account.USER"] = &Account{
		UserID:       "amzn1.account.USER",
		AccessToken:  "old-expired-token",
		RefreshToken: "my-refresh-token",
		ExpiresAt:    time.Now().Add(-1 * time.Hour).Unix(),
	}
	svc.mu.Unlock()

	accessToken, username, err := svc.GetFreshToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if accessToken != "new-access-token" {
		t.Errorf("expected new-access-token, got %s", accessToken)
	}
	if username != "amzn1.account.USER" {
		t.Errorf("expected amzn1.account.USER, got %s", username)
	}

	svc.mu.RLock()
	account := svc.accounts["amzn1.account.USER"]
	svc.mu.RUnlock()

	if account.RefreshToken != "new-refresh-token" {
		t.Errorf("refresh token should be updated, got %s", account.RefreshToken)
	}
}

func TestGetFreshTokenNoAccounts(t *testing.T) {
	svc := NewAmazonService("cid", "csecret", "ueberboese-login://amazon", t.TempDir())

	_, _, err := svc.GetFreshToken()
	if err == nil {
		t.Error("expected error when no accounts exist")
	}
}

func TestGetFreshTokenNotExpired(t *testing.T) {
	svc := NewAmazonService("cid", "csecret", "ueberboese-login://amazon", t.TempDir())

	svc.mu.Lock()
	svc.accounts["amzn1.account.USER"] = &Account{
		UserID:       "amzn1.account.USER",
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
	if username != "amzn1.account.USER" {
		t.Errorf("expected amzn1.account.USER, got %s", username)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	svc := NewAmazonService("cid", "csecret", "ueberboese-login://amazon", dir)
	svc.mu.Lock()
	svc.accounts["amzn1.account.USER1"] = &Account{
		UserID:       "amzn1.account.USER1",
		DisplayName:  "Test User",
		Email:        "test@example.com",
		AccessToken:  "at",
		RefreshToken: "rt",
		ExpiresAt:    1234567890,
	}
	svc.accounts["amzn1.account.USER2"] = &Account{
		UserID:       "amzn1.account.USER2",
		DisplayName:  "User Two",
		Email:        "two@example.com",
		AccessToken:  "at2",
		RefreshToken: "rt2",
		ExpiresAt:    9876543210,
	}
	svc.mu.Unlock()

	if err := svc.save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	accountsFile := filepath.Join(dir, "amazon", "accounts.json")
	if _, err := os.Stat(accountsFile); os.IsNotExist(err) {
		t.Fatal("amazon/accounts.json was not created")
	}

	svc2 := NewAmazonService("cid", "csecret", "ueberboese-login://amazon", dir)
	if err := svc2.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	svc2.mu.RLock()
	defer svc2.mu.RUnlock()

	if len(svc2.accounts) != 2 {
		t.Fatalf("expected 2 accounts after load, got %d", len(svc2.accounts))
	}

	u1, ok := svc2.accounts["amzn1.account.USER1"]
	if !ok {
		t.Fatal("USER1 not found after load")
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
