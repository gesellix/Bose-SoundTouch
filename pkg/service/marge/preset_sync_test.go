package marge

import (
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// presetSyncFixture registers devices of one account and gives each the
// presets it should start with.
func presetSyncFixture(t *testing.T, account string, banks map[string][]models.ServicePreset) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "preset-sync-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)

	for device, presets := range banks {
		if mkErr := os.MkdirAll(ds.AccountDeviceDir(account, device), 0o755); mkErr != nil {
			t.Fatalf("mkdir %s: %v", device, mkErr)
		}

		if saveErr := ds.SaveDeviceInfo(account, device, &models.ServiceDeviceInfo{
			DeviceID:  device,
			AccountID: account,
			IPAddress: "192.0.2.10",
			Name:      device,
		}); saveErr != nil {
			t.Fatalf("save device %s: %v", device, saveErr)
		}

		if len(presets) == 0 {
			continue
		}

		if saveErr := ds.SavePresets(account, device, presets); saveErr != nil {
			t.Fatalf("save presets for %s: %v", device, saveErr)
		}
	}

	return ds
}

func radioPreset(button, name, location string) models.ServicePreset {
	preset := models.ServicePreset{ButtonNumber: button, ID: button}
	preset.Source = "LOCAL_INTERNET_RADIO"
	preset.Location = location
	preset.Name = name

	return preset
}

func presetAt(t *testing.T, ds *datastore.DataStore, account, device, button string) models.ServicePreset {
	t.Helper()

	presets, err := ds.GetPresetsReadOnly(account, device)
	if err != nil {
		t.Fatalf("read presets of %s: %v", device, err)
	}

	for i := range presets {
		if presets[i].ButtonNumber == button || presets[i].ID == button {
			return presets[i]
		}
	}

	return models.ServicePreset{}
}

func TestPropagatePresetWriteSharesTheSlotWithAgreeingSpeakers(t *testing.T) {
	const account = "7000001"

	before := []models.ServicePreset{radioPreset("1", "Old", "http://example.invalid/old")}
	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		"SPEAKERA01": before,
		"SPEAKERB02": {radioPreset("1", "Old", "http://example.invalid/old")},
	})

	if err := ds.SavePresets(account, "SPEAKERA01",
		[]models.ServicePreset{radioPreset("1", "New", "http://example.invalid/new")}); err != nil {
		t.Fatalf("write new preset: %v", err)
	}

	applied, skipped := PropagatePresetWrite(ds, account, "SPEAKERA01", 1, before)
	if len(applied) != 1 || applied[0].DeviceID != "SPEAKERB02" {
		t.Fatalf("applied = %+v, want the sibling speaker", applied)
	}

	if len(skipped) != 0 {
		t.Fatalf("skipped = %+v, want none", skipped)
	}

	if got := presetAt(t, ds, account, "SPEAKERB02", "1"); got.Location != "http://example.invalid/new" {
		t.Fatalf("sibling preset = %+v, want the new location", got)
	}
}

func TestPropagatePresetWriteAdoptsOntoAnEmptySpeakerEvenWhenOff(t *testing.T) {
	const account = "7000002"

	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		"SPEAKERA01": {radioPreset("2", "Station", "http://example.invalid/station")},
		"SPEAKERNEW": nil,
	})

	if err := ds.SaveAccountInfo(account, &models.ServiceAccountInfo{
		AccountID:  account,
		PresetSync: PresetSyncOff,
	}); err != nil {
		t.Fatalf("save account info: %v", err)
	}

	applied, _ := PropagatePresetWrite(ds, account, "SPEAKERA01", 2,
		[]models.ServicePreset{radioPreset("2", "Station", "http://example.invalid/station")})
	if len(applied) != 1 || applied[0].DeviceID != "SPEAKERNEW" {
		t.Fatalf("applied = %+v, want the speaker without presets", applied)
	}

	if got := presetAt(t, ds, account, "SPEAKERNEW", "2"); got.Location != "http://example.invalid/station" {
		t.Fatalf("new speaker did not adopt the preset: %+v", got)
	}
}

func TestPropagatePresetWriteLeavesDivergingSpeakersAlone(t *testing.T) {
	const account = "7000003"

	kitchen := []models.ServicePreset{radioPreset("1", "Kitchen station", "http://example.invalid/kitchen")}
	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		"SPEAKERA01": kitchen,
		"SPEAKERB02": {radioPreset("1", "Bedroom station", "http://example.invalid/bedroom")},
	})

	if applied, _ := PropagatePresetWrite(ds, account, "SPEAKERA01", 1, kitchen); len(applied) != 0 {
		t.Fatalf("applied = %+v, want nothing while the banks disagree", applied)
	}

	if got := presetAt(t, ds, account, "SPEAKERB02", "1"); got.Location != "http://example.invalid/bedroom" {
		t.Fatalf("diverging speaker was overwritten: %+v", got)
	}

	// The owner can still ask for it explicitly.
	if err := ds.SaveAccountInfo(account, &models.ServiceAccountInfo{
		AccountID:  account,
		PresetSync: PresetSyncOn,
	}); err != nil {
		t.Fatalf("save account info: %v", err)
	}

	if applied, _ := PropagatePresetWrite(ds, account, "SPEAKERA01", 1, kitchen); len(applied) != 1 {
		t.Fatalf("applied = %+v, want the sibling once sync is on", applied)
	}

	if got := presetAt(t, ds, account, "SPEAKERB02", "1"); got.Location != "http://example.invalid/kitchen" {
		t.Fatalf("sibling preset = %+v, want the source speaker's", got)
	}
}

func TestPropagatePresetRemovalClearsTheSameSlot(t *testing.T) {
	const account = "7000004"

	shared := radioPreset("3", "Shared", "http://example.invalid/shared")
	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		"SPEAKERA01": {shared},
		"SPEAKERB02": {radioPreset("3", "Shared", "http://example.invalid/shared")},
	})

	if applied := PropagatePresetRemoval(ds, account, "SPEAKERA01", 3, []models.ServicePreset{shared}); len(applied) != 1 {
		t.Fatalf("applied = %+v, want the sibling", applied)
	}

	if got := presetAt(t, ds, account, "SPEAKERB02", "3"); got.Location != "" {
		t.Fatalf("sibling slot was not cleared: %+v", got)
	}
}

func TestPresetSyncIgnoresOtherAccounts(t *testing.T) {
	const account = "7000005"

	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		"SPEAKERA01": {radioPreset("1", "Station", "http://example.invalid/station")},
	})

	const foreign = "7000006"
	if mkErr := os.MkdirAll(ds.AccountDeviceDir(foreign, "SPEAKERX09"), 0o755); mkErr != nil {
		t.Fatalf("mkdir foreign device: %v", mkErr)
	}

	if saveErr := ds.SaveDeviceInfo(foreign, "SPEAKERX09", &models.ServiceDeviceInfo{
		DeviceID:  "SPEAKERX09",
		AccountID: foreign,
	}); saveErr != nil {
		t.Fatalf("save foreign device: %v", saveErr)
	}

	if applied, _ := PropagatePresetWrite(ds, account, "SPEAKERA01", 1,
		[]models.ServicePreset{radioPreset("1", "Station", "http://example.invalid/station")}); len(applied) != 0 {
		t.Fatalf("applied = %+v, want nothing outside the account", applied)
	}
}

// spotifyPreset builds a SPOTIFY preset claiming account, the shape a real
// speaker reports for a linked music service.
func spotifyPreset(button, name, location, account string) models.ServicePreset {
	preset := models.ServicePreset{ButtonNumber: button, ID: button}
	preset.Source = "SPOTIFY"
	preset.SourceAccount = account
	preset.Location = location
	preset.Name = name

	return preset
}

// spotifySource is a configured SPOTIFY source keyed by account, the shape
// AddSource/learnSource persists for a linked music service.
func spotifySource(account string) models.ConfiguredSource {
	source := models.ConfiguredSource{
		ID:            "20001",
		Type:          "Audio",
		SourceKeyType: "SPOTIFY",
	}
	source.SourceKeyAccount = account
	source.SourceKey.Type = "SPOTIFY"
	source.SourceKey.Account = account

	return source
}

// TestPropagatePresetWriteSkipsSpotifyWithoutAMatchingSource is the
// reproducer for issue 495: sharing a SPOTIFY preset must not write it
// verbatim onto a sibling that has no SPOTIFY source at all -- that leaves
// the sibling with a preset it answers INVALID_SOURCE for.
func TestPropagatePresetWriteSkipsSpotifyWithoutAMatchingSource(t *testing.T) {
	const account = "7000007"

	original := radioPreset("1", "Old", "http://example.invalid/old")
	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		"SPEAKERA01": {original},
		"SPEAKERB02": {original},
	})

	// SPEAKERB02 gets no Sources.xml at all, so it falls back to AfterTouch's
	// managed defaults, which never include SPOTIFY (issue 495: neither did
	// the reporter's second speaker).
	if err := ds.SavePresets(account, "SPEAKERA01",
		[]models.ServicePreset{spotifyPreset("1", "My Playlist", "spotify:playlist:abc", "listener@example.invalid")}); err != nil {
		t.Fatalf("write spotify preset: %v", err)
	}

	applied, skipped := PropagatePresetWrite(ds, account, "SPEAKERA01", 1, []models.ServicePreset{original})
	if len(applied) != 0 {
		t.Fatalf("applied = %+v, want nothing: the sibling has no SPOTIFY source", applied)
	}

	if len(skipped) != 1 || skipped[0].DeviceID != "SPEAKERB02" {
		t.Fatalf("skipped = %+v, want SPEAKERB02 reported", skipped)
	}

	if got := presetAt(t, ds, account, "SPEAKERB02", "1"); got.Location != original.Location {
		t.Fatalf("sibling preset = %+v, want it left untouched", got)
	}
}

// TestPropagatePresetWriteSkipsSpotifyWithADifferentAccount covers a sibling
// that does have Spotify, but linked to a different account than the preset
// claims: that source cannot serve the preset either, so it must be skipped
// rather than bound to the wrong account.
func TestPropagatePresetWriteSkipsSpotifyWithADifferentAccount(t *testing.T) {
	const account = "7000008"

	original := radioPreset("1", "Old", "http://example.invalid/old")
	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		"SPEAKERA01": {original},
		"SPEAKERB02": {original},
	})

	if err := ds.SaveConfiguredSources(account, "SPEAKERB02",
		[]models.ConfiguredSource{spotifySource("someoneelse@example.invalid")}); err != nil {
		t.Fatalf("save sibling sources: %v", err)
	}

	if err := ds.SavePresets(account, "SPEAKERA01",
		[]models.ServicePreset{spotifyPreset("1", "My Playlist", "spotify:playlist:abc", "listener@example.invalid")}); err != nil {
		t.Fatalf("write spotify preset: %v", err)
	}

	applied, skipped := PropagatePresetWrite(ds, account, "SPEAKERA01", 1, []models.ServicePreset{original})
	if len(applied) != 0 {
		t.Fatalf("applied = %+v, want nothing: the sibling's SPOTIFY is a different account", applied)
	}

	if len(skipped) != 1 || skipped[0].DeviceID != "SPEAKERB02" {
		t.Fatalf("skipped = %+v, want SPEAKERB02 reported", skipped)
	}

	if got := presetAt(t, ds, account, "SPEAKERB02", "1"); got.Location != original.Location {
		t.Fatalf("sibling preset = %+v, want it left untouched", got)
	}
}

// TestPropagatePresetWriteSharesSpotifyWithTheSameAccount is the control
// case: once the sibling has SPOTIFY linked under the same account the
// preset claims, sharing proceeds exactly as for a radio preset.
func TestPropagatePresetWriteSharesSpotifyWithTheSameAccount(t *testing.T) {
	const account = "7000009"

	original := radioPreset("1", "Old", "http://example.invalid/old")
	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		"SPEAKERA01": {original},
		"SPEAKERB02": {original},
	})

	if err := ds.SaveConfiguredSources(account, "SPEAKERB02",
		[]models.ConfiguredSource{spotifySource("listener@example.invalid")}); err != nil {
		t.Fatalf("save sibling sources: %v", err)
	}

	if err := ds.SavePresets(account, "SPEAKERA01",
		[]models.ServicePreset{spotifyPreset("1", "My Playlist", "spotify:playlist:abc", "listener@example.invalid")}); err != nil {
		t.Fatalf("write spotify preset: %v", err)
	}

	applied, skipped := PropagatePresetWrite(ds, account, "SPEAKERA01", 1, []models.ServicePreset{original})
	if len(skipped) != 0 {
		t.Fatalf("skipped = %+v, want none: the sibling has the same SPOTIFY account", skipped)
	}

	if len(applied) != 1 || applied[0].DeviceID != "SPEAKERB02" {
		t.Fatalf("applied = %+v, want the sibling speaker", applied)
	}

	if got := presetAt(t, ds, account, "SPEAKERB02", "1"); got.Location != "spotify:playlist:abc" {
		t.Fatalf("sibling preset = %+v, want the shared spotify location", got)
	}
}

// TestPropagatePresetWriteAddsMissingTuneInBeforeWriting covers the other
// half of issue 495: TUNEIN (like RADIO_BROWSER and LOCAL_INTERNET_RADIO) is
// a source AfterTouch mints itself, so a sibling that lacks it gets the
// canonical entry added before the preset is written, rather than being
// skipped like a linked music service.
func TestPropagatePresetWriteAddsMissingTuneInBeforeWriting(t *testing.T) {
	const account = "7000011"

	original := radioPreset("1", "Old", "http://example.invalid/old")
	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		"SPEAKERA01": {original},
		"SPEAKERB02": {original},
	})

	// Give SPEAKERB02 an explicit Sources.xml missing TUNEIN, so it does not
	// fall back to AfterTouch's defaults (which already include TUNEIN and
	// would make this test pass without exercising the add path).
	withoutTuneIn := make([]models.ConfiguredSource, 0)

	for _, s := range ds.GetDefaultSources() {
		if s.SourceKeyType != "TUNEIN" {
			withoutTuneIn = append(withoutTuneIn, s)
		}
	}

	if err := ds.SaveConfiguredSources(account, "SPEAKERB02", withoutTuneIn); err != nil {
		t.Fatalf("save sibling sources: %v", err)
	}

	if err := ds.SavePresets(account, "SPEAKERA01",
		[]models.ServicePreset{{
			ServiceContentItem: models.ServiceContentItem{
				Source:   "TUNEIN",
				Location: "/stations/byuuid/00000000-0000-0000-0000-000000000000",
				Name:     "A Station",
			},
			ButtonNumber: "1",
			ID:           "1",
		}}); err != nil {
		t.Fatalf("write tunein preset: %v", err)
	}

	applied, skipped := PropagatePresetWrite(ds, account, "SPEAKERA01", 1, []models.ServicePreset{original})
	if len(skipped) != 0 {
		t.Fatalf("skipped = %+v, want none: TUNEIN can be added", skipped)
	}

	if len(applied) != 1 || applied[0].DeviceID != "SPEAKERB02" {
		t.Fatalf("applied = %+v, want the sibling speaker", applied)
	}

	sources, err := ds.GetConfiguredSources(account, "SPEAKERB02")
	if err != nil {
		t.Fatalf("read sibling sources: %v", err)
	}

	found := false

	for i := range sources {
		if sources[i].SourceKeyType == "TUNEIN" {
			found = true
		}
	}

	if !found {
		t.Fatalf("sources = %+v, want TUNEIN added", sources)
	}

	if got := presetAt(t, ds, account, "SPEAKERB02", "1"); got.Location != "/stations/byuuid/00000000-0000-0000-0000-000000000000" {
		t.Fatalf("sibling preset = %+v, want the shared tunein location", got)
	}
}

// TestPropagatePresetWriteSkipsStoredMusicEvenThoughItIsGenerallyAddable
// documents the STORED_MUSIC decision for this code path: even though
// models.SourceAvailability calls STORED_MUSIC addable (the player's "add a
// source elsewhere" flow can add it by talking to the speaker directly),
// preset sync has no established way to reach a sibling speaker for a write
// like setMusicServiceAccount, so it is treated like a source that cannot be
// added here and the sibling is skipped.
func TestPropagatePresetWriteSkipsStoredMusicEvenThoughItIsGenerallyAddable(t *testing.T) {
	const account = "7000012"

	original := radioPreset("1", "Old", "http://example.invalid/old")
	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		"SPEAKERA01": {original},
		"SPEAKERB02": {original},
	})

	if err := ds.SavePresets(account, "SPEAKERA01",
		[]models.ServicePreset{{
			ServiceContentItem: models.ServiceContentItem{
				Source:        "STORED_MUSIC",
				SourceAccount: "UUUUUUUU-UUUU-UUUU-UUUU-UUUUUUUUUUUU/0",
				Location:      "4:cont2:615:part12:39",
				Name:          "A Folder",
			},
			ButtonNumber: "1",
			ID:           "1",
		}}); err != nil {
		t.Fatalf("write stored music preset: %v", err)
	}

	applied, skipped := PropagatePresetWrite(ds, account, "SPEAKERA01", 1, []models.ServicePreset{original})
	if len(applied) != 0 {
		t.Fatalf("applied = %+v, want nothing: STORED_MUSIC is registered on the speaker itself", applied)
	}

	if len(skipped) != 1 || skipped[0].DeviceID != "SPEAKERB02" {
		t.Fatalf("skipped = %+v, want SPEAKERB02 reported", skipped)
	}

	if got := presetAt(t, ds, account, "SPEAKERB02", "1"); got.Location != original.Location {
		t.Fatalf("sibling preset = %+v, want it left untouched", got)
	}
}
