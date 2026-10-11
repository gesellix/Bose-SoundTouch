package marge

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// Issue 784: clearing a slot blanked its row instead of removing it. The blank
// row had no button number, so storing the slot again added a seventh row
// next to it, and /full logged a skipped preset on every fetch.
func TestRemovePresetDropsTheRow_GH784(t *testing.T) {
	const account, device = "7840001", "DEVICEID01"

	ds := datastore.NewDataStore(t.TempDir())

	presets := make([]models.ServicePreset, 0, 6)
	for i := 1; i <= 6; i++ {
		presets = append(presets, radioPreset(strconv.Itoa(i), "Station "+strconv.Itoa(i), "http://example.invalid/"+strconv.Itoa(i)))
	}

	if err := ds.SavePresets(account, device, presets); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	if err := RemovePreset(ds, account, device, 6); err != nil {
		t.Fatalf("RemovePreset: %v", err)
	}

	got, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if len(got) != 5 {
		t.Fatalf("after clearing slot 6: %d rows, want 5", len(got))
	}

	if _, err := ds.MutatePresets(account, device, func(current []models.ServicePreset) ([]models.ServicePreset, error) {
		return upsertPresetByButton(current, radioPreset("6", "Again", "http://example.invalid/again")), nil
	}); err != nil {
		t.Fatalf("store slot 6 again: %v", err)
	}

	got, err = ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if len(got) != 6 {
		t.Fatalf("after storing slot 6 again: %d rows, want 6: %+v", len(got), got)
	}
}

// Rows blanked by older versions are dropped when the file is read, and the
// file is rewritten without them: the id-less row from the speaker the clear
// came from, and the row that kept its id on the speaker it was shared to.
func TestBlankPresetRowsAreDroppedOnRead_GH784(t *testing.T) {
	const account, device = "7840002", "DEVICEID02"

	ds := datastore.NewDataStore(t.TempDir())
	if err := ds.SavePresets(account, device, []models.ServicePreset{
		radioPreset("1", "Kept", "http://example.invalid/kept"),
	}); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.PresetsFile)
	legacy := `<presets>` +
		`<preset id="1" createdOn="1" updatedOn="1"><contentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="http://example.invalid/kept" sourceAccount="" isPresetable="true"><itemName>Kept</itemName><containerArt></containerArt></contentItem></preset>` +
		`<preset id="" createdOn="" updatedOn=""><contentItem type="" location="" sourceAccount="" isPresetable="true"><itemName></itemName><containerArt></containerArt></contentItem></preset>` +
		`<preset id="6" createdOn="" updatedOn=""><contentItem type="" location="" sourceAccount="" isPresetable="true"><itemName></itemName><containerArt></containerArt></contentItem></preset>` +
		`</presets>`

	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy Presets.xml: %v", err)
	}

	got, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if len(got) != 1 || got[0].ButtonNumber != "1" {
		t.Fatalf("presets = %+v, want only button 1", got)
	}

	snapshot, err := ds.ReadPresetSnapshot(account, device)
	if err != nil {
		t.Fatalf("ReadPresetSnapshot: %v", err)
	}

	if len(snapshot.Presets) != 1 {
		t.Fatalf("rewritten file still holds %d rows, want 1", len(snapshot.Presets))
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}

	if string(raw) == legacy {
		t.Fatal("Presets.xml was not rewritten")
	}
}
