package datastore

import (
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func storedPreset(button, source, location, name string) models.ServicePreset {
	p := models.ServicePreset{ButtonNumber: button, ID: button}
	p.Source = source
	p.Location = location
	p.Name = name

	return p
}

// The issue 697 shape: eight stored rows for a six-button speaker, which makes
// every sync look destructive and keeps being served back to the speaker.
func TestClassifyStoredPresetsNamesWhyARowCannotBeRecalled(t *testing.T) {
	rows := ClassifyStoredPresets([]models.ServicePreset{
		storedPreset("1", "TUNEIN", "s1", "MDR JUMP"),
		storedPreset("2", "SPOTIFY", "spotify:album:1", "White Water"),
		storedPreset("7", "TUNEIN", "s7", "Out of range"),
		storedPreset("", "TUNEIN", "s8", "No id at all"),
		storedPreset("x", "TUNEIN", "s9", "Not a number"),
		storedPreset("3", "", "", "Nothing to play"),
	})

	want := []string{
		models.StoredPresetOK,
		models.StoredPresetOK,
		models.StoredPresetOutOfRange,
		models.StoredPresetNoSlot,
		models.StoredPresetNoSlot,
		models.StoredPresetEmptyContent,
	}

	if len(rows) != len(want) {
		t.Fatalf("expected %d rows, got %d", len(want), len(rows))
	}

	for i := range want {
		if rows[i].Verdict != want[i] {
			t.Errorf("row %d: verdict %q, want %q", i, rows[i].Verdict, want[i])
		}

		if rows[i].Index != i {
			t.Errorf("row %d carries index %d", i, rows[i].Index)
		}
	}

	if !rows[0].OccupiesAButton() || rows[2].OccupiesAButton() {
		t.Error("OccupiesAButton must be true only for a row on a button the speaker has")
	}
}

func TestDropStoredPresetRowsCutsTheListToWhatFits(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	stored := []models.ServicePreset{
		storedPreset("1", "TUNEIN", "s1", "MDR JUMP"),
		storedPreset("7", "TUNEIN", "s7", "Out of range"),
		storedPreset("2", "SPOTIFY", "spotify:album:1", "White Water"),
		storedPreset("", "TUNEIN", "s8", "No id at all"),
	}

	if err := ds.SavePresets("ACCOUNT01", "DEVICEID01", stored); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	rows, err := ds.StoredPresets("ACCOUNT01", "DEVICEID01")
	if err != nil {
		t.Fatalf("StoredPresets: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("expected the 4 stored rows, got %d", len(rows))
	}

	drop := []int{}

	for _, row := range rows {
		if !row.OccupiesAButton() {
			drop = append(drop, row.Index)
		}
	}

	kept, err := ds.DropStoredPresetRows("ACCOUNT01", "DEVICEID01", drop, len(rows))
	if err != nil {
		t.Fatalf("DropStoredPresetRows: %v", err)
	}

	if len(kept) != 2 {
		t.Fatalf("expected 2 rows left, got %d", len(kept))
	}

	for _, row := range kept {
		if !row.OccupiesAButton() {
			t.Errorf("row on button %q survived the repair with verdict %q", row.Button, row.Verdict)
		}
	}

	// The catalog keeps what the repair removed, so a row deleted by mistake
	// can be put back from the pick list.
	names := map[string]bool{}
	for _, e := range ds.GetCatalog() {
		names[e.Name] = true
	}

	for _, name := range []string{"Out of range", "No id at all"} {
		if !names[name] {
			t.Errorf("%q is gone from the catalog as well as from the stored list", name)
		}
	}
}

// The count the caller was looking at is the guard: without it, a repair
// computed against a stale view would delete rows by position that have since
// moved.
func TestDropStoredPresetRowsRefusesAStaleView(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	if err := ds.SavePresets("ACCOUNT01", "DEVICEID01", []models.ServicePreset{
		storedPreset("1", "TUNEIN", "s1", "MDR JUMP"),
		storedPreset("2", "SPOTIFY", "spotify:album:1", "White Water"),
	}); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	_, err := ds.DropStoredPresetRows("ACCOUNT01", "DEVICEID01", []int{1}, 3)
	if err == nil {
		t.Fatal("expected a stale row count to be refused")
	}

	if !strings.Contains(err.Error(), "reload") {
		t.Errorf("the error should tell the caller what to do, got %q", err)
	}

	rows, _ := ds.StoredPresets("ACCOUNT01", "DEVICEID01")
	if len(rows) != 2 {
		t.Fatalf("a refused repair must change nothing, got %d rows", len(rows))
	}
}

func TestDropStoredPresetRowsRejectsAnIndexOutsideTheList(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	if err := ds.SavePresets("ACCOUNT01", "DEVICEID01", []models.ServicePreset{
		storedPreset("1", "TUNEIN", "s1", "MDR JUMP"),
	}); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	if _, err := ds.DropStoredPresetRows("ACCOUNT01", "DEVICEID01", []int{5}, 1); err == nil {
		t.Fatal("expected an out-of-list row index to be refused")
	}

	if rows, _ := ds.StoredPresets("ACCOUNT01", "DEVICEID01"); len(rows) != 1 {
		t.Fatalf("a refused repair must change nothing, got %d rows", len(rows))
	}
}

// DeviceDirExists is what lets a caller holding a speaker-reported account
// tell "this pairing is on disk" from "this is a name we have nothing for",
// without that question costing a full device listing.
func TestDeviceDirExists(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	if err := ds.SavePresets("ACCOUNT01", "DEVICEID01", []models.ServicePreset{
		storedPreset("1", "TUNEIN", "s1", "MDR JUMP"),
	}); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	if !ds.DeviceDirExists("ACCOUNT01", "DEVICEID01") {
		t.Error("expected the account we just wrote under to exist")
	}

	for _, tt := range []struct{ account, device string }{
		{"ACCOUNT02", "DEVICEID01"},
		{"ACCOUNT01", "DEVICEID02"},
		{"", "DEVICEID01"},
		{"ACCOUNT01", ""},
		{"../escape", "DEVICEID01"},
	} {
		if ds.DeviceDirExists(tt.account, tt.device) {
			t.Errorf("DeviceDirExists(%q, %q) = true, want false", tt.account, tt.device)
		}
	}
}
