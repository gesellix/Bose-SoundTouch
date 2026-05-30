package marge

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// configuredFromSpeakerSources mirrors the HTTP /sources import branch of
// setup.(*Manager).syncSources (cmd path: SyncDeviceData -> syncSources,
// pkg/service/setup/setup.go). It is a verbatim field copy: the speaker's
// <sourceItem source="X"> becomes ConfiguredSource.SourceKey.Type=X with no
// ID, no protocol Type and no SourceProviderID. Replicated here (rather than
// imported) because the upstream is inline I/O code and the field mapping is
// the input to the real /full builder, not the thing under test.
func configuredFromSpeakerSources(srs models.Sources) []models.ConfiguredSource {
	var out []models.ConfiguredSource

	for _, s := range srs.SourceItem {
		cs := models.ConfiguredSource{DisplayName: s.DisplayName}

		if s.Status == models.SourceStatusReady {
			cs.SecretType = constants.CredentialTypeToken
		}

		if s.Source == constants.ProviderSpotify {
			cs.SecretType = constants.CredentialTypeTokenV3
		}

		cs.SourceKey.Type = s.Source
		cs.SourceKey.Account = s.SourceAccount
		cs.SourceKeyType = s.Source
		cs.SourceKeyAccount = s.SourceAccount

		out = append(out, cs)
	}

	return out
}

// TestI334FullOmitsSourcesWithoutProviderID guards the #334 fix: a speaker's
// legitimate /sources list (the fixture has zero INVALID_SOURCE entries) is
// imported, persisted, and then turned into the /full <sources> block by
// getAccountSources.  After the fix:
//
//   - Every emitted source must have a non-empty SourceProviderID; any source
//     without one (STORED_MUSIC_MEDIA_RENDERER, UPNP, …) would be rejected by
//     the speaker as INVALID_SOURCE, creating the feedback loop that #334
//     identified.
//   - The two specific device-local slots from v-tron's datastore
//     (StoredMusicUserName / STORED_MUSIC_MEDIA_RENDERER and UPnPUserName /
//     UPNP) must be absent from the emitted set entirely.
//
// Precedent: AUX was excluded the same way for issue #195 (see the comment
// block immediately before the ProviderAux continue in getAccountSources).
// This test extends that pattern to all sources whose type has no entry in
// constants.StaticProviders.
func TestI334FullOmitsSourcesWithoutProviderID(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "i334_speaker_sources.xml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var srs models.Sources
	if err := xml.Unmarshal(raw, &srs); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	imported := configuredFromSpeakerSources(srs)
	if len(imported) == 0 {
		t.Fatal("fixture produced no sources")
	}

	// Filter as the import path does: drop unresolvable sources before save.
	var servable []models.ConfiguredSource
	for _, s := range imported {
		if HasResolvableProviderID(s) {
			servable = append(servable, s)
		}
	}

	const (
		account = "ACCT01"
		device  = "DEVICEID01"
	)

	ds := datastore.NewDataStore(t.TempDir())
	if err := ds.SaveConfiguredSources(account, device, servable); err != nil {
		t.Fatalf("save configured sources: %v", err)
	}

	// Real /full source build: merge defaults + PrepareConfiguredSource +
	// mapToFullResponseSource, exactly as AccountFullToXML invokes it.
	full := getAccountSources(ds, account, device)

	t.Logf("%-26s | %-8s | %-13s | %s", "out.name", "out.type", "out.provider", "credential.type")
	for _, s := range full {
		t.Logf("%-26q | %-8q | %-13q | %q", s.Name, s.Type, s.SourceProviderID, s.Credential.Type)
	}

	// Every emitted source must have a non-empty SourceProviderID.
	for _, s := range full {
		if s.SourceProviderID == "" {
			t.Errorf("source name=%q was emitted with empty SourceProviderID — it would be rejected as INVALID_SOURCE (#334)", s.Name)
		}
	}

	// The two specific device-local slots from the fixture must be absent.
	emittedNames := make(map[string]bool, len(full))
	for _, s := range full {
		emittedNames[s.Name] = true
	}

	for _, mustBeAbsent := range []string{"StoredMusicUserName", "UPnPUserName"} {
		if emittedNames[mustBeAbsent] {
			t.Errorf("source name=%q must not appear in /full (device-local slot with no resolvable sourceproviderid, issue #334)", mustBeAbsent)
		}
	}
}

// TestHasResolvableProviderID verifies the HasResolvableProviderID predicate
// used to filter sources before persistence and before serving via /full.
func TestHasResolvableProviderID(t *testing.T) {
	// Sources that MUST be servable (true): their SourceKey.Type maps to a
	// constants.StaticProviders entry, so ensureSourceProviderID can fill the
	// canonical providerid.
	trueTable := []struct {
		name string
		typ  string
	}{
		{"TUNEIN", constants.ProviderTunein},
		{"SPOTIFY", constants.ProviderSpotify},
		{"RADIO_BROWSER", constants.ProviderRadioBrowser},
		{"LOCAL_INTERNET_RADIO", constants.ProviderLocalInternetRadio},
		{"STORED_MUSIC", constants.ProviderStoredMusic},
		{"INTERNET_RADIO", constants.ProviderInternetRadio},
		{"AUX", constants.ProviderAux},
	}

	for _, tc := range trueTable {
		t.Run("true/"+tc.name, func(t *testing.T) {
			s := models.ConfiguredSource{}
			s.SourceKey.Type = tc.typ

			if !HasResolvableProviderID(s) {
				t.Errorf("HasResolvableProviderID(%q) = false, want true", tc.typ)
			}
		})
	}

	// Sources that MUST NOT be servable (false): no StaticProviders entry,
	// so ensureSourceProviderID would leave SourceProviderID empty and the
	// speaker would reject the source as INVALID_SOURCE.
	falseTable := []struct {
		name string
		typ  string
	}{
		{"STORED_MUSIC_MEDIA_RENDERER", "STORED_MUSIC_MEDIA_RENDERER"},
		{"UPNP", "UPNP"},
		{"INVALID_SOURCE", "INVALID_SOURCE"},
		{"empty type", ""},
	}

	for _, tc := range falseTable {
		t.Run("false/"+tc.name, func(t *testing.T) {
			s := models.ConfiguredSource{}
			s.SourceKey.Type = tc.typ

			if HasResolvableProviderID(s) {
				t.Errorf("HasResolvableProviderID(%q) = true, want false", tc.typ)
			}
		})
	}

	// A source that already carries a non-empty SourceProviderID must return
	// true regardless of its type — the existing providerid wins.
	t.Run("true/existing-providerid-overrides-type", func(t *testing.T) {
		s := models.ConfiguredSource{SourceProviderID: "99"}
		s.SourceKey.Type = "STORED_MUSIC_MEDIA_RENDERER" // normally false

		if !HasResolvableProviderID(s) {
			t.Error("HasResolvableProviderID with non-empty SourceProviderID = false, want true")
		}
	})

	// Legacy SourceKeyType field must also be checked when SourceKey.Type is empty.
	t.Run("true/legacy-sourcekeytype", func(t *testing.T) {
		s := models.ConfiguredSource{SourceKeyType: constants.ProviderTunein}
		// SourceKey.Type intentionally left empty

		if !HasResolvableProviderID(s) {
			t.Errorf("HasResolvableProviderID with SourceKeyType=%q (SourceKey.Type empty) = false, want true", constants.ProviderTunein)
		}
	})
}
