package marge

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TestSourceKeyTypeFromFullSource pins the projection of an upstream
// FullResponseSource (which carries <type>Audio</type> + a numeric
// <sourceproviderid>) back to the speaker-perspective SourceKeyType
// ("TUNEIN", "INTERNET_RADIO", …). syncPresets/syncRecents persist the
// projected value so the on-disk ServicePreset.Source matches what the
// speaker would write via its own /presets endpoint — speaker is the
// source of truth.
func TestSourceKeyTypeFromFullSource(t *testing.T) {
	cases := []struct {
		name string
		in   models.FullResponseSource
		want string
	}{
		{
			name: "tunein providerid 25 -> TUNEIN",
			in:   models.FullResponseSource{Type: "Audio", SourceProviderID: "25"},
			want: "TUNEIN",
		},
		{
			name: "internet_radio providerid 2 -> INTERNET_RADIO",
			in:   models.FullResponseSource{Type: "Audio", SourceProviderID: "2"},
			want: "INTERNET_RADIO",
		},
		{
			name: "local_internet_radio providerid 11 -> LOCAL_INTERNET_RADIO",
			in:   models.FullResponseSource{Type: "Audio", SourceProviderID: "11"},
			want: "LOCAL_INTERNET_RADIO",
		},
		{
			name: "spotify providerid 15 -> SPOTIFY",
			in:   models.FullResponseSource{Type: "Audio", SourceProviderID: "15"},
			want: "SPOTIFY",
		},
		{
			name: "radio_browser providerid 39 -> RADIO_BROWSER",
			in:   models.FullResponseSource{Type: "Audio", SourceProviderID: "39"},
			want: "RADIO_BROWSER",
		},
		{
			name: "unknown providerid falls back to upstream Type",
			in:   models.FullResponseSource{Type: "Audio", SourceProviderID: "99999"},
			want: "Audio",
		},
		{
			name: "empty providerid falls back to upstream Type",
			in:   models.FullResponseSource{Type: "Audio", SourceProviderID: ""},
			want: "Audio",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceKeyTypeFromFullSource(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
