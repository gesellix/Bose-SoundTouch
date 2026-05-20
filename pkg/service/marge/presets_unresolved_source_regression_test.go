package marge

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

// TestMapPresetsToFullResponse_UnresolvedSource is a regression test for
// GH-269: when a preset on disk references a source that is no longer in
// the device's configured-sources list (e.g. after the user restored
// Presets.xml from a backup but Sources.xml was reset, or sources were
// renumbered after a factory reset), AfterTouch used to emit the preset
// in /full with a completely empty inner <source/> block.
//
// The speaker decodes /full as protobuf, where the inner source block
// has required fields (id, type, sourceproviderid, credential). An empty
// block fails IsInitialized() and the speaker aborts the whole account
// sync — wiping its locally stored presets in the process. Users saw
// "/presets is empty within seconds of AfterTouch coming online" even
// though the browser-readable /full XML looked right.
//
// The fix:
//
//  1. Presets that resolve against the configured-sources list are
//     emitted unchanged (happy path, no regression).
//  2. Presets whose SourceKeyType matches a built-in radio provider
//     (TuneIn, InternetRadio, LocalInternetRadio, RadioBrowser) get a
//     synthesised source block carrying the canonical default ID and
//     sourceproviderid. The credential is empty so play-time will fail
//     visibly, but the sync survives and other presets stay intact.
//  3. Presets whose SourceKeyType is account-bound (Spotify, Amazon)
//     and unresolvable are dropped from the response. The preset stays
//     on disk and returns once the source is repopulated; the speaker's
//     slot reverts to "Select a preset" until then.
func TestMapPresetsToFullResponse_UnresolvedSource(t *testing.T) {
	// Configured sources contain only TuneIn — RADIO_BROWSER, INTERNET_RADIO
	// and SPOTIFY are deliberately absent so unresolved presets exercise
	// the synthesise / skip branches.
	configured := []models.ConfiguredSource{
		{
			ID:               "14774275",
			Type:             "Audio",
			SourceKeyType:    constants.ProviderTunein,
			SourceProviderID: "25",
			CreatedOn:        "2017-07-20T16:43:48.000+00:00",
			UpdatedOn:        "2017-07-20T16:43:48.000+00:00",
		},
	}

	presets := []models.ServicePreset{
		// 1. Happy path: SourceID resolves directly against configured.
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:            "Resolved TuneIn",
				Source:          constants.ProviderTunein,
				SourceID:        "14774275",
				Location:        "/v1/playback/station/s166521",
				ContentItemType: "stationurl",
			},
			ButtonNumber: "1",
			CreatedOn:    "2026-04-04T21:25:33.000+00:00",
			UpdatedOn:    "2026-04-04T21:25:33.000+00:00",
		},
		// 2. Synthesise: INTERNET_RADIO type with no configured match.
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:            "Orphaned Internet Radio",
				Source:          constants.ProviderInternetRadio,
				SourceID:        "88888888",
				Location:        "http://example.invalid/stream.mp3",
				ContentItemType: "stationurl",
			},
			ButtonNumber: "2",
			CreatedOn:    "2026-04-04T21:25:33.000+00:00",
			UpdatedOn:    "2026-04-04T21:25:33.000+00:00",
		},
		// 3. Skip: Spotify is account-bound; no canonical default.
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:            "Orphaned Spotify",
				Source:          constants.ProviderSpotify,
				SourceID:        "100004",
				Location:        "/playback/container/abc",
				ContentItemType: "tracklisturl",
			},
			ButtonNumber: "3",
			CreatedOn:    "2026-04-04T21:25:33.000+00:00",
			UpdatedOn:    "2026-04-04T21:25:33.000+00:00",
		},
		// 4. Synthesise: RADIO_BROWSER with no configured match.
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:            "Orphaned RadioBrowser",
				Source:          constants.ProviderRadioBrowser,
				SourceID:        "99999999",
				Location:        "/something/radio-browser",
				ContentItemType: "stationurl",
			},
			ButtonNumber: "4",
			CreatedOn:    "2026-04-04T21:25:33.000+00:00",
			UpdatedOn:    "2026-04-04T21:25:33.000+00:00",
		},
	}

	got := mapPresetsToFullResponse(presets, configured)

	if len(got) != 3 {
		t.Fatalf("expected 3 emitted presets (1 resolved + 2 synthesised, Spotify skipped), got %d: %+v", len(got), got)
	}

	byButton := map[string]models.FullResponsePreset{}
	for _, p := range got {
		byButton[p.ButtonNumber] = p
	}

	if _, ok := byButton["3"]; ok {
		t.Errorf("expected preset 3 (orphaned Spotify) to be skipped, but it was emitted")
	}

	requireNonEmptySourceBlock(t, "preset 1 (resolved TuneIn)", byButton["1"])
	if byButton["1"].Source.ID != "14774275" {
		t.Errorf("preset 1: expected configured source id 14774275, got %q", byButton["1"].Source.ID)
	}

	if byButton["1"].Source.SourceProviderID != "25" {
		t.Errorf("preset 1: expected sourceproviderid 25 (TuneIn), got %q", byButton["1"].Source.SourceProviderID)
	}

	requireNonEmptySourceBlock(t, "preset 2 (synthesised InternetRadio)", byButton["2"])
	if byButton["2"].Source.ID != "10002" {
		t.Errorf("preset 2: expected canonical InternetRadio id 10002, got %q", byButton["2"].Source.ID)
	}

	if byButton["2"].Source.SourceProviderID != "2" {
		t.Errorf("preset 2: expected sourceproviderid 2 (InternetRadio), got %q", byButton["2"].Source.SourceProviderID)
	}

	requireNonEmptySourceBlock(t, "preset 4 (synthesised RadioBrowser)", byButton["4"])
	if byButton["4"].Source.ID != "10005" {
		t.Errorf("preset 4: expected canonical RadioBrowser id 10005, got %q", byButton["4"].Source.ID)
	}

	if byButton["4"].Source.SourceProviderID != "39" {
		t.Errorf("preset 4: expected sourceproviderid 39 (RadioBrowser), got %q", byButton["4"].Source.SourceProviderID)
	}
}

// requireNonEmptySourceBlock asserts the FullResponseSource has every
// protobuf-required leaf field populated. This is the structural invariant
// the speaker enforces when decoding /full — see the comment near
// AccountFullToXML about not stripping empty <sourceproviderid>.
func requireNonEmptySourceBlock(t *testing.T, label string, p models.FullResponsePreset) {
	t.Helper()

	if p.Source.ID == "" {
		t.Errorf("%s: source.id is empty", label)
	}

	if p.Source.Type == "" {
		t.Errorf("%s: source.type is empty", label)
	}

	if p.Source.SourceProviderID == "" {
		t.Errorf("%s: sourceproviderid is empty", label)
	}

	if p.Source.CreatedOn == "" {
		t.Errorf("%s: source.createdOn is empty", label)
	}

	if p.Source.UpdatedOn == "" {
		t.Errorf("%s: source.updatedOn is empty", label)
	}
}

// TestMapRecentsToFullResponse_UnresolvedSource is the recent-side twin of
// the preset regression above. Same protobuf invariant applies to the
// inner <source> of <recent>; an empty block aborts the whole /full sync
// — the older AccountFullToXML_RecentWithPoisonedSourceProviderID case
// caught one form of this via post-marshal stripping. The skip/synthesise
// path here closes the other form: orphan recent whose source is no
// longer configured at all.
func TestMapRecentsToFullResponse_UnresolvedSource(t *testing.T) {
	configured := []models.ConfiguredSource{
		{
			ID:               "14774275",
			Type:             "Audio",
			SourceKeyType:    constants.ProviderTunein,
			SourceProviderID: "25",
			CreatedOn:        "2017-07-20T16:43:48.000+00:00",
			UpdatedOn:        "2017-07-20T16:43:48.000+00:00",
		},
	}

	recents := []models.ServiceRecent{
		// 1. Resolved by exact SourceID match.
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:       "rec-1",
				Name:     "Resolved TuneIn",
				Source:   constants.ProviderTunein,
				SourceID: "14774275",
				Location: "/v1/playback/station/s166521",
			},
			LastPlayedAt: "2026-04-04T21:25:33.000+00:00",
		},
		// 2. Synthesise: LOCAL_INTERNET_RADIO no longer configured.
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:       "rec-2",
				Name:     "Orphaned laut.fm",
				Source:   constants.ProviderLocalInternetRadio,
				SourceID: "77777777",
				Location: "http://example.invalid/custom/v1/playback/abc",
			},
			LastPlayedAt: "2026-04-04T21:25:33.000+00:00",
		},
		// 3. Skip: Spotify is account-bound.
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:       "rec-3",
				Name:     "Orphaned Spotify",
				Source:   constants.ProviderSpotify,
				SourceID: "100004",
				Location: "/playback/container/abc",
			},
			LastPlayedAt: "2026-04-04T21:25:33.000+00:00",
		},
	}

	got := mapRecentsToFullResponse(recents, configured)

	if len(got) != 2 {
		t.Fatalf("expected 2 emitted recents (1 resolved + 1 synthesised, Spotify skipped), got %d", len(got))
	}

	byID := map[string]models.FullResponseRecent{}
	for _, r := range got {
		byID[r.ID] = r
	}

	if _, ok := byID["rec-3"]; ok {
		t.Errorf("expected rec-3 (orphaned Spotify) to be skipped, but it was emitted")
	}

	if byID["rec-1"].Source.SourceProviderID != "25" {
		t.Errorf("rec-1: expected sourceproviderid=25 (TuneIn), got %q", byID["rec-1"].Source.SourceProviderID)
	}

	if byID["rec-2"].Source.ID != "10003" {
		t.Errorf("rec-2: expected synthesised LocalInternetRadio id=10003, got %q", byID["rec-2"].Source.ID)
	}

	if byID["rec-2"].Source.SourceProviderID != "11" {
		t.Errorf("rec-2: expected synthesised LocalInternetRadio providerid=11, got %q", byID["rec-2"].Source.SourceProviderID)
	}
}

// TestMapPresetsToFullResponse_StrictTypeMatch_GH343 is the regression for
// GH-343: a preset stored with Source=TUNEIN used to come back from /full
// rebound to RADIOPLAYER because the numeric SourceID collided with a
// RADIOPLAYER source in the configured-sources list. The speaker, trusting
// /full as ground truth, then locally re-attributed the preset's source.
//
// Strict-match refuses to bind a TUNEIN preset to a RADIOPLAYER source,
// even when the IDs match. The fallback type-search then finds the right
// TUNEIN source.
func TestMapPresetsToFullResponse_StrictTypeMatch_GH343(t *testing.T) {
	// Two configured sources with the same numeric ID. In real life they'd
	// be from different devices/accounts; the bug surfaced when a stale
	// migration left a RADIOPLAYER source with the same ID a TUNEIN preset
	// referenced. Order matters here — RADIOPLAYER comes first so the
	// strict-match has to actively skip it to reach the TUNEIN entry.
	configured := []models.ConfiguredSource{
		{
			ID:               "10004",
			Type:             "Audio",
			SourceKeyType:    "RADIOPLAYER",
			SourceProviderID: "35",
		},
		{
			ID:               "10004",
			Type:             "Audio",
			SourceKeyType:    constants.ProviderTunein,
			SourceProviderID: "25",
		},
	}

	presets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:            "MDR JUMP",
				Source:          constants.ProviderTunein,
				SourceID:        "10004",
				Location:        "/v1/playback/station/s6634",
				ContentItemType: "stationurl",
			},
			ButtonNumber: "1",
		},
	}

	got := mapPresetsToFullResponse(presets, configured)

	if len(got) != 1 {
		t.Fatalf("expected 1 emitted preset, got %d", len(got))
	}

	if got[0].Source.SourceProviderID != "25" {
		t.Errorf("strict-match should have bound to TuneIn (sourceproviderid=25), got %q — cross-type bind not refused?", got[0].Source.SourceProviderID)
	}
}

// TestSourceTypeCompatible pins the strict-match policy:
//   - identical types compatible
//   - either side empty treated as "no info, allow"
//   - both set and different — refused
func TestSourceTypeCompatible(t *testing.T) {
	cases := []struct {
		claimed, configured string
		want                bool
	}{
		{"TUNEIN", "TUNEIN", true},
		{"TUNEIN", "RADIOPLAYER", false},
		{"", "TUNEIN", true},
		{"TUNEIN", "", true},
		{"", "", true},
		{"SPOTIFY", "SPOTIFY", true},
		{"SPOTIFY", "AMAZON", false},
	}

	for _, tc := range cases {
		if got := sourceTypeCompatible(tc.claimed, tc.configured); got != tc.want {
			t.Errorf("sourceTypeCompatible(%q, %q) = %v, want %v", tc.claimed, tc.configured, got, tc.want)
		}
	}
}

// TestCanonicalDefaultsByType pins the type → (id, providerid) mapping
// against the canonical IDs the speaker firmware ships with.
// canonicalProviderIDByID (the inverse) is already tested implicitly via
// the recents regression test; we want a direct check here too.
func TestCanonicalDefaultsByType(t *testing.T) {
	cases := []struct {
		sourceKeyType     string
		wantID            string
		wantProviderID    string
		wantSynthesisable bool
	}{
		{constants.ProviderInternetRadio, "10002", "2", true},
		{constants.ProviderLocalInternetRadio, "10003", "11", true},
		{constants.ProviderTunein, "10004", "25", true},
		{constants.ProviderRadioBrowser, "10005", "39", true},
		{constants.ProviderSpotify, "", "", false},
		{constants.ProviderAmazon, "", "", false},
		{"COMPLETELY_UNKNOWN", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.sourceKeyType, func(t *testing.T) {
			id, providerID := canonicalDefaultsByType(tc.sourceKeyType)
			if id != tc.wantID {
				t.Errorf("id: want %q, got %q", tc.wantID, id)
			}

			if providerID != tc.wantProviderID {
				t.Errorf("providerID: want %q, got %q", tc.wantProviderID, providerID)
			}

			synthesisable := id != "" && providerID != ""
			if synthesisable != tc.wantSynthesisable {
				t.Errorf("synthesisable: want %v, got %v", tc.wantSynthesisable, synthesisable)
			}
		})
	}
}
