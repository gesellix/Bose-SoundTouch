package marge

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TestUpdateOrCreateRecent_MoveToFrontPreservesList is a regression test for the
// recents move-to-front corruption: when an existing recent is re-played, the
// in-place slice shuffle at the match branch could (a) return the wrong recent
// (a list neighbor) and (b) drop or duplicate entries in the saved list.
//
// Live evidence (account 6919733 / device A81B6A536A98, 2026-06-14): re-playing
// the Spotify album "White Water" returned the neighboring "Sand Castle Tapes"
// recent, and both Spotify recents subsequently vanished from a list that was
// well under the 10-item cap.
func TestUpdateOrCreateRecent_MoveToFrontPreservesList(t *testing.T) {
	spotify := &models.ConfiguredSource{}
	spotify.SourceKeyType = "SPOTIFY"
	spotify.SourceKeyAccount = "gesellix"

	mk := func(id, name, loc string) models.ServiceRecent {
		var r models.ServiceRecent
		r.ID = id
		r.Name = name
		r.Source = "SPOTIFY"
		r.SourceAccount = "gesellix"
		r.Location = loc

		return r
	}

	// Three distinct Spotify recents sharing the same source; they differ only
	// by location (the discriminator in the match branch).
	recents := []models.ServiceRecent{
		mk("1", "Sunday", "loc-sunday"),
		mk("2", "White Water", "loc-white"),
		mk("3", "Sand Castle", "loc-sand"),
	}

	// Re-play "White Water" (the middle entry) -> it should move to front,
	// the returned recent must BE White Water, and no entry may be lost.
	recentObj, out := updateOrCreateRecent(recents, "White Water", spotify, "tracklisturl", "loc-white", "DEVICEID01", 12345)

	if recentObj.Name != "White Water" || recentObj.Location != "loc-white" {
		t.Errorf("returned recent = %q (loc %q), want White Water/loc-white (neighbor leakage)", recentObj.Name, recentObj.Location)
	}

	if len(out) != 3 {
		t.Fatalf("recents count = %d, want 3 (entry lost/duplicated): %s", len(out), names(out))
	}

	seen := map[string]int{}
	for i := range out {
		seen[out[i].Location]++
	}

	for _, loc := range []string{"loc-sunday", "loc-white", "loc-sand"} {
		if seen[loc] != 1 {
			t.Errorf("location %q appears %d times, want 1: %s", loc, seen[loc], names(out))
		}
	}

	if out[0].Location != "loc-white" {
		t.Errorf("front entry = %q, want White Water moved to front: %s", out[0].Name, names(out))
	}
}

func names(rs []models.ServiceRecent) string {
	s := "["
	for i := range rs {
		if i > 0 {
			s += ", "
		}

		s += rs[i].Name + "(" + rs[i].Location + ")"
	}

	return s + "]"
}
