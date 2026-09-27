package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestPlayerEnabledDefaultsToTrue covers the upgrade-safety requirement of
// issue 762: an install that never touched the setting -- fresh datastore,
// no settings.json entry, Server constructed with its normal defaults --
// must report the player as enabled.
func TestPlayerEnabledDefaultsToTrue(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	_, server := setupRouter("http://127.0.0.1:8000", ds)

	if !server.PlayerEnabled() {
		t.Fatalf("PlayerEnabled() = false, want true (default)")
	}
}

// TestPlayerEnabledSettingRoundTrip mirrors TestCatalogSizeSettingRoundTrip:
// an omitted field must preserve whatever is stored (issue #589's shape),
// and an explicit true/false must both persist and take effect live.
func TestPlayerEnabledSettingRoundTrip(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	r, server := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)

	defer ts.Close()

	if status := postSettings(t, ts, map[string]any{
		"server_url": "http://127.0.0.1:8000", "player_enabled": false,
	}); status != http.StatusOK {
		t.Fatalf("expected OK, got %d", status)
	}

	persisted, err := ds.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}

	if persisted.PlayerEnabled == nil || *persisted.PlayerEnabled {
		t.Fatalf("PlayerEnabled = %v, want false", persisted.PlayerEnabled)
	}

	if server.PlayerEnabled() {
		t.Fatalf("server.PlayerEnabled() = true, want the live flag to follow the save")
	}

	// A form that does not know about the field (or a save that doesn't
	// touch it) must not reset it -- the same rule TLSExtraHosts/CatalogSize
	// follow, and important here because it would silently re-enable the
	// player's background device polling.
	if status := postSettings(t, ts, map[string]any{
		"server_url": "http://127.0.0.1:8000",
	}); status != http.StatusOK {
		t.Fatalf("expected OK, got %d", status)
	}

	persisted, _ = ds.GetSettings()
	if persisted.PlayerEnabled == nil || *persisted.PlayerEnabled {
		t.Fatalf("an omitted player_enabled reset the stored value: %v", persisted.PlayerEnabled)
	}

	if server.PlayerEnabled() {
		t.Fatalf("server.PlayerEnabled() = true, want it to stay false after an unrelated save")
	}

	if status := postSettings(t, ts, map[string]any{
		"server_url": "http://127.0.0.1:8000", "player_enabled": true,
	}); status != http.StatusOK {
		t.Fatalf("expected OK, got %d", status)
	}

	persisted, _ = ds.GetSettings()
	if persisted.PlayerEnabled == nil || !*persisted.PlayerEnabled {
		t.Fatalf("PlayerEnabled = %v, want true", persisted.PlayerEnabled)
	}

	if !server.PlayerEnabled() {
		t.Fatalf("server.PlayerEnabled() = false, want the live flag to follow the save")
	}
}

// TestPlayerEnabledChangedHookFiresOnlyOnChange covers the live-toggle wiring
// the embedded build relies on to stop/start the player's background device
// polling without a restart: the hook must fire exactly once per actual
// value change, and not at all when a settings save leaves it untouched.
func TestPlayerEnabledChangedHookFiresOnlyOnChange(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	r, server := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)

	defer ts.Close()

	var got []bool
	server.SetPlayerEnabledChangedHook(func(enabled bool) {
		got = append(got, enabled)
	})

	// Same value as the default (true): must not fire.
	postSettings(t, ts, map[string]any{"server_url": "http://127.0.0.1:8000", "player_enabled": true})
	// Omitted: must not fire.
	postSettings(t, ts, map[string]any{"server_url": "http://127.0.0.1:8000"})
	// Actual change to false: must fire once.
	postSettings(t, ts, map[string]any{"server_url": "http://127.0.0.1:8000", "player_enabled": false})
	// Same value again (false): must not fire.
	postSettings(t, ts, map[string]any{"server_url": "http://127.0.0.1:8000", "player_enabled": false})
	// Actual change back to true: must fire once.
	postSettings(t, ts, map[string]any{"server_url": "http://127.0.0.1:8000", "player_enabled": true})

	want := []bool{false, true}
	if len(got) != len(want) {
		t.Fatalf("hook fired %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hook fired %v, want %v", got, want)
		}
	}
}

// TestHandleGetSettingsIncludesPlayerEnabled covers the read side: the
// Settings JSON must expose player_enabled so the admin UI can render it.
func TestHandleGetSettingsIncludesPlayerEnabled(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	r, _ := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)

	defer ts.Close()

	res, err := http.Get(ts.URL + "/setup/settings")
	if err != nil {
		t.Fatalf("GET /setup/settings: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	enabled, ok := body["player_enabled"].(bool)
	if !ok {
		t.Fatalf("player_enabled missing or not a bool: %#v", body["player_enabled"])
	}

	if !enabled {
		t.Fatalf("player_enabled = false, want true (default)")
	}
}

// TestPlayerGateMiddleware covers the router-level gate (issue 762): enabled
// passes the request through, disabled refuses it with a clear message
// instead of leaving the caller to whatever the router's default 404 looks
// like.
func TestPlayerGateMiddleware(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	server := NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	gated := server.PlayerGateMiddleware(next)

	t.Run("enabled passes through", func(t *testing.T) {
		called = false

		req := httptest.NewRequest(http.MethodGet, "/app", nil)
		rec := httptest.NewRecorder()
		gated.ServeHTTP(rec, req)

		if !called {
			t.Fatalf("next handler was not called while enabled")
		}

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("disabled refuses with a clear message", func(t *testing.T) {
		called = false
		server.SetPlayerEnabled(false)

		defer server.SetPlayerEnabled(true)

		req := httptest.NewRequest(http.MethodGet, "/api/control/devices", nil)
		rec := httptest.NewRecorder()
		gated.ServeHTTP(rec, req)

		if called {
			t.Fatalf("next handler was called while disabled")
		}

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}

		if got := rec.Body.String(); got != playerDisabledMessage+"\n" {
			t.Fatalf("body = %q, want %q", got, playerDisabledMessage+"\n")
		}
	})
}
