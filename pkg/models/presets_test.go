package models

import (
	"encoding/xml"
	"testing"
)

// reporterXML is the /presets response captured from the speaker that
// crashed the CLI in issue #308 (ST10 post factory reset, FW 27.0.6).
// Two configured presets followed by three self-closing <preset/>
// placeholders. The original crash happened on the first <preset/>:
// GetDisplayName() handled the nil ContentItem, but the very next
// line dereferenced ContentItem.Source unconditionally.
const reporterXML = `<presets>
<preset id="1" createdOn="1348058580" updatedOn="1348058580">
<ContentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s6634" sourceAccount="" isPresetable="true">
<itemName>MDR JUMP</itemName>
<containerArt/>
</ContentItem>
</preset>
<preset id="2" createdOn="1348058580" updatedOn="1348058580">
<ContentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s10637" sourceAccount="" isPresetable="true">
<itemName>SUNSHINE LIVE</itemName>
<containerArt>
http://cdn-profiles.tunein.com/s10637/images/logog.png?t=637791086340000000
</containerArt>
</ContentItem>
</preset>
<preset/>
<preset/>
<preset/>
</presets>`

// invalidSourceXML is the second placeholder shape observed in the
// wild (gesellix's ST10/ST20 on FW 27.0.6, never factory-reset). The
// firmware here populates ContentItem with source="INVALID_SOURCE"
// for unconfigured slots — non-nil but useless, so the old IsEmpty
// (== nil only) returned false and the placeholders polluted listings.
const invalidSourceXML = `<?xml version="1.0" encoding="UTF-8" ?><presets>` +
	`<preset id="0"><ContentItem source="INVALID_SOURCE" isPresetable="true" /></preset>` +
	`<preset id="0"><ContentItem source="INVALID_SOURCE" isPresetable="true" /></preset>` +
	`<preset id="0"><ContentItem source="INVALID_SOURCE" isPresetable="true" /></preset>` +
	`<preset id="1"><ContentItem source="SPOTIFY" type="tracklisturl" location="/playback/container/abc" sourceAccount="user" isPresetable="true"><itemName>Sand Castle Tapes</itemName><containerArt></containerArt></ContentItem></preset>` +
	`<preset id="2" createdOn="1778965482" updatedOn="1778965482"><ContentItem source="SPOTIFY" type="tracklisturl" location="/playback/container/def" sourceAccount="user" isPresetable="true"><itemName>Unplugged</itemName><containerArt>https://example.com/art.jpg</containerArt></ContentItem></preset>` +
	`<preset id="6"><ContentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s166521" sourceAccount="" isPresetable="true"><itemName>SMOOTH JAZZ</itemName><containerArt>https://example.com/logo.png</containerArt></ContentItem></preset>` +
	`</presets>`

func TestIsEmpty_NoContentItem(t *testing.T) {
	// Shape A: <preset/> — ContentItem == nil. This is the shape
	// behind the issue #308 crash.
	p := Preset{}
	if !p.IsEmpty() {
		t.Error("IsEmpty() should be true when ContentItem is nil")
	}
}

func TestIsEmpty_InvalidSourcePlaceholder(t *testing.T) {
	// Shape B: ContentItem present but Source == "INVALID_SOURCE".
	// Observed on devices that never had a factory reset.
	p := Preset{
		ContentItem: &ContentItem{Source: "INVALID_SOURCE", IsPresetable: true},
	}
	if !p.IsEmpty() {
		t.Error("IsEmpty() should be true when ContentItem has INVALID_SOURCE")
	}
}

func TestIsEmpty_EmptySource(t *testing.T) {
	// A ContentItem with no Source can't drive playback. Treat it
	// as empty too — defensive, not tied to a single observed shape.
	p := Preset{ContentItem: &ContentItem{}}
	if !p.IsEmpty() {
		t.Error("IsEmpty() should be true when ContentItem.Source is empty")
	}
}

func TestIsEmpty_RealPreset(t *testing.T) {
	p := Preset{
		ContentItem: &ContentItem{
			Source:   "TUNEIN",
			ItemName: "MDR JUMP",
		},
	}
	if p.IsEmpty() {
		t.Error("IsEmpty() should be false for a configured preset")
	}
}

func TestReporterXML_DoesNotPanicAndFiltersEmpty(t *testing.T) {
	// Reproducer for issue #308: simulate the loop that crashed the
	// CLI. The fix is two-fold: IsEmpty now recognises <preset/>,
	// and callers use the nil-safe Get* accessors. Walking every
	// preset through the same paths the CLI uses must not panic on
	// any entry.
	var presets Presets

	if err := xml.Unmarshal([]byte(reporterXML), &presets); err != nil {
		t.Fatalf("Failed to unmarshal reporter XML: %v", err)
	}

	if got := len(presets.Preset); got != 5 {
		t.Fatalf("Expected 5 preset entries (2 configured + 3 empty), got %d", got)
	}

	emptyCount := 0
	configuredCount := 0

	for _, p := range presets.Preset {
		// The CLI now skips empty presets before dereferencing
		// anything on ContentItem. The IsEmpty call must catch all
		// three <preset/> entries.
		if p.IsEmpty() {
			emptyCount++
			continue
		}

		configuredCount++

		// These calls would have panicked pre-fix on the empty
		// entries; here they exercise the still-printed paths for
		// the real ones.
		_ = p.GetDisplayName()
		_ = p.GetSource()
		_ = p.GetSourceAccount()
		_ = p.GetLocation()
	}

	if emptyCount != 3 {
		t.Errorf("Expected 3 empty presets, got %d", emptyCount)
	}

	if configuredCount != 2 {
		t.Errorf("Expected 2 configured presets, got %d", configuredCount)
	}

	// HasPresets should reflect "there are real presets" — not
	// confused by the placeholders.
	if !presets.HasPresets() {
		t.Error("HasPresets() should be true (2 real presets present)")
	}

	if got := presets.GetUsedPresetSlots(); len(got) != 2 {
		t.Errorf("GetUsedPresetSlots() = %v; want 2 entries", got)
	}
}

func TestInvalidSourceXML_PlaceholdersFilteredOut(t *testing.T) {
	// Second-shape reproducer: three INVALID_SOURCE placeholders
	// preceding three real presets. Before the IsEmpty extension,
	// listings printed "0. Preset 0 / Source: INVALID_SOURCE" three
	// times before the real entries — annoying, not crashing.
	var presets Presets

	if err := xml.Unmarshal([]byte(invalidSourceXML), &presets); err != nil {
		t.Fatalf("Failed to unmarshal invalid-source XML: %v", err)
	}

	if got := len(presets.Preset); got != 6 {
		t.Fatalf("Expected 6 preset entries, got %d", got)
	}

	configured := 0

	for _, p := range presets.Preset {
		if !p.IsEmpty() {
			configured++
		}
	}

	if configured != 3 {
		t.Errorf("Expected 3 configured presets (after filtering INVALID_SOURCE placeholders), got %d",
			configured)
	}

	// The three placeholders all carry id="0", so used-slot
	// reporting should ignore them and show only the real ids.
	used := presets.GetUsedPresetSlots()
	if len(used) != 3 {
		t.Fatalf("GetUsedPresetSlots() = %v; want 3 entries", used)
	}

	wantIDs := map[int]bool{1: true, 2: true, 6: true}
	for _, id := range used {
		if !wantIDs[id] {
			t.Errorf("Unexpected used slot id %d; want one of %v", id, []int{1, 2, 6})
		}
	}
}
