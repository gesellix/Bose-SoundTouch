package soundtouchweb

import "testing"

// TestStoredMusicTypeForReplay covers deriving the ContentItem type from a
// STORED_MUSIC recent's location when the stored type is empty. Without a type
// the speaker rejects the select with INVALID_SOURCE.
//
// The locations are real shapes, captured on a SoundTouch 10 (FW 27.0.6)
// against two independent media servers, plus one from the issue 702 report.
// Tracks carry a kind suffix; containers carry none, and no server produced
// the " DIR" suffix the first version of this helper assumed.
func TestStoredMusicTypeForReplay(t *testing.T) {
	cases := []struct {
		name     string
		location string
		want     string
	}{
		{"track, example-dlna-server", "1$0 TRACK", "track"},
		{"track, FRITZ!Box", "5:audio5:part13:3171:5 TRACK", "track"},
		{"track, suffix wins over anything else", "1$4$2 TRACK", "track"},

		// Containers: no whitespace, no kind suffix. Typing these as "track"
		// was issue 702 — a folder recent replayed as a single track.
		{"album container, example-dlna-server", "1", "dir"},
		{"folder container, FRITZ!Box", "4:cont1:20:0:0:", "dir"},
		{"folder container, reporter's server", "22$2935", "dir"},
		{"folder container from a recentsUpdated frame", "4:cont2:615:part12:39", "dir"},

		// An explicit suffix is still honoured, whatever it says.
		{"explicit DIR suffix", "1 DIR", "dir"},
		{"explicit CONTAINER suffix", "1 CONTAINER", "container"},

		// The handler rejects an empty location before reaching here, so this
		// only pins that the fallback is the container side.
		{"empty location", "", "dir"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := storedMusicTypeForReplay(c.location); got != c.want {
				t.Errorf("storedMusicTypeForReplay(%q) = %q, want %q", c.location, got, c.want)
			}
		})
	}
}
