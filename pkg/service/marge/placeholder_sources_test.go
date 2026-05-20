package marge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestEnsurePlaceholderSources_SeedsSpotifyConnect verifies that the
// SPOTIFY/SpotifyConnectUserName placeholder is created for a fresh device
// and that subsequent calls are no-ops (idempotent).
func TestEnsurePlaceholderSources_SeedsSpotifyConnect(t *testing.T) {
	ds, account, device := newMargeTestDatastore(t)

	if err := EnsurePlaceholderSources(ds, account, device); err != nil {
		t.Fatalf("EnsurePlaceholderSources: %v", err)
	}

	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("GetConfiguredSources: %v", err)
	}

	if !hasSourceWithKey(sources, "SPOTIFY", PlaceholderSpotifyConnectAccount) {
		t.Fatalf("expected SPOTIFY/%s placeholder, got %d sources: %+v",
			PlaceholderSpotifyConnectAccount, len(sources), sourceSummary(sources))
	}

	// Find it and assert the cosmetic fields look right (UNAVAILABLE, no creds).
	var placeholder *models.ConfiguredSource

	for i := range sources {
		if sources[i].SourceKey.Type == "SPOTIFY" && sources[i].SourceKey.Account == PlaceholderSpotifyConnectAccount {
			placeholder = &sources[i]
			break
		}
	}

	if placeholder == nil {
		t.Fatalf("could not locate placeholder source after seeding")
	}

	// No credentials — placeholders are pure match anchors. (Status carries
	// xml:"-" so it doesn't round-trip through XML storage; not asserted.)
	if placeholder.Secret != "" || placeholder.Credential.Value != "" {
		t.Errorf("placeholder should carry no credential, got Secret=%q credential=%q",
			placeholder.Secret, placeholder.Credential.Value)
	}

	if placeholder.SourceProviderID != "15" {
		t.Errorf("placeholder SourceProviderID = %q, want 15", placeholder.SourceProviderID)
	}

	// Idempotency: second call must not add a duplicate.
	if err := EnsurePlaceholderSources(ds, account, device); err != nil {
		t.Fatalf("EnsurePlaceholderSources (second call): %v", err)
	}

	after, _ := ds.GetConfiguredSources(account, device)

	connectCount := 0
	for i := range after {
		if after[i].SourceKey.Type == "SPOTIFY" && after[i].SourceKey.Account == PlaceholderSpotifyConnectAccount {
			connectCount++
		}
	}

	if connectCount != 1 {
		t.Errorf("expected exactly 1 SPOTIFY/%s placeholder after second call, got %d",
			PlaceholderSpotifyConnectAccount, connectCount)
	}
}

// TestEnsurePlaceholderSources_CoexistsWithOAuth verifies that seeding the
// placeholder does not collide with or overwrite an existing OAuth-brokered
// SPOTIFY source under the same account.
func TestEnsurePlaceholderSources_CoexistsWithOAuth(t *testing.T) {
	ds, account, device := newMargeTestDatastore(t)

	// Pre-populate the OAuth-brokered Spotify source (same as bridgeSpotifyToMarge would).
	if _, err := AddSource(ds, account, "gesellix", "15", "bs-deadbeef", "token_version_3", "Gesell IX"); err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	if err := EnsurePlaceholderSources(ds, account, device); err != nil {
		t.Fatalf("EnsurePlaceholderSources: %v", err)
	}

	sources, _ := ds.GetConfiguredSources(account, device)

	// Both must be present.
	if !hasSourceWithKey(sources, "SPOTIFY", "gesellix") {
		t.Errorf("OAuth source SPOTIFY/gesellix missing after placeholder seeding")
	}

	if !hasSourceWithKey(sources, "SPOTIFY", PlaceholderSpotifyConnectAccount) {
		t.Errorf("placeholder source SPOTIFY/%s missing after seeding", PlaceholderSpotifyConnectAccount)
	}
}

// TestUpdatePreset_BindsToPlaceholderByExactAccount is the regression that
// motivated this whole change: when the speaker sends storePreset with
// source=SPOTIFY sourceAccount=SpotifyConnectUserName, marge must bind the
// preset to the placeholder, NOT the OAuth-brokered "gesellix" entry which
// the old type-only fallback would have picked.
func TestUpdatePreset_BindsToPlaceholderByExactAccount(t *testing.T) {
	ds, account, device := newMargeTestDatastore(t)

	// OAuth-brokered Spotify (the speaker should NOT bind to this for a
	// Connect-initiated preset).
	if _, err := AddSource(ds, account, "gesellix", "15", "bs-deadbeef", "token_version_3", "Gesell IX"); err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	// Placeholder for Spotify Connect.
	if err := EnsurePlaceholderSources(ds, account, device); err != nil {
		t.Fatalf("EnsurePlaceholderSources: %v", err)
	}

	// Look up the IDs of the two SPOTIFY sources so we can match on them
	// (the rendered preset XML doesn't echo Username back, so checking the
	// embedded <source id=...> attribute is the reliable signal).
	sources, _ := ds.GetConfiguredSources(account, device)

	placeholderID := findSourceID(sources, "SPOTIFY", PlaceholderSpotifyConnectAccount)
	oauthID := findSourceID(sources, "SPOTIFY", "gesellix")

	if placeholderID == "" || oauthID == "" || placeholderID == oauthID {
		t.Fatalf("test setup wrong: placeholderID=%q oauthID=%q", placeholderID, oauthID)
	}

	// Speaker payload modeled on what rhino sent: source/sourceAccount carry
	// the firmware-internal slug, sourceid is absent (the speaker doesn't
	// know our SRC_ IDs at storePreset time for a Connect-managed session).
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<preset>
  <name>Sand Castle Tapes</name>
  <source>SPOTIFY</source>
  <sourceaccount>SpotifyConnectUserName</sourceaccount>
  <location>/playback/container/c3BvdGlmeTphbGJ1bTo3M1I2YXlQS2VWWEV2SnR4UTR4SDNv</location>
  <contentItemType>tracklisturl</contentItemType>
  <containerArt>https://i.scdn.co/image/ab67616d0000b273</containerArt>
</preset>`)

	resp, err := UpdatePreset(ds, account, device, 1, body)
	if err != nil {
		t.Fatalf("UpdatePreset failed: %v", err)
	}

	xmlStr := string(resp)

	if !strings.Contains(xmlStr, `id="`+placeholderID+`"`) {
		t.Errorf("preset bound to wrong source: expected embedded <source id=%q>, got:\n%s", placeholderID, xmlStr)
	}

	if strings.Contains(xmlStr, `id="`+oauthID+`"`) {
		t.Errorf("preset unexpectedly bound to OAuth source <id=%q>; should have hit placeholder.\nResponse: %s", oauthID, xmlStr)
	}
}

// TestUpdatePreset_FallsBackToOAuthWhenNoPlaceholderMatch verifies the
// existing type-only fallback still works for presets whose sourceAccount
// matches the OAuth user (or is absent), so OAuth-driven presets keep
// binding correctly.
func TestUpdatePreset_FallsBackToOAuthWhenNoPlaceholderMatch(t *testing.T) {
	ds, account, device := newMargeTestDatastore(t)

	if _, err := AddSource(ds, account, "gesellix", "15", "bs-deadbeef", "token_version_3", "Gesell IX"); err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	if err := EnsurePlaceholderSources(ds, account, device); err != nil {
		t.Fatalf("EnsurePlaceholderSources: %v", err)
	}

	sources, _ := ds.GetConfiguredSources(account, device)

	placeholderID := findSourceID(sources, "SPOTIFY", PlaceholderSpotifyConnectAccount)
	oauthID := findSourceID(sources, "SPOTIFY", "gesellix")

	if placeholderID == "" || oauthID == "" || placeholderID == oauthID {
		t.Fatalf("test setup wrong: placeholderID=%q oauthID=%q", placeholderID, oauthID)
	}

	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<preset>
  <name>OAuth Album</name>
  <source>SPOTIFY</source>
  <sourceaccount>gesellix</sourceaccount>
  <location>/playback/container/c3BvdGlmeTphbGJ1bTpvYXV0aA==</location>
  <contentItemType>tracklisturl</contentItemType>
</preset>`)

	resp, err := UpdatePreset(ds, account, device, 2, body)
	if err != nil {
		t.Fatalf("UpdatePreset failed: %v", err)
	}

	xmlStr := string(resp)

	if !strings.Contains(xmlStr, `id="`+oauthID+`"`) {
		t.Errorf("preset did not bind to OAuth source <id=%q>; got:\n%s", oauthID, xmlStr)
	}

	if strings.Contains(xmlStr, `id="`+placeholderID+`"`) {
		t.Errorf("preset unexpectedly bound to placeholder <id=%q>; OAuth preset collapsed wrong direction.\nResponse: %s", placeholderID, xmlStr)
	}
}

func findSourceID(sources []models.ConfiguredSource, sourceKeyType, sourceKeyAccount string) string {
	for i := range sources {
		if (sources[i].SourceKey.Type == sourceKeyType && sources[i].SourceKey.Account == sourceKeyAccount) ||
			(sources[i].SourceKeyType == sourceKeyType && sources[i].SourceKeyAccount == sourceKeyAccount) {
			return sources[i].ID
		}
	}

	return ""
}

func newMargeTestDatastore(t *testing.T) (*datastore.DataStore, string, string) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "marge-placeholder-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)

	const account = "1234567"

	const device = "DEVPLACE"

	info := &models.ServiceDeviceInfo{
		DeviceID:  device,
		AccountID: account,
		Name:      "Test Speaker",
	}
	if err := ds.SaveDeviceInfo(account, device, info); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	// marge.AddSource walks accounts/{account}/devices and needs the
	// per-device dir present to write configuredsources.xml.
	if err := os.MkdirAll(filepath.Join(ds.AccountDevicesDir(account), device), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	return ds, account, device
}

func sourceSummary(sources []models.ConfiguredSource) []string {
	out := make([]string, 0, len(sources))
	for i := range sources {
		out = append(out, sources[i].SourceKey.Type+"/"+sources[i].SourceKey.Account+"#"+sources[i].ID)
	}

	return out
}
