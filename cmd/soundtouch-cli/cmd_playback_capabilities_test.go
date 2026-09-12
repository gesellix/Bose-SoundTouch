package main

import (
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// The two fixtures under pkg/client/testdata and pkg/service/setup/testdata are
// the only captured now-playing responses in the tree, both Spotify, and both
// carry skipEnabled and a trackID. The radio and physical-input cases below are
// what the sweep is meant to confirm or refute on hardware, so they are written
// as the shapes the matrix expects, not as measurements.
func TestSkipVerdictClassifiesSources(t *testing.T) {
	tests := []struct {
		name       string
		nowPlaying *models.NowPlaying
		want       string
	}{
		{
			name: "track source with a trackID is verifiable",
			nowPlaying: &models.NowPlaying{
				Source:      "SPOTIFY",
				TrackID:     "spotify:track:3LX0dk3YT8cUgp7XxUJgTB",
				SkipEnabled: &models.SkipEnabled{},
			},
			want: "verifiable by trackID",
		},
		{
			name: "source claiming skip without a trackID cannot be verified",
			nowPlaying: &models.NowPlaying{
				Source:      "TUNEIN",
				SkipEnabled: &models.SkipEnabled{},
			},
			want: "claims skip, no trackID",
		},
		{
			name:       "source with no track concept",
			nowPlaying: &models.NowPlaying{Source: "AUX"},
			want:       "no track concept",
		},
		{
			name:       "standby has nothing to say",
			nowPlaying: &models.NowPlaying{Source: "STANDBY"},
			want:       "n/a (nothing playing)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := observeCapabilities(test.nowPlaying).skipVerdict(); got != test.want {
				t.Errorf("skipVerdict() = %q, want %q", got, test.want)
			}
		})
	}
}

// A sweep is useless if it reprints the same source every poll, and equally
// useless if it misses the change a skip produces.
func TestReportsSameCapabilitiesDetectsTheChangesThatMatter(t *testing.T) {
	base := observeCapabilities(&models.NowPlaying{
		Source:      "SPOTIFY",
		TrackID:     "spotify:track:one",
		Track:       "First track",
		SkipEnabled: &models.SkipEnabled{},
	})

	tests := []struct {
		name    string
		current *models.NowPlaying
		same    bool
	}{
		{
			name: "same track reported again",
			current: &models.NowPlaying{
				Source:      "SPOTIFY",
				TrackID:     "spotify:track:one",
				Track:       "First track",
				SkipEnabled: &models.SkipEnabled{},
			},
			same: true,
		},
		{
			name: "a skip changes the trackID",
			current: &models.NowPlaying{
				Source:      "SPOTIFY",
				TrackID:     "spotify:track:two",
				Track:       "Second track",
				SkipEnabled: &models.SkipEnabled{},
			},
			same: false,
		},
		{
			name: "switching source changes everything",
			current: &models.NowPlaying{
				Source: "AUX",
			},
			same: false,
		},
		{
			name: "a stream rewriting only its title is still a new row",
			current: &models.NowPlaying{
				Source:      "SPOTIFY",
				TrackID:     "spotify:track:one",
				Track:       "Rolling title",
				SkipEnabled: &models.SkipEnabled{},
			},
			same: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := reportsSameCapabilities(base, observeCapabilities(test.current)); got != test.same {
				t.Errorf("reportsSameCapabilities() = %t, want %t", got, test.same)
			}
		})
	}
}

func TestCapabilityRowFillsEveryColumn(t *testing.T) {
	observed := time.Date(2026, 9, 12, 14, 30, 15, 0, time.UTC)
	row := capabilityRow(observed, observeCapabilities(&models.NowPlaying{
		Source:              "LOCAL_INTERNET_RADIO",
		PlayStatus:          models.PlayStatusPlaying,
		StreamType:          "RADIO_STREAMING",
		SkipPreviousEnabled: &models.SkipPreviousEnabled{},
	}))

	if len(row) != len(capabilityHeader()) {
		t.Fatalf("row has %d columns, header has %d", len(row), len(capabilityHeader()))
	}

	// Empty fields must not collapse the columns of a tab-separated row.
	for i, cell := range row {
		if cell == "" {
			t.Errorf("column %q is empty, want a placeholder", capabilityHeader()[i])
		}
	}

	if row[0] != "14:30:15" {
		t.Errorf("time column = %q, want %q", row[0], "14:30:15")
	}

	if row[5] != "-" {
		t.Errorf("trackID column = %q, want the %q placeholder", row[5], "-")
	}
}
