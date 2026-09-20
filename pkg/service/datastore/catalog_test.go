package datastore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/catalog"
)

func seen(min int) time.Time {
	return time.Date(2026, 9, 20, 12, min, 0, 0, time.UTC)
}

func TestCatalogRoundTrip(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	ds.RecordCatalogEntries([]catalog.Entry{
		{Source: "TUNEIN", Location: "s1", Name: "WDR 2", Origin: catalog.OriginPreset, LastSeen: seen(0)},
		{Source: "RADIO_BROWSER", Location: "uuid-2", Name: "FM4", Origin: catalog.OriginRecent, LastSeen: seen(5)},
	})

	// Read through a second datastore on the same directory, so the assertion
	// is about what was persisted rather than about in-memory state.
	entries := NewDataStore(ds.DataDir).GetCatalog()
	if len(entries) != 2 {
		t.Fatalf("expected 2 persisted entries, got %d", len(entries))
	}

	if entries[0].Name != "FM4" {
		t.Errorf("expected the newest sighting first, got %q", entries[0].Name)
	}
}

func TestGetCatalogOnAnEmptyDataDir(t *testing.T) {
	if entries := NewDataStore(t.TempDir()).GetCatalog(); len(entries) != 0 {
		t.Fatalf("expected no entries, got %d", len(entries))
	}
}

// The catalog is derived state: it rebuilds itself from the next write, so a
// corrupted file must not propagate an error into a preset write.
func TestCorruptCatalogFileIsIgnoredAndRebuilt(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	if err := os.MkdirAll(ds.DataDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := os.WriteFile(filepath.Join(ds.DataDir, CatalogFile), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if entries := ds.GetCatalog(); len(entries) != 0 {
		t.Fatalf("expected a corrupt catalog to read as empty, got %d entries", len(entries))
	}

	ds.RecordCatalogEntries([]catalog.Entry{{Source: "TUNEIN", Location: "s1", LastSeen: seen(0)}})

	if entries := ds.GetCatalog(); len(entries) != 1 {
		t.Fatalf("expected the catalog to rebuild, got %d entries", len(entries))
	}
}

func TestCatalogSizeSettingCapsAndDisables(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	two := 2
	if err := ds.SaveSettings(Settings{CatalogSize: &two}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	for i := 1; i <= 4; i++ {
		ds.RecordCatalogEntries([]catalog.Entry{
			{Source: "TUNEIN", Location: string(rune('a' + i)), LastSeen: seen(i)},
		})
	}

	if entries := ds.GetCatalog(); len(entries) != 2 {
		t.Fatalf("expected the configured cap of 2, got %d entries", len(entries))
	}

	off := 0
	if err := ds.SaveSettings(Settings{CatalogSize: &off}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	ds.RecordCatalogEntries([]catalog.Entry{{Source: "TUNEIN", Location: "z", LastSeen: seen(9)}})

	if entries := ds.GetCatalog(); len(entries) != 0 {
		t.Fatalf("expected a disabled catalog to hold nothing, got %d entries", len(entries))
	}
}

// An unset CatalogSize must not read as "disabled": that is the difference
// between a default install collecting a pick list and collecting nothing.
func TestUnsetCatalogSizeUsesTheDefault(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	if err := ds.SaveSettings(Settings{ServerURL: "http://192.0.2.1:8000"}); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	if got := ds.catalogSize(); got != catalog.DefaultSize {
		t.Fatalf("catalogSize() = %d, want the default %d", got, catalog.DefaultSize)
	}
}

// The feed: every preset write files what it stored, so nothing has to be
// recovered from hand-edited XML later.
func TestSavePresetsFilesTheCatalog(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	preset := models.ServicePreset{ContainerArt: "http://192.0.2.10/art.png"}
	preset.Source = "TUNEIN"
	preset.Location = "s12345"
	preset.Name = "WDR 2"
	preset.ButtonNumber = "1"

	if err := ds.SavePresets("1234567", "DEVICEID01", []models.ServicePreset{preset}); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	entries := ds.GetCatalog()
	if len(entries) != 1 {
		t.Fatalf("expected 1 catalog entry, got %d", len(entries))
	}

	if entries[0].Origin != catalog.OriginPreset {
		t.Errorf("Origin = %q, want %q", entries[0].Origin, catalog.OriginPreset)
	}

	if entries[0].DeviceID != "DEVICEID01" {
		t.Errorf("DeviceID = %q, want the device that was written", entries[0].DeviceID)
	}

	// Clearing the slot must not cost the catalog entry -- that is the whole
	// point of keeping one (issues 697, 715).
	if err := ds.SavePresets("1234567", "DEVICEID01", nil); err != nil {
		t.Fatalf("SavePresets (clear): %v", err)
	}

	if entries := ds.GetCatalog(); len(entries) != 1 {
		t.Fatalf("an emptied slot took its catalog entry with it: %d entries left", len(entries))
	}
}

// Recents carry no artwork, so a station that plays after having been a preset
// must not lose the logo the preset sighting contributed.
func TestSaveRecentsDoesNotCostAPresetEntryItsArtwork(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	preset := models.ServicePreset{ContainerArt: "http://192.0.2.10/art.png"}
	preset.Source = "TUNEIN"
	preset.Location = "s12345"
	preset.Name = "WDR 2"
	preset.ButtonNumber = "1"

	if err := ds.SavePresets("1234567", "DEVICEID01", []models.ServicePreset{preset}); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	// The speaker echoes the source name back as sourceAccount in recents.
	recent := models.ServiceRecent{}
	recent.Source = "TUNEIN"
	recent.SourceAccount = "TUNEIN"
	recent.Location = "s12345"
	recent.Name = "WDR 2"
	recent.ID = "1"

	if err := ds.SaveRecents("1234567", "DEVICEID01", []models.ServiceRecent{recent}); err != nil {
		t.Fatalf("SaveRecents: %v", err)
	}

	entries := ds.GetCatalog()
	if len(entries) != 1 {
		t.Fatalf("expected the recents sighting to merge into the preset entry, got %d entries", len(entries))
	}

	if entries[0].ContainerArt != "http://192.0.2.10/art.png" {
		t.Errorf("artwork lost: %q", entries[0].ContainerArt)
	}

	if entries[0].Origin != catalog.OriginPreset {
		t.Errorf("Origin = %q, want it to stay %q", entries[0].Origin, catalog.OriginPreset)
	}
}
