package datastore

import (
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TestSaveRecents_DeduplicatesByID is a regression test for the recents
// duplication bug: a speaker<->marge sync could re-store the same recent (same
// ID) multiple times, crowding the capped list and evicting other sources from
// the speaker's recents. SaveRecents must dedup by ID (first occurrence wins).
func TestSaveRecents_DeduplicatesByID(t *testing.T) {
	tmp, err := os.MkdirTemp("", "recents-dedup-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmp) }()

	ds := NewDataStore(tmp)
	account, device := "6919733", "A81B6A536A98"

	mk := func(id, name string) models.ServiceRecent {
		var r models.ServiceRecent
		r.ID = id
		r.Name = name
		r.Source = "7"
		r.SourceAccount = "4d696e69-444c-164e-9d41-72ecda78e4c1/0"
		r.Location = "1$4$2 TRACK"

		return r
	}

	// Same ID four times (the observed live state), plus two distinct recents.
	in := []models.ServiceRecent{
		mk("260614006", "03 - Salvation"),
		mk("260614006", "03 - Salvation"),
		mk("260614006", "03 - Salvation"),
		mk("260614006", "03 - Salvation"),
		mk("260614004", "06 - Back Burner"),
		mk("260613001", "Artifact"),
	}

	if err := ds.SaveRecents(account, device, in); err != nil {
		t.Fatalf("SaveRecents: %v", err)
	}

	out, err := ds.GetRecents(account, device)
	if err != nil {
		t.Fatalf("GetRecents: %v", err)
	}

	counts := map[string]int{}
	for _, r := range out {
		counts[r.ID]++
	}

	if counts["260614006"] != 1 {
		t.Errorf("duplicate recent not deduped: id 260614006 appears %d times (want 1)", counts["260614006"])
	}

	if len(out) != 3 {
		t.Errorf("expected 3 distinct recents, got %d: %+v", len(out), counts)
	}
}
