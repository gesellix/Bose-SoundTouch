package handlers

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/gesellix/bose-soundtouch/pkg/service/spotify"
)

// TestPrimeDeviceWithSpotify_RegistersMargeSource is a regression test for the
// "AddPreset - failed due to invalid SourceID" failure observed when storing a
// Spotify preset on a primed device. The watchdog priming path used to push
// ZeroConf credentials without writing a SPOTIFY ConfiguredSource into the
// marge datastore — so marge.UpdatePreset later had nothing to match
// SourceID="SPOTIFY" against and rejected the storePreset request.
//
// This test verifies that PrimeDeviceWithSpotify now also calls marge.AddSource
// for the device's account, producing a ConfiguredSource with
// SourceProviderID="15" (constants.SpotifyProviderID).
func TestPrimeDeviceWithSpotify_RegistersMargeSource(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	server := NewServer(ds, nil, "http://localhost", false, false, false)

	// Fake speaker that accepts the ZeroConf push via the simplified
	// (non-DH) fallback AND records whether /notification (sourcesUpdated)
	// was hit.
	var notified atomic.Bool

	speakerTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/notification" {
			notified.Store(true)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8" ?><status>/notification</status>`)

			return
		}

		switch r.URL.Query().Get("action") {
		case "getInfo":
			http.Error(w, "not supported", http.StatusNotFound)
		case "addUser":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer speakerTS.Close()

	speakerHostPort := strings.TrimPrefix(speakerTS.URL, "http://")

	speakerHost, _, err := net.SplitHostPort(speakerHostPort)
	if err != nil {
		t.Fatalf("split speaker URL: %v", err)
	}

	// Register the device under a real account so the IP→account lookup succeeds.
	const accountID = "acc-prime"
	const deviceID = "DEVPRIME"
	devInfo := &models.ServiceDeviceInfo{
		DeviceID:  deviceID,
		AccountID: accountID,
		Name:      "Test Speaker",
		IPAddress: speakerHost,
	}

	if err := ds.SaveDeviceInfo(accountID, deviceID, devInfo); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	// marge.AddSource walks the account/devices dir — make sure the per-device
	// subdir exists so the source actually gets persisted.
	if err := os.MkdirAll(filepath.Join(ds.AccountDevicesDir(accountID), deviceID), 0o755); err != nil {
		t.Fatalf("MkdirAll device dir: %v", err)
	}

	// Pre-seed a linked Spotify account so PrimeDeviceWithSpotify has something
	// to push. The token is valid for an hour so GetFreshToken won't try to
	// refresh against a live endpoint. We point the token endpoint at a noop
	// URL just in case, so a stray refresh would fail loudly rather than fan
	// out to the internet.
	spotifyDir := filepath.Join(tmpDir, "spotify")
	if err := os.MkdirAll(spotifyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll spotify dir: %v", err)
	}

	accountsPayload := map[string]map[string]any{
		"spotify-user": {
			"user_id":       "spotify-user",
			"display_name":  "Spotify User",
			"email":         "user@example.com",
			"access_token":  "fresh-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    time.Now().Add(time.Hour).Unix(),
			"bose_secret":   "bs-deadbeef",
		},
	}
	accountsJSON, err := json.Marshal(accountsPayload)
	if err != nil {
		t.Fatalf("marshal accounts: %v", err)
	}

	if err := os.WriteFile(filepath.Join(spotifyDir, "accounts.json"), accountsJSON, 0o600); err != nil {
		t.Fatalf("write accounts.json: %v", err)
	}

	ss := spotify.NewSpotifyService("client-id", "client-secret", "http://localhost/callback", tmpDir)
	// Unused fallback token endpoint — defensive in case the test ever drifts
	// to an expired token.
	ss.SetEndpoints("http://127.0.0.1:1/token", "http://127.0.0.1:1")

	if err := ss.Load(); err != nil {
		t.Fatalf("Load spotify accounts: %v", err)
	}

	if len(ss.GetAccounts()) != 1 {
		t.Fatalf("expected 1 spotify account after Load, got %d", len(ss.GetAccounts()))
	}

	server.SetSpotifyService(ss)

	// Sanity: no SPOTIFY source registered yet.
	sources, _ := ds.GetConfiguredSources(accountID, deviceID)
	if hasSpotifySource(sources) {
		t.Fatalf("precondition failed: SPOTIFY source already present before priming")
	}

	// Pass host:port so the ZeroConf push hits our test server instead of the
	// hard-coded :8200 fallback. The IP→account lookup strips the port before
	// matching against devInfo.IPAddress.
	server.PrimeDeviceWithSpotify(speakerHostPort)

	sources, err = ds.GetConfiguredSources(accountID, deviceID)
	if err != nil {
		t.Fatalf("GetConfiguredSources after priming: %v", err)
	}

	if !hasSpotifySource(sources) {
		for _, src := range sources {
			t.Logf("source after priming: ID=%s providerID=%s keyType=%s account=%s", src.ID, src.SourceProviderID, src.SourceKey.Type, src.SourceKey.Account)
		}

		t.Fatalf("expected a SPOTIFY ConfiguredSource (providerID=%d) after priming", constants.SpotifyProviderID)
	}

	// The speaker's on-device Sources.xml only refreshes when we tell it to —
	// without this notification storePreset keeps failing even though marge
	// already has the SPOTIFY source.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && !notified.Load() {
		time.Sleep(20 * time.Millisecond)
	}

	if !notified.Load() {
		t.Errorf("speaker did not receive a sourcesUpdated /notification after priming")
	}
}

// TestPrimeDeviceWithSpotify_SkipsWhenDeviceUnmapped ensures that priming a
// device whose IP is not associated with any account does NOT fabricate a
// source under the "default" account — the previous behavior would silently
// pollute marge with sources for devices that never asked.
func TestPrimeDeviceWithSpotify_SkipsWhenDeviceUnmapped(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	server := NewServer(ds, nil, "http://localhost", false, false, false)

	speakerTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "getInfo":
			http.Error(w, "not supported", http.StatusNotFound)
		case "addUser":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer speakerTS.Close()

	speakerURL, _ := url.Parse(speakerTS.URL)
	speakerHostPort := speakerURL.Host

	// Pre-seed a Spotify account but do NOT register any device.
	spotifyDir := filepath.Join(tmpDir, "spotify")
	_ = os.MkdirAll(spotifyDir, 0o755)
	accountsPayload := map[string]map[string]any{
		"spotify-user": {
			"user_id":      "spotify-user",
			"display_name": "Spotify User",
			"access_token": "fresh-access-token",
			"expires_at":   time.Now().Add(time.Hour).Unix(),
			"bose_secret":  "bs-deadbeef",
		},
	}

	accountsJSON, err := json.Marshal(accountsPayload)
	if err != nil {
		t.Fatalf("marshal accounts: %v", err)
	}

	_ = os.WriteFile(filepath.Join(spotifyDir, "accounts.json"), accountsJSON, 0o600)

	ss := spotify.NewSpotifyService("client-id", "client-secret", "http://localhost/callback", tmpDir)
	ss.SetEndpoints("http://127.0.0.1:1/token", "http://127.0.0.1:1")

	if err := ss.Load(); err != nil {
		t.Fatalf("Load spotify accounts: %v", err)
	}

	server.SetSpotifyService(ss)

	server.PrimeDeviceWithSpotify(speakerHostPort)

	// "default" account should have no SPOTIFY source added by us.
	sources, _ := ds.GetConfiguredSources("default", "")
	if hasSpotifySource(sources) {
		t.Errorf("priming an unmapped device wrote a SPOTIFY source under 'default' — should have been skipped")
	}
}

// TestPrimeDeviceWithSpotify_LiveMargeAccountUUIDWins covers the production
// scenario the previous test didn't catch: a device whose datastore
// ServiceDeviceInfo.AccountID is "default" (or stale) but whose live
// :8090/info reports a real paired margeAccountUUID. The SPOTIFY source must
// land under the paired account — that's the account marge.UpdatePreset
// receives storePreset under, so writing anywhere else means the preset still
// fails with "AddPreset - failed due to invalid SourceID".
//
// Mirrors setup.populateDeviceInfo's resolution order (datastore ← live /info)
// rather than guessing.
func TestPrimeDeviceWithSpotify_LiveMargeAccountUUIDWins(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	server := NewServer(ds, nil, "http://localhost", false, false, false)

	const (
		datastoreAccount = "default" // stale / fallback
		pairedAccount    = "1111111" // live margeAccountUUID from /info
		deviceID         = "DEVPAIR"
	)

	// Fake speaker that serves both /info and the ZeroConf /zc.
	var speakerHost string

	speakerTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8" ?>`+
				`<info deviceID="`+deviceID+`">`+
				`<name>Paired Speaker</name><type>SoundTouch 20</type>`+
				`<margeAccountUUID>`+pairedAccount+`</margeAccountUUID>`+
				`</info>`)
		case r.URL.Path == "/notification":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8" ?><status>/notification</status>`)
		default:
			switch r.URL.Query().Get("action") {
			case "getInfo":
				http.Error(w, "not supported", http.StatusNotFound)
			case "addUser":
				w.WriteHeader(http.StatusOK)
			default:
				http.NotFound(w, r)
			}
		}
	}))
	defer speakerTS.Close()

	speakerHostPort := strings.TrimPrefix(speakerTS.URL, "http://")
	speakerHost, _, _ = net.SplitHostPort(speakerHostPort)

	// Register the device under the STALE account so the datastore lookup
	// would yield the wrong answer if used in isolation.
	devInfo := &models.ServiceDeviceInfo{
		DeviceID:  deviceID,
		AccountID: datastoreAccount,
		Name:      "Paired Speaker",
		IPAddress: speakerHost,
	}
	if err := ds.SaveDeviceInfo(datastoreAccount, deviceID, devInfo); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	// And make sure the paired account's device dir exists so
	// marge.AddSource can persist the source (it walks accounts/devices/...).
	if err := os.MkdirAll(filepath.Join(ds.AccountDevicesDir(pairedAccount), deviceID), 0o755); err != nil {
		t.Fatalf("MkdirAll paired dir: %v", err)
	}

	// Pre-seed a Spotify account so priming has something to push.
	spotifyDir := filepath.Join(tmpDir, "spotify")
	_ = os.MkdirAll(spotifyDir, 0o755)
	accountsPayload := map[string]map[string]any{
		"spotify-user": {
			"user_id":      "spotify-user",
			"display_name": "Spotify User",
			"access_token": "fresh-access-token",
			"expires_at":   time.Now().Add(time.Hour).Unix(),
			"bose_secret":  "bs-deadbeef",
		},
	}

	accountsJSON, err := json.Marshal(accountsPayload)
	if err != nil {
		t.Fatalf("marshal accounts: %v", err)
	}

	_ = os.WriteFile(filepath.Join(spotifyDir, "accounts.json"), accountsJSON, 0o600)

	ss := spotify.NewSpotifyService("client-id", "client-secret", "http://localhost/callback", tmpDir)
	ss.SetEndpoints("http://127.0.0.1:1/token", "http://127.0.0.1:1")

	if err := ss.Load(); err != nil {
		t.Fatalf("Load spotify accounts: %v", err)
	}

	server.SetSpotifyService(ss)

	// Wire a real setup.Manager so resolvePairedAccount reaches /info.
	// HTTPGet uses the default net/http client, which hits the httptest
	// server directly via deviceIP=host:port.
	server.sm = setup.NewManager("http://localhost", ds, nil)

	server.PrimeDeviceWithSpotify(speakerHostPort)

	// SPOTIFY source must be under the PAIRED account, not the datastore one.
	pairedSources, err := ds.GetConfiguredSources(pairedAccount, deviceID)
	if err != nil {
		t.Fatalf("GetConfiguredSources(paired): %v", err)
	}

	if !hasSpotifySource(pairedSources) {
		t.Errorf("expected SPOTIFY source under paired account %s, got %d sources", pairedAccount, len(pairedSources))
	}

	// And it must NOT have been written under the stale datastore account.
	staleSources, _ := ds.GetConfiguredSources(datastoreAccount, deviceID)
	if hasSpotifySource(staleSources) {
		t.Errorf("SPOTIFY source unexpectedly written under stale datastore account %s — should follow live margeAccountUUID", datastoreAccount)
	}
}

func hasSpotifySource(sources []models.ConfiguredSource) bool {
	for _, src := range sources {
		if src.SourceProviderID == "15" || src.SourceKey.Type == constants.ProviderSpotify {
			return true
		}
	}

	return false
}
