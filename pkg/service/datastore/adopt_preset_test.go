package datastore

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestSetStoredPresetReplacesOneSlotOnly(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	if err := ds.SavePresets("ACCOUNT01", "DEVICEID01", []models.ServicePreset{
		storedPreset("1", "TUNEIN", "s1", "MDR JUMP"),
		storedPreset("2", "SPOTIFY", "spotify:album:1", "White Water"),
		storedPreset("3", "TUNEIN", "s3", "WDR 2"),
	}); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	rows, err := ds.SetStoredPreset("ACCOUNT01", "DEVICEID01",
		storedPreset("2", "RADIO_BROWSER", "uuid-fm4", "FM4"))
	if err != nil {
		t.Fatalf("SetStoredPreset: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("expected the list to stay three rows, got %d", len(rows))
	}

	bySlot := map[int]models.StoredPresetRow{}
	for _, row := range rows {
		bySlot[row.Slot] = row
	}

	if bySlot[2].Name != "FM4" {
		t.Errorf("slot 2 = %q, want the adopted content", bySlot[2].Name)
	}

	// The point of a per-slot choice is that the other five are untouched.
	if bySlot[1].Name != "MDR JUMP" || bySlot[3].Name != "WDR 2" {
		t.Errorf("neighbouring slots changed: %+v", rows)
	}
}

// Choosing one side of a disagreement must not lose the other: the displaced
// content stays on the pick list, so the choice is reversible.
func TestSetStoredPresetFilesWhatItDisplaces(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	if err := ds.SavePresets("ACCOUNT01", "DEVICEID01", []models.ServicePreset{
		storedPreset("1", "TUNEIN", "s1", "The one we had"),
	}); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	if _, err := ds.SetStoredPreset("ACCOUNT01", "DEVICEID01",
		storedPreset("1", "SPOTIFY", "spotify:album:9", "The speaker's one")); err != nil {
		t.Fatalf("SetStoredPreset: %v", err)
	}

	var found bool

	for _, entry := range ds.GetCatalog() {
		if entry.Name == "The one we had" {
			found = true
		}
	}

	if !found {
		t.Fatal("the displaced content is not in the catalog, so the choice cannot be undone")
	}
}

func TestSetStoredPresetFillsAnEmptySlot(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	rows, err := ds.SetStoredPreset("ACCOUNT01", "DEVICEID01",
		storedPreset("4", "TUNEIN", "s4", "Newly stored"))
	if err != nil {
		t.Fatalf("SetStoredPreset: %v", err)
	}

	if len(rows) != 1 || rows[0].Slot != 4 || rows[0].Verdict != models.StoredPresetOK {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestSetStoredPresetRejectsAButtonTheSpeakerDoesNotHave(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	for _, button := range []string{"0", "7", "", "x"} {
		if _, err := ds.SetStoredPreset("ACCOUNT01", "DEVICEID01",
			storedPreset(button, "TUNEIN", "s1", "Nowhere")); err == nil {
			t.Errorf("button %q: expected a refusal", button)
		}
	}
}
