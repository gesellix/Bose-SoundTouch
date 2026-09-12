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

// The probe is the measurement the matrix actually turns on: a trackID is only
// worth trusting once a real skip has been seen to move it.
func TestClassifySkipProbe(t *testing.T) {
	observe := func(source, trackID, track string) capabilityObservation {
		return observeCapabilities(&models.NowPlaying{Source: source, TrackID: trackID, Track: track})
	}

	tests := []struct {
		name          string
		before, after capabilityObservation
		want          string
	}{
		{
			name:   "a real skip moves the trackID",
			before: observe("SPOTIFY", "track:one", "First"),
			after:  observe("SPOTIFY", "track:two", "Second"),
			want:   "trackID changed: skips are verifiable on this source",
		},
		{
			name:   "a rolling stream title is not a skip",
			before: observe("TUNEIN", "station:one", "Song A"),
			after:  observe("TUNEIN", "station:one", "Song B"),
			want:   "trackID unchanged while the title changed: the title moves on its own and cannot confirm a skip",
		},
		{
			name:   "a source with no track concept has nothing to verify",
			before: observe("AUX", "", ""),
			after:  observe("AUX", "", ""),
			want:   "no trackID before or after: a readback has nothing to verify, so the player settles on the write",
		},
		{
			name:   "a frozen trackID is reported, not glossed over",
			before: observe("STORED_MUSIC", "track:one", "First"),
			after:  observe("STORED_MUSIC", "track:one", "First"),
			want:   "nothing changed within the probe window: either the skip did not land, or this source reports no change for one",
		},
		{
			name:   "a source change invalidates the probe",
			before: observe("SPOTIFY", "track:one", "First"),
			after:  observe("AUX", "", ""),
			want:   "source changed during the probe, so nothing can be concluded; rerun while one source stays put",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifySkipProbe(test.before, test.after); got != test.want {
				t.Errorf("classifySkipProbe() = %q, want %q", got, test.want)
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
