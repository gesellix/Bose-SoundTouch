package datastore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
