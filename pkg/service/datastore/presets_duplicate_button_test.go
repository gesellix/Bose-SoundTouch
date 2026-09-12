package datastore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// A Presets.xml carrying two entries for button 6, as observed on a real
// device (issue 715). Written in the canonical lowercase <contentItem>
// spelling, so the duplicate is the only reason a rewrite would be needed.
const duplicateButtonPresetsXML = `<?xml version="1.0" encoding="UTF-8" ?>
<presets>
  <preset id="1" createdOn="" updatedOn="">
    <contentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s1" sourceAccount="" isPresetable="true">
      <itemName>A Station</itemName>
    </contentItem>
  </preset>
  <preset id="4" createdOn="" updatedOn="">
    <contentItem source="STORED_MUSIC" type="" location="1" sourceAccount="UUUUUUUU-UUUU-UUUU-UUUU-UUUUUUUUUUUU/0" isPresetable="true">
      <itemName>A Folder</itemName>
    </contentItem>
  </preset>
  <preset id="6" createdOn="" updatedOn="">
    <contentItem source="RADIO_BROWSER" type="stationurl" location="/stations/byuuid/0000" sourceAccount="" isPresetable="true">
      <itemName>Stale Entry</itemName>
    </contentItem>
  </preset>
  <preset id="6" createdOn="" updatedOn="">
    <contentItem source="RADIO_BROWSER" type="stationurl" location="/stations/byuuid/0000" sourceAccount="" isPresetable="true">
      <itemName>Fresh Entry</itemName>
    </contentItem>
  </preset>
</presets>`

func writeDuplicatePresets(t *testing.T) (ds *DataStore, account, device, path string) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "st-dup-preset-*")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	ds = NewDataStore(tempDir)
	account = "1234567"
	device = "001122334455"

	if mkErr := os.MkdirAll(ds.AccountDeviceDir(account, device), 0o755); mkErr != nil {
		t.Fatalf("mkdir device dir: %v", mkErr)
	}

	path = filepath.Join(ds.AccountDeviceDir(account, device), "Presets.xml")
	if wErr := os.WriteFile(path, []byte(duplicateButtonPresetsXML), 0o600); wErr != nil {
		t.Fatalf("write Presets.xml: %v", wErr)
	}

	return ds, account, device, path
}

func buttonNumbersOf(presets []models.ServicePreset) []string {
	out := make([]string, 0, len(presets))
	for i := range presets {
		out = append(out, presets[i].ButtonNumber)
	}

	return out
}

// A preset list longer than the speaker's six slots costs a slot, so the
// duplicate has to be collapsed — and repaired on disk, not filtered on every
// read.
func TestGetPresets_CollapsesDuplicateButtonAndRepairsTheFile(t *testing.T) {
	ds, account, device, path := writeDuplicatePresets(t)

	presets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if got, want := buttonNumbersOf(presets), []string{"1", "4", "6"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("buttons = %v, want %v", got, want)
	}

	// Later wins: the newest save is the one the user just made.
	if presets[2].Name != "Fresh Entry" {
		t.Errorf("button 6 name = %q, want %q", presets[2].Name, "Fresh Entry")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back Presets.xml: %v", err)
	}

	if n := strings.Count(string(data), `id="6"`); n != 1 {
		t.Errorf("Presets.xml still has %d entries for button 6, want 1 after the repair:\n%s", n, data)
	}

	if !strings.Contains(string(data), "Fresh Entry") {
		t.Errorf("repaired file lost the surviving entry:\n%s", data)
	}
}

// The read-only accessor exists for preflight paths that must not mutate
// datastore state, so it may collapse in memory but must leave the file alone.
func TestGetPresetsReadOnly_CollapsesWithoutWriting(t *testing.T) {
	ds, account, device, path := writeDuplicatePresets(t)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Presets.xml: %v", err)
	}

	presets, err := ds.GetPresetsReadOnly(account, device)
	if err != nil {
		t.Fatalf("GetPresetsReadOnly: %v", err)
	}

	if got, want := buttonNumbersOf(presets), []string{"1", "4", "6"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("buttons = %v, want %v", got, want)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Presets.xml again: %v", err)
	}

	if string(before) != string(after) {
		t.Error("GetPresetsReadOnly rewrote Presets.xml; it must not mutate datastore state")
	}
}

// The save path is the backstop: no writer may persist two entries for one
// slot, mirroring SaveRecents and SaveConfiguredSources.
func TestSavePresets_DedupesByButtonNumber(t *testing.T) {
	ds, account, device, path := writeDuplicatePresets(t)

	presets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{Name: "Stale Entry", Source: "RADIO_BROWSER"},
			ID:                 "6",
			ButtonNumber:       "6",
		},
		{
			ServiceContentItem: models.ServiceContentItem{Name: "Fresh Entry", Source: "RADIO_BROWSER"},
			ID:                 "6",
			ButtonNumber:       "6",
		},
	}

	if err := ds.SavePresets(account, device, presets); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Presets.xml: %v", err)
	}

	if n := strings.Count(string(data), `id="6"`); n != 1 {
		t.Errorf("wrote %d entries for button 6, want 1:\n%s", n, data)
	}

	if !strings.Contains(string(data), "Fresh Entry") {
		t.Errorf("kept the stale entry instead of the newest one:\n%s", data)
	}
}
