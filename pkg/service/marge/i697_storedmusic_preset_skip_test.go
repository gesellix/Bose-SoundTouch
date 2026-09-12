package marge

import (
	"strconv"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

// Issue 697: syncing a device that holds both RADIO_BROWSER and STORED_MUSIC
// presets silently drops the STORED_MUSIC ones. The reporter's log shows the
// skip branch firing with an empty source id and an account that is the media
// server's *display name*:
//
//	[Marge] /full: skipping preset 2 — source "STORED_MUSIC" (id="",
//	account="<server display name>") not in configured sources and not
//	synthesisable; the speaker's preset slot will revert to empty …
//
// Both halves of that line matter. A configured STORED_MUSIC source is keyed
// by account "<UDN>/0" (see AddSource), so an account holding a display name
// cannot match it; and canonicalDefaultsByType has no STORED_MUSIC entry, so
// the synthesise branch cannot rescue it either. The preset is therefore
// dropped from /full, and the speaker empties that slot.
//
// syncPresets stores SourceAccount from p.Source.Username, i.e. whatever the
// speaker reported in the preset's source block, which is where the display
// name comes from.
//
// Identifiers below are placeholders; only the shape is from the report.
const (
	i697ServerUDN         = "UUUUUUUU-UUUU-UUUU-UUUU-UUUUUUUUUUUU/0"
	i697ServerDisplayName = "MEDIASERVER1"
)

func i697ConfiguredSources() []models.ConfiguredSource {
	return []models.ConfiguredSource{
		{
			ID:               "10005",
			Type:             "Audio",
			SourceKeyType:    constants.ProviderRadioBrowser,
			SourceProviderID: strconv.Itoa(constants.RadioBrowserProviderID),
			CreatedOn:        constants.DateStr,
			UpdatedOn:        constants.DateStr,
		},
		{
			// As AddSource persists a media server: the account identity is
			// the UDN, while the human-readable name lives alongside it.
			ID:               "10007",
			Type:             "Audio",
			SourceKeyType:    constants.ProviderStoredMusic,
			SourceKeyAccount: i697ServerUDN,
			SourceName:       i697ServerDisplayName,
			SourceProviderID: strconv.Itoa(constants.StoredMusicProviderID),
			CreatedOn:        constants.DateStr,
			UpdatedOn:        constants.DateStr,
		},
	}
}

func i697PresetButtons(t *testing.T, presets []models.FullResponsePreset) map[string]string {
	t.Helper()

	out := map[string]string{}

	for i := range presets {
		out[presets[i].ButtonNumber] = presets[i].Name
	}

	return out
}

// TestMapPresetsToFullResponse_StoredMusicPresetByServerName_GH697 is the
// reproducer: a STORED_MUSIC preset whose account is the server's display
// name, next to a resolvable RADIO_BROWSER preset, must survive the mapping.
func TestMapPresetsToFullResponse_StoredMusicPresetByServerName_GH697(t *testing.T) {
	presets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:            "A Station",
				Source:          constants.ProviderRadioBrowser,
				SourceID:        "10005",
				Location:        "/stations/byuuid/00000000-0000-0000-0000-000000000000",
				ContentItemType: "stationurl",
			},
			ButtonNumber: "1",
		},
		{
			// The speaker reported the source block's username as the media
			// server's display name, and no source id at all.
			ServiceContentItem: models.ServiceContentItem{
				Name:          "A Folder",
				Source:        constants.ProviderStoredMusic,
				SourceAccount: i697ServerDisplayName,
				Location:      "4:cont2:615:part12:39",
			},
			ButtonNumber: "2",
		},
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:          "Another Folder",
				Source:        constants.ProviderStoredMusic,
				SourceAccount: i697ServerDisplayName,
				Location:      "4:cont2:615:part12:40",
			},
			ButtonNumber: "3",
		},
	}

	got := mapPresetsToFullResponse(presets, i697ConfiguredSources())
	buttons := i697PresetButtons(t, got)

	if len(got) != len(presets) {
		t.Errorf("embedded %d preset(s), want %d — a dropped preset empties that slot on the speaker (got %v)",
			len(got), len(presets), buttons)
	}

	for _, button := range []string{"1", "2", "3"} {
		if _, ok := buttons[button]; !ok {
			t.Errorf("preset %s was skipped", button)
		}
	}

	// The STORED_MUSIC presets must bind to the media server's own source, so
	// play-time still resolves the right account.
	for i := range got {
		if got[i].ButtonNumber == "1" {
			continue
		}

		if got[i].Source.SourceProviderID != strconv.Itoa(constants.StoredMusicProviderID) {
			t.Errorf("preset %s bound to sourceproviderid %q, want the STORED_MUSIC provider",
				got[i].ButtonNumber, got[i].Source.SourceProviderID)
		}

		if got[i].Source.Username != i697ServerUDN && got[i].Source.Username != "" {
			t.Errorf("preset %s bound to username %q, want the server's UDN account or empty",
				got[i].ButtonNumber, got[i].Source.Username)
		}
	}
}

// Recents are matched the same way and were dropped the same way.
func TestMapRecentsToFullResponse_StoredMusicRecentByServerName_GH697(t *testing.T) {
	recents := []models.ServiceRecent{
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:            "1",
				Name:          "A Folder",
				Source:        constants.ProviderStoredMusic,
				SourceAccount: i697ServerDisplayName,
				Location:      "4:cont2:615:part12:39",
			},
		},
	}

	got := mapRecentsToFullResponse(recents, i697ConfiguredSources())

	if len(got) != 1 {
		t.Fatalf("embedded %d recent(s), want 1", len(got))
	}

	if got[0].Source.SourceProviderID != strconv.Itoa(constants.StoredMusicProviderID) {
		t.Errorf("bound to sourceproviderid %q, want the STORED_MUSIC provider", got[0].Source.SourceProviderID)
	}
}

// A media server that is genuinely gone from the configured list must still
// leave the preset in place: the slot keeps its name and play-time fails
// visibly, rather than the speaker emptying the slot.
func TestMapPresetsToFullResponse_StoredMusicServerGone_GH697(t *testing.T) {
	presets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:          "A Folder",
				Source:        constants.ProviderStoredMusic,
				SourceAccount: "SOMEOTHERSERVER",
				Location:      "4:cont2:615:part12:39",
			},
			ButtonNumber: "2",
		},
	}

	// Only RADIO_BROWSER is configured: no media server at all.
	configured := i697ConfiguredSources()[:1]

	got := mapPresetsToFullResponse(presets, configured)

	if len(got) != 1 {
		t.Fatalf("embedded %d preset(s), want 1: an unresolvable media server must not empty the slot", len(got))
	}

	if got[0].Source.SourceProviderID != strconv.Itoa(constants.StoredMusicProviderID) {
		t.Errorf("synthesised sourceproviderid %q, want the STORED_MUSIC provider", got[0].Source.SourceProviderID)
	}
}

// The write path stops the mismatch being persisted again.
func TestCanonicalSourceAccount_GH697(t *testing.T) {
	sources := i697ConfiguredSources()

	tests := []struct {
		name       string
		sourceType string
		reported   string
		want       string
	}{
		{
			name:       "media server display name resolves to its account",
			sourceType: constants.ProviderStoredMusic,
			reported:   i697ServerDisplayName,
			want:       i697ServerUDN,
		},
		{
			name:       "the account form is already canonical",
			sourceType: constants.ProviderStoredMusic,
			reported:   i697ServerUDN,
			want:       i697ServerUDN,
		},
		{
			name:       "an unexplained account is kept verbatim",
			sourceType: constants.ProviderStoredMusic,
			reported:   "SOMEOTHERSERVER",
			want:       "SOMEOTHERSERVER",
		},
		{
			name:       "empty stays empty",
			sourceType: constants.ProviderStoredMusic,
			reported:   "",
			want:       "",
		},
		{
			// Only STORED_MUSIC accepts a display name. Elsewhere the account
			// is the identity, and rewriting it could bind the wrong account.
			name:       "a display name is not resolved for other providers",
			sourceType: constants.ProviderRadioBrowser,
			reported:   i697ServerDisplayName,
			want:       i697ServerDisplayName,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := canonicalSourceAccount(sources, test.sourceType, test.reported); got != test.want {
				t.Errorf("canonicalSourceAccount(%q, %q) = %q, want %q",
					test.sourceType, test.reported, got, test.want)
			}
		})
	}
}

// TestMapPresetsToFullResponse_StoredMusicPresetByAccount_GH697 is the control
// case: the same preset keyed by the account form AddSource persists resolves
// today, which is what makes the display-name case a mismatch rather than a
// missing feature.
func TestMapPresetsToFullResponse_StoredMusicPresetByAccount_GH697(t *testing.T) {
	presets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:          "A Folder",
				Source:        constants.ProviderStoredMusic,
				SourceAccount: i697ServerUDN,
				Location:      "4:cont2:615:part12:39",
			},
			ButtonNumber: "2",
		},
	}

	got := mapPresetsToFullResponse(presets, i697ConfiguredSources())

	if len(got) != 1 {
		t.Fatalf("embedded %d preset(s), want 1: the account form must resolve", len(got))
	}

	if got[0].Source.SourceProviderID != strconv.Itoa(constants.StoredMusicProviderID) {
		t.Errorf("bound to sourceproviderid %q, want the STORED_MUSIC provider", got[0].Source.SourceProviderID)
	}
}
