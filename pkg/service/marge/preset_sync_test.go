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

	applied := PropagatePresetWrite(ds, account, "SPEAKERA01", 1, before)
	if len(applied) != 1 || applied[0].DeviceID != "SPEAKERB02" {
		t.Fatalf("applied = %+v, want the sibling speaker", applied)
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

	applied := PropagatePresetWrite(ds, account, "SPEAKERA01", 2,
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

	if applied := PropagatePresetWrite(ds, account, "SPEAKERA01", 1, kitchen); len(applied) != 0 {
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

	if applied := PropagatePresetWrite(ds, account, "SPEAKERA01", 1, kitchen); len(applied) != 1 {
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

	if applied := PropagatePresetWrite(ds, account, "SPEAKERA01", 1,
		[]models.ServicePreset{radioPreset("1", "Station", "http://example.invalid/station")}); len(applied) != 0 {
		t.Fatalf("applied = %+v, want nothing outside the account", applied)
	}
}
