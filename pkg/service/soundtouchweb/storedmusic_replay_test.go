package soundtouchweb

import "testing"

// TestStoredMusicTypeForReplay covers deriving the ContentItem type from a
// STORED_MUSIC recent's location when the stored type is empty. Without a type
// the speaker rejects the select with INVALID_SOURCE.
func TestStoredMusicTypeForReplay(t *testing.T) {
	cases := []struct {
		location string
		want     string
	}{
		{"1$4$2 TRACK", "track"},
		{"5:audio5:part13:5521:5 TRACK", "track"},
		{"1 DIR", "dir"},
		{"1 CONTAINER", "container"},
		{"noSuffix", "track"}, // fallback
		{"", "track"},         // fallback
	}

	for _, c := range cases {
		if got := storedMusicTypeForReplay(c.location); got != c.want {
			t.Errorf("storedMusicTypeForReplay(%q) = %q, want %q", c.location, got, c.want)
		}
	}
}
