package catalog

import (
	"testing"
	"time"
)

func at(min int) time.Time {
	return time.Date(2026, 9, 20, 12, min, 0, 0, time.UTC)
}

// The speaker echoes the source name back as sourceAccount in recents while
// writing an empty one in presets, so the raw values differ for one and the
// same station. Without the normalisation the catalog would hold it twice and
// the pick list would show a duplicate.
func TestIdentityNormalizesThePlaceholderAccount(t *testing.T) {
	preset := Identity("TUNEIN", "", "s12345")
	recent := Identity("TUNEIN", "TUNEIN", "s12345")

	if preset != recent {
		t.Fatalf("placeholder account not normalized: %s vs %s", preset, recent)
	}

	if Identity("TUNEIN", "user@example.test", "s12345") == preset {
		t.Fatal("a real account must not collapse into the placeholder identity")
	}
}

func TestIdentityDistinguishesSourceAndLocation(t *testing.T) {
	base := Identity("TUNEIN", "", "s1")

	for _, other := range []string{
		Identity("RADIO_BROWSER", "", "s1"),
		Identity("TUNEIN", "", "s2"),
	} {
		if other == base {
			t.Fatalf("identity collision: %s", other)
		}
	}
}

func TestRecordIgnoresEntriesThatCannotBePickedAgain(t *testing.T) {
	var c Catalog

	for _, e := range []Entry{
		{Source: "", Location: "s1"},
		{Source: "TUNEIN", Location: ""},
	} {
		if c.Record(e, DefaultSize) {
			t.Fatalf("recorded an unusable entry: %+v", e)
		}
	}

	if len(c.Entries) != 0 {
		t.Fatalf("expected an empty catalog, got %d entries", len(c.Entries))
	}
}

func TestRecordMergesKeepsArtworkAndPromotesPresetOrigin(t *testing.T) {
	var c Catalog

	c.Record(Entry{
		Source: "TUNEIN", Location: "s1", Name: "WDR 2",
		ContainerArt: "http://192.0.2.10/art.png", Origin: OriginPreset,
		DeviceID: "DEVICEID01", FirstSeen: at(0), LastSeen: at(0),
	}, DefaultSize)

	// A recents sighting of the same station: no artwork (the speaker never
	// puts any there), and a weaker origin.
	c.Record(Entry{
		Source: "TUNEIN", SourceAccount: "TUNEIN", Location: "s1", Name: "WDR 2",
		Origin: OriginRecent, DeviceID: "DEVICEID02", FirstSeen: at(5), LastSeen: at(5),
	}, DefaultSize)

	if len(c.Entries) != 1 {
		t.Fatalf("expected one merged entry, got %d", len(c.Entries))
	}

	got := c.Entries[0]

	if got.ContainerArt != "http://192.0.2.10/art.png" {
		t.Errorf("artwork lost on merge: %q", got.ContainerArt)
	}

	if got.Origin != OriginPreset {
		t.Errorf("preset origin demoted to %q", got.Origin)
	}

	if !got.FirstSeen.Equal(at(0)) {
		t.Errorf("FirstSeen = %v, want the earlier sighting %v", got.FirstSeen, at(0))
	}

	if !got.LastSeen.Equal(at(5)) {
		t.Errorf("LastSeen = %v, want the later sighting %v", got.LastSeen, at(5))
	}

	if got.DeviceID != "DEVICEID02" {
		t.Errorf("DeviceID = %q, want the device of the latest sighting", got.DeviceID)
	}
}

func TestRecordDropsTheOldestSightingAtTheCap(t *testing.T) {
	var c Catalog

	for i := 1; i <= 4; i++ {
		c.Record(Entry{
			Source: "TUNEIN", Location: string(rune('a' + i)), LastSeen: at(i),
		}, 3)
	}

	list := c.List()
	if len(list) != 3 {
		t.Fatalf("expected the cap to hold at 3, got %d", len(list))
	}

	if !list[0].LastSeen.Equal(at(4)) {
		t.Errorf("list is not newest-first: %v", list[0].LastSeen)
	}

	for _, e := range list {
		if e.LastSeen.Equal(at(1)) {
			t.Fatal("the oldest sighting survived the cap")
		}
	}
}

func TestRecordReportsWhetherAnythingChanged(t *testing.T) {
	var c Catalog

	e := Entry{Source: "TUNEIN", Location: "s1", Name: "WDR 2", FirstSeen: at(0), LastSeen: at(0)}

	if !c.Record(e, DefaultSize) {
		t.Fatal("the first sighting must count as a change")
	}

	if c.Record(e, DefaultSize) {
		t.Fatal("recording the identical sighting again must not count as a change")
	}
}

// A size of zero is how an operator turns the catalog off, which has to drop
// what is already stored rather than freeze it in place.
func TestZeroSizeDisablesAndClears(t *testing.T) {
	var c Catalog

	c.Record(Entry{Source: "TUNEIN", Location: "s1", LastSeen: at(0)}, DefaultSize)

	if !c.Record(Entry{Source: "TUNEIN", Location: "s2", LastSeen: at(1)}, 0) {
		t.Fatal("disabling a non-empty catalog must count as a change")
	}

	if len(c.Entries) != 0 {
		t.Fatalf("expected the catalog to be cleared, got %d entries", len(c.Entries))
	}
}

func TestTrimAppliesALoweredCap(t *testing.T) {
	var c Catalog

	for i := 1; i <= 5; i++ {
		c.Record(Entry{Source: "TUNEIN", Location: string(rune('a' + i)), LastSeen: at(i)}, DefaultSize)
	}

	if !c.Trim(2) {
		t.Fatal("lowering the cap must report a change")
	}

	if len(c.Entries) != 2 {
		t.Fatalf("expected 2 entries after the trim, got %d", len(c.Entries))
	}

	if c.Trim(2) {
		t.Fatal("trimming to the same cap again must not report a change")
	}
}

func TestListDoesNotAliasTheStoredEntries(t *testing.T) {
	var c Catalog

	c.Record(Entry{Source: "TUNEIN", Location: "s1", Name: "WDR 2", LastSeen: at(0)}, DefaultSize)

	list := c.List()
	list[0].Name = "mutated"

	if c.Entries[0].Name != "WDR 2" {
		t.Fatal("List returned a view onto the stored entries")
	}
}

// A speaker re-posts its whole preset list on every sync. Each of those is a
// genuine new sighting, but rewriting the catalog for a moved clock reading
// alone is write amplification on a speaker's flash volume.
func TestRepeatedSightingsDoNotCountAsChangesWithinTheGranularity(t *testing.T) {
	var c Catalog

	e := Entry{Source: "TUNEIN", Location: "s1", Name: "WDR 2", LastSeen: at(0)}
	c.Record(e, DefaultSize)

	e.LastSeen = at(0).Add(SeenGranularity - time.Minute)

	if c.Record(e, DefaultSize) {
		t.Fatal("a sighting within the granularity must not count as a change")
	}

	if !c.Entries[0].LastSeen.Equal(at(0)) {
		t.Fatal("the stored entry was rewritten anyway")
	}

	e.LastSeen = at(0).Add(SeenGranularity)

	if !c.Record(e, DefaultSize) {
		t.Fatal("a sighting past the granularity must count as a change")
	}

	if !c.Entries[0].LastSeen.Equal(at(0).Add(SeenGranularity)) {
		t.Fatalf("LastSeen was not advanced: %v", c.Entries[0].LastSeen)
	}
}

// Metadata that actually differs is worth a write whatever the clock says:
// this is how a station that gains artwork reaches the pick list.
func TestNewMetadataCountsAsAChangeWithinTheGranularity(t *testing.T) {
	var c Catalog

	c.Record(Entry{Source: "TUNEIN", Location: "s1", Name: "WDR 2", LastSeen: at(0)}, DefaultSize)

	if !c.Record(Entry{
		Source: "TUNEIN", Location: "s1", Name: "WDR 2",
		ContainerArt: "http://192.0.2.10/art.png", LastSeen: at(1),
	}, DefaultSize) {
		t.Fatal("newly learned artwork must count as a change")
	}
}
